package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/store"
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage notes",
}

var noteAddCmd = &cobra.Command{
	Use:   "add <content>",
	Short: "Add a new note",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := strings.Join(args, " ")

		// Read from stdin if content is "-"
		if content == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			content = strings.TrimSpace(string(data))
		}

		var tags []string
		if t, _ := cmd.Flags().GetString("tags"); t != "" {
			tags = strings.Split(t, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
		}

		source, _ := cmd.Flags().GetString("source")
		sessionFlag, _ := cmd.Flags().GetInt64("session")

		var sessionID *int64
		if cmd.Flags().Changed("session") {
			sessionID = &sessionFlag
		} else {
			sess, _ := appStore.ActiveSession()
			if sess != nil {
				sessionID = &sess.ID
			}
		}

		note, err := appStore.CreateNote(content, sessionID, tags, source)
		if err != nil {
			return err
		}
		fmt.Printf("Note #%d created.\n", note.ID)
		return nil
	},
}

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := store.NoteFilter{}

		if cmd.Flags().Changed("session") {
			sid, _ := cmd.Flags().GetInt64("session")
			filter.SessionID = &sid
		}
		if t, _ := cmd.Flags().GetString("tag"); t != "" {
			filter.Tag = &t
		}

		notes, err := appStore.ListNotes(filter)
		if err != nil {
			return err
		}

		if len(notes) == 0 {
			fmt.Println("No notes found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCONTENT\tTAGS\tSESSION")
		for _, n := range notes {
			content := n.Content
			if len(content) > 50 {
				content = content[:50] + "..."
			}
			tagsStr := strings.Join(n.Tags, ",")
			sessStr := "-"
			if n.SessionID != nil {
				sessStr = fmt.Sprintf("#%d", *n.SessionID)
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", n.ID, content, tagsStr, sessStr)
		}
		return w.Flush()
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid note ID: %s", args[0])
		}

		note, err := appStore.GetNote(id)
		if err != nil {
			return err
		}

		fmt.Printf("Note #%d\n", note.ID)
		if note.SessionID != nil {
			fmt.Printf("  Session: #%d\n", *note.SessionID)
		}
		if len(note.Tags) > 0 {
			fmt.Printf("  Tags:    %s\n", strings.Join(note.Tags, ", "))
		}
		if note.Source != "" {
			fmt.Printf("  Source:  %s\n", note.Source)
		}
		fmt.Printf("  Created: %s\n", note.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Printf("\n%s\n", note.Content)
		return nil
	},
}

var noteRecallCmd = &cobra.Command{
	Use:   "recall <id>",
	Short: "Recall a note to reinforce memory strength",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid note ID: %s", args[0])
		}

		before, err := appStore.GetNote(id)
		if err != nil {
			return err
		}
		if err := appStore.RecallNote(id, time.Now().UTC(), "cli"); err != nil {
			return err
		}
		after, err := appStore.GetNote(id)
		if err != nil {
			return err
		}

		fmt.Printf("Note #%d recalled. strength: %.3f -> %.3f (recall_count=%d)\n",
			id, before.Strength, after.Strength, after.RecallCount)
		return nil
	},
}

func noteFilterForSession(sessionID int64) store.NoteFilter {
	return store.NoteFilter{SessionID: &sessionID}
}

func init() {
	noteAddCmd.Flags().String("tags", "", "Comma-separated tags")
	noteAddCmd.Flags().String("source", "", "Note source")
	noteAddCmd.Flags().Int64("session", 0, "Session ID (default: active session)")

	noteListCmd.Flags().Int64("session", 0, "Filter by session ID")
	noteListCmd.Flags().String("tag", "", "Filter by tag")

	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteShowCmd, noteRecallCmd)
	rootCmd.AddCommand(noteCmd)
}
