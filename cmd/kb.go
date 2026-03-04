package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/kb"
)

var kbCmd = &cobra.Command{
	Use:   "kb",
	Short: "Knowledge base operations",
}

var kbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List knowledge base files",
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := appKB.List()
		if err != nil {
			return err
		}

		if len(files) == 0 {
			fmt.Println("No knowledge base files found.")
			return nil
		}

		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	},
}

var kbShowCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show a knowledge base file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := appKB.Read(args[0])
		if err != nil {
			return err
		}
		fmt.Print(content)
		return nil
	},
}

var kbSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search knowledge base",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		results, err := appKB.Search(query)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No results found.")
			return nil
		}

		for _, r := range results {
			fmt.Printf("%s:%d: %s\n", r.Path, r.Line, r.Content)
		}
		return nil
	},
}

var kbImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import KB files with front matter weights into the database",
	Long: `Scans all knowledge base markdown files for YAML front matter metadata.
For each file with metadata, creates a note in the database using the
embedded weights (strength, salience, decay_rate, etc.) and establishes
memory edges from the related entries.

This enables portable knowledge: copy KB files from another environment
and run 'sb kb import' to restore weights into the local database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := appKB.List()
		if err != nil {
			return fmt.Errorf("list KB files: %w", err)
		}
		if len(files) == 0 {
			fmt.Println("No knowledge base files found.")
			return nil
		}

		// Phase 1: Import notes and collect path→noteID mapping.
		kbPathToNoteID := make(map[string]int64)
		type pendingEdge struct {
			noteID int64
			meta   kb.ImportMeta
		}
		var pending []pendingEdge
		imported := 0

		for _, f := range files {
			content, err := appKB.Read(f)
			if err != nil {
				fmt.Printf("  skip %s: read error: %v\n", f, err)
				continue
			}

			meta, body := kb.ParseFrontMatter(content)
			if meta.Strength == 0 && meta.Salience == 0 && len(meta.Tags) == 0 {
				// No meaningful front matter; skip.
				continue
			}

			noteID, err := appStore.ImportKBNote(body, meta)
			if err != nil {
				fmt.Printf("  skip %s: import error: %v\n", f, err)
				continue
			}

			_ = appStore.MapKBNotes(f, []int64{noteID})
			kbPathToNoteID[f] = noteID
			imported++

			if len(meta.Related) > 0 {
				pending = append(pending, pendingEdge{
					noteID: noteID,
					meta:   kb.ImportMeta{Related: meta.Related},
				})
			}

			fmt.Printf("  imported %s (note #%d)\n", f, noteID)
		}

		// Phase 2: Create memory edges (now that all notes exist).
		edgesCreated := 0
		for _, p := range pending {
			for _, r := range p.meta.Related {
				targetID, ok := kbPathToNoteID[r.Path]
				if !ok || targetID == p.noteID {
					continue
				}
				weight := r.Weight
				if weight <= 0 {
					weight = 0.10
				}
				if err := appStore.LinkNotes(p.noteID, targetID, weight, "kb-import"); err != nil {
					continue
				}
				edgesCreated++
			}
		}

		fmt.Printf("\nImported %d KB files, created %d memory edges.\n", imported, edgesCreated)
		return nil
	},
}

var kbExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Update KB files with current weights from the database",
	Long: `Reads current note weights from the database and updates the YAML
front matter in each KB file. This ensures the portable metadata
reflects the latest strength, salience, and relationship weights.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := appKB.List()
		if err != nil {
			return fmt.Errorf("list KB files: %w", err)
		}
		if len(files) == 0 {
			fmt.Println("No knowledge base files found.")
			return nil
		}

		updated := 0
		for _, f := range files {
			notes, err := appStore.NotesByKBPath(f)
			if err != nil || len(notes) == 0 {
				continue
			}

			content, err := appKB.Read(f)
			if err != nil {
				continue
			}
			body := kb.StripFrontMatter(content)

			relatedFiles, _ := appStore.RelatedKBFiles(f, 5)
			meta := kb.BuildMetadataForKBWrite(notes, relatedFiles)

			if err := appKB.WriteWithMetadata(f, body, meta); err != nil {
				fmt.Printf("  skip %s: write error: %v\n", f, err)
				continue
			}

			updated++
			fmt.Printf("  exported %s\n", f)
		}

		fmt.Printf("\nExported weights for %d KB files.\n", updated)
		return nil
	},
}

func init() {
	kbCmd.AddCommand(kbListCmd, kbShowCmd, kbSearchCmd, kbImportCmd, kbExportCmd)
	rootCmd.AddCommand(kbCmd)
}
