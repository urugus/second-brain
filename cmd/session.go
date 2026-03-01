package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/model"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage work sessions",
}

var sessionStartCmd = &cobra.Command{
	Use:   "start <title>",
	Short: "Start a new session",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.Join(args, " ")
		goal, _ := cmd.Flags().GetString("goal")

		sess, err := appStore.CreateSession(title, goal)
		if err != nil {
			return err
		}
		fmt.Printf("Session #%d started: %s\n", sess.ID, sess.Title)
		return nil
	},
}

var sessionEndCmd = &cobra.Command{
	Use:   "end",
	Short: "End the current active session",
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := appStore.ActiveSession()
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no active session")
		}

		summary, _ := cmd.Flags().GetString("summary")
		if err := appStore.EndSession(sess.ID, summary); err != nil {
			return err
		}
		fmt.Printf("Session #%d ended: %s\n", sess.ID, sess.Title)
		return nil
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		var statusFilter *model.SessionStatus
		if s, _ := cmd.Flags().GetString("status"); s != "" {
			status := model.SessionStatus(s)
			statusFilter = &status
		}

		sessions, err := appStore.ListSessions(statusFilter)
		if err != nil {
			return err
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tSTARTED")
		for _, s := range sessions {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", s.ID, s.Title, s.Status, s.StartedAt.Format("2006-01-02 15:04"))
		}
		return w.Flush()
	},
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show session details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid session ID: %s", args[0])
		}

		sess, err := appStore.GetSession(id)
		if err != nil {
			return err
		}

		fmt.Printf("Session #%d: %s\n", sess.ID, sess.Title)
		fmt.Printf("  Status:  %s\n", sess.Status)
		fmt.Printf("  Goal:    %s\n", sess.Goal)
		fmt.Printf("  Started: %s\n", sess.StartedAt.Format("2006-01-02 15:04"))
		if sess.EndedAt != nil {
			fmt.Printf("  Ended:   %s\n", sess.EndedAt.Format("2006-01-02 15:04"))
		}
		if sess.Summary != "" {
			fmt.Printf("  Summary: %s\n", sess.Summary)
		}

		// Show tasks
		tasks, _ := appStore.ListTasks(taskFilterForSession(id))
		if len(tasks) > 0 {
			fmt.Println("\n  Tasks:")
			for _, t := range tasks {
				fmt.Printf("    [%s] #%d %s\n", statusIcon(string(t.Status)), t.ID, t.Title)
			}
		}

		// Show notes
		notes, _ := appStore.ListNotes(noteFilterForSession(id))
		if len(notes) > 0 {
			fmt.Println("\n  Notes:")
			for _, n := range notes {
				content := n.Content
				if len(content) > 60 {
					content = content[:60] + "..."
				}
				fmt.Printf("    #%d %s\n", n.ID, content)
			}
		}

		return nil
	},
}

func statusIcon(status string) string {
	switch status {
	case "done":
		return "x"
	case "in_progress":
		return "~"
	case "cancelled":
		return "-"
	default:
		return " "
	}
}

func init() {
	sessionStartCmd.Flags().String("goal", "", "Session goal")
	sessionEndCmd.Flags().String("summary", "", "Session summary")
	sessionListCmd.Flags().String("status", "", "Filter by status (active, completed, abandoned)")

	sessionCmd.AddCommand(sessionStartCmd, sessionEndCmd, sessionListCmd, sessionShowCmd)
	rootCmd.AddCommand(sessionCmd)
}
