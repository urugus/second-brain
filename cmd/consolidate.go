package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	claudeAdapter "github.com/urugus/second-brain/internal/adapter/claude"
	"github.com/urugus/second-brain/internal/consolidation"
)

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Consolidate a session into the knowledge base",
	Long: `Analyze a completed session using an AI agent and produce knowledge base
updates, a session summary, and suggested follow-up tasks.

By default, consolidates the most recently completed session that has not
yet been consolidated. Use --session to specify a particular session.`,
	RunE: runConsolidate,
}

func init() {
	consolidateCmd.Flags().Int64("session", 0, "Session ID to consolidate (default: latest unconsolidated)")
	consolidateCmd.Flags().BoolP("yes", "y", false, "Auto-approve all changes")
	consolidateCmd.Flags().Bool("dry-run", false, "Show proposed changes without applying")
	consolidateCmd.Flags().String("model", "", "Claude model to use")
	rootCmd.AddCommand(consolidateCmd)
}

func runConsolidate(cmd *cobra.Command, args []string) error {
	// Build agent
	var opts []claudeAdapter.Option
	if model, _ := cmd.Flags().GetString("model"); model != "" {
		opts = append(opts, claudeAdapter.WithModel(model))
	}
	agent := claudeAdapter.New(opts...)

	// Build service
	svc := consolidation.NewService(appStore, appKB, agent)

	// Determine session ID
	sessionID, _ := cmd.Flags().GetInt64("session")
	if sessionID == 0 {
		sess, err := appStore.LatestUnconsolidatedSession()
		if err != nil {
			return fmt.Errorf("find session: %w", err)
		}
		if sess == nil {
			fmt.Println("No unconsolidated sessions found.")
			return nil
		}
		sessionID = sess.ID
		fmt.Printf("Found unconsolidated session #%d: %s\n", sess.ID, sess.Title)
	}

	// Propose
	fmt.Printf("Consolidating session #%d...\n", sessionID)
	proposed, err := svc.Propose(cmd.Context(), sessionID)
	if err != nil {
		return err
	}

	// Display proposal
	displayProposal(proposed)

	// Dry-run mode
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Println("\n(dry-run mode: no changes applied)")
		return nil
	}

	// Get approval
	autoApprove, _ := cmd.Flags().GetBool("yes")
	approvedKB, approvedTasks := getApproval(proposed, autoApprove)

	if len(approvedKB) == 0 && len(approvedTasks) == 0 {
		fmt.Println("\nNo changes approved.")
		return nil
	}

	// Apply
	if err := svc.Apply(cmd.Context(), proposed, approvedKB, approvedTasks); err != nil {
		return err
	}

	fmt.Printf("\nDone. %d KB files updated, %d tasks created.\n", len(approvedKB), len(approvedTasks))
	return nil
}

func displayProposal(p *consolidation.ProposedChanges) {
	fmt.Printf("\n=== Session #%d ===\n", p.SessionID)
	fmt.Printf("\nSummary: %s\n", p.Summary)

	if len(p.KBUpdates) > 0 {
		fmt.Printf("\n--- KB Updates (%d files) ---\n", len(p.KBUpdates))
		for i, u := range p.KBUpdates {
			label := "UPDATE"
			if u.IsNew {
				label = "NEW"
			}
			fmt.Printf("\n[%d] %s (%s)\n", i+1, u.Path, label)
			fmt.Printf("    Reason: %s\n", u.Reason)

			if u.IsNew {
				lines := strings.Count(u.Content, "\n")
				fmt.Printf("    (%d lines)\n", lines)
			} else {
				displaySimpleDiff(u.OldContent, u.Content)
			}
		}
	}

	if len(p.SuggestedTasks) > 0 {
		fmt.Printf("\n--- Suggested Tasks (%d) ---\n", len(p.SuggestedTasks))
		for i, t := range p.SuggestedTasks {
			fmt.Printf("[%d] %q\n", i+1, t)
		}
	}
}

func displaySimpleDiff(old, new string) {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// Simple line-count diff summary
	added := 0
	removed := 0
	if len(newLines) > len(oldLines) {
		added = len(newLines) - len(oldLines)
	} else {
		removed = len(oldLines) - len(newLines)
	}

	changed := 0
	minLen := len(oldLines)
	if len(newLines) < minLen {
		minLen = len(newLines)
	}
	for i := 0; i < minLen; i++ {
		if oldLines[i] != newLines[i] {
			changed++
		}
	}

	fmt.Printf("    %d lines changed, +%d/-%d lines\n", changed, added, removed)
}

func getApproval(p *consolidation.ProposedChanges, autoApprove bool) ([]int, []int) {
	if autoApprove {
		kb := make([]int, len(p.KBUpdates))
		for i := range kb {
			kb[i] = i
		}
		tasks := make([]int, len(p.SuggestedTasks))
		for i := range tasks {
			tasks[i] = i
		}
		return kb, tasks
	}

	scanner := bufio.NewScanner(os.Stdin)
	var approvedKB []int
	var approvedTasks []int

	for i, u := range p.KBUpdates {
		label := "NEW"
		if !u.IsNew {
			label = "UPDATE"
		}
		fmt.Printf("\nApply [%d] %s (%s)? [y/n/v(iew)] ", i+1, u.Path, label)

		for scanner.Scan() {
			input := strings.ToLower(strings.TrimSpace(scanner.Text()))
			switch input {
			case "y", "yes":
				approvedKB = append(approvedKB, i)
				goto nextKB
			case "n", "no":
				goto nextKB
			case "v", "view":
				fmt.Println("---")
				fmt.Print(u.Content)
				fmt.Println("---")
				fmt.Printf("Apply [%d] %s? [y/n] ", i+1, u.Path)
			default:
				fmt.Print("[y/n/v] ")
			}
		}
	nextKB:
	}

	for i, t := range p.SuggestedTasks {
		fmt.Printf("\nCreate task [%d] %q? [y/n] ", i+1, t)

		for scanner.Scan() {
			input := strings.ToLower(strings.TrimSpace(scanner.Text()))
			switch input {
			case "y", "yes":
				approvedTasks = append(approvedTasks, i)
				goto nextTask
			case "n", "no":
				goto nextTask
			default:
				fmt.Print("[y/n] ")
			}
		}
	nextTask:
	}

	return approvedKB, approvedTasks
}
