package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

var taskAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a new task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.Join(args, " ")
		desc, _ := cmd.Flags().GetString("desc")
		priority, _ := cmd.Flags().GetInt("priority")
		sessionFlag, _ := cmd.Flags().GetInt64("session")

		var sessionID *int64
		if cmd.Flags().Changed("session") {
			sessionID = &sessionFlag
		} else {
			// Auto-attach to active session if exists
			sess, _ := appStore.ActiveSession()
			if sess != nil {
				sessionID = &sess.ID
			}
		}

		task, err := appStore.CreateTask(title, desc, sessionID, priority)
		if err != nil {
			return err
		}
		fmt.Printf("Task #%d created: %s\n", task.ID, task.Title)
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := store.TaskFilter{}

		if s, _ := cmd.Flags().GetString("status"); s != "" {
			status := model.TaskStatus(s)
			filter.Status = &status
		}
		if cmd.Flags().Changed("session") {
			sid, _ := cmd.Flags().GetInt64("session")
			filter.SessionID = &sid
		}

		tasks, err := appStore.ListTasks(filter)
		if err != nil {
			return err
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTITLE\tSTATUS\tPRIORITY\tSESSION")
		for _, t := range tasks {
			sessStr := "-"
			if t.SessionID != nil {
				sessStr = fmt.Sprintf("#%d", *t.SessionID)
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\n", t.ID, t.Title, t.Status, t.Priority, sessStr)
		}
		return w.Flush()
	},
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		// Handle status change separately (with event)
		if cmd.Flags().Changed("status") {
			s, _ := cmd.Flags().GetString("status")
			if err := appStore.UpdateTaskStatus(id, model.TaskStatus(s)); err != nil {
				return err
			}
		}

		var title, desc *string
		var priority *int
		if cmd.Flags().Changed("title") {
			v, _ := cmd.Flags().GetString("title")
			title = &v
		}
		if cmd.Flags().Changed("desc") {
			v, _ := cmd.Flags().GetString("desc")
			desc = &v
		}
		if cmd.Flags().Changed("priority") {
			v, _ := cmd.Flags().GetInt("priority")
			priority = &v
		}

		if title != nil || desc != nil || priority != nil {
			if err := appStore.UpdateTask(id, title, desc, priority); err != nil {
				return err
			}
		}

		fmt.Printf("Task #%d updated.\n", id)
		return nil
	},
}

var taskDoneCmd = &cobra.Command{
	Use:   "done <id>",
	Short: "Mark a task as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		if err := appStore.UpdateTaskStatus(id, model.TaskDone); err != nil {
			return err
		}
		fmt.Printf("Task #%d marked as done.\n", id)
		return nil
	},
}

func taskFilterForSession(sessionID int64) store.TaskFilter {
	return store.TaskFilter{SessionID: &sessionID}
}

func init() {
	taskAddCmd.Flags().String("desc", "", "Task description")
	taskAddCmd.Flags().Int("priority", 0, "Priority (0=none, 1=low, 2=medium, 3=high)")
	taskAddCmd.Flags().Int64("session", 0, "Session ID (default: active session)")

	taskListCmd.Flags().String("status", "", "Filter by status (todo, in_progress, done, cancelled)")
	taskListCmd.Flags().Int64("session", 0, "Filter by session ID")

	taskUpdateCmd.Flags().String("title", "", "New title")
	taskUpdateCmd.Flags().String("desc", "", "New description")
	taskUpdateCmd.Flags().String("status", "", "New status")
	taskUpdateCmd.Flags().Int("priority", 0, "New priority")

	taskCmd.AddCommand(taskAddCmd, taskListCmd, taskUpdateCmd, taskDoneCmd)
	rootCmd.AddCommand(taskCmd)
}
