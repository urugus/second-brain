package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/adapter"
	claudeAdapter "github.com/urugus/second-brain/internal/adapter/claude"
	"github.com/urugus/second-brain/internal/consolidation"
	sbsync "github.com/urugus/second-brain/internal/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync external sources into second brain",
}

var syncRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a sync now",
	Long: `Execute a sync by calling Claude with MCP tools to check external sources
and save important information to second-brain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		modelFlag, _ := cmd.Flags().GetString("model")
		executor := &adapter.DefaultExecutor{}
		svc := sbsync.NewService(appStore, executor, modelFlag)

		fmt.Println("Starting sync...")
		result, err := svc.Run(cmd.Context())
		if err != nil {
			return err
		}

		fmt.Printf("\nSync completed.\n")
		fmt.Printf("  Summary: %s\n", result.Summary)
		fmt.Printf("  Notes added: %d\n", result.NotesAdded)
		fmt.Printf("  Tasks added: %d\n", result.TasksAdded)
		fmt.Printf("  Prediction error: notes %.2f -> %d (%+.2f), tasks %.2f -> %d (%+.2f)\n",
			result.PredictedNotes, result.NotesAdded, result.NotesError,
			result.PredictedTasks, result.TasksAdded, result.TasksError,
		)
		if result.PriorityDelta != 0 {
			fmt.Printf("  Task priority adjusted: %+d (affected %d todo tasks)\n", result.PriorityDelta, result.AdjustedTasks)
		}
		if len(result.KBFilesUpdated) > 0 {
			fmt.Printf("  KB files updated: %v\n", result.KBFilesUpdated)
		}

		if err := trySleepConsolidate(cmd, modelFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Sleep consolidation error: %v\n", err)
		}

		return nil
	},
}

var syncEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable automatic sync via cron",
	RunE: func(cmd *cobra.Command, args []string) error {
		intervalStr, _ := cmd.Flags().GetString("interval")
		interval, err := time.ParseDuration(intervalStr)
		if err != nil {
			return fmt.Errorf("invalid interval %q: %w", intervalStr, err)
		}

		if err := sbsync.Enable(interval); err != nil {
			return err
		}
		fmt.Printf("Sync enabled: every %s\n", interval)
		return nil
	},
}

var syncDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable automatic sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := sbsync.Disable(); err != nil {
			return err
		}
		fmt.Println("Sync disabled.")
		return nil
	},
}

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		enabled, schedule, err := sbsync.IsEnabled()
		if err != nil {
			return err
		}
		if enabled {
			fmt.Printf("Cron: enabled (%s)\n", schedule)
		} else {
			fmt.Println("Cron: disabled")
		}

		latest, err := appStore.LatestSyncLog()
		if err != nil {
			fmt.Println("Last sync: never")
			return nil
		}
		fmt.Printf("Last sync: %s (%s)\n", latest.CreatedAt.Local().Format("2006-01-02 15:04"), latest.Status)
		if latest.OutputSummary != "" {
			fmt.Printf("  Summary: %s\n", latest.OutputSummary)
		}
		if latest.ErrorMessage != "" {
			fmt.Printf("  Error: %s\n", latest.ErrorMessage)
		}
		return nil
	},
}

var syncLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Show sync history",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")

		logs, err := appStore.ListSyncLogs(limit)
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			fmt.Println("No sync history.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tNOTES\tTASKS\tDURATION\tTIME")
		for _, l := range logs {
			fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%dms\t%s\n",
				l.ID, l.Status, l.NotesAdded, l.TasksAdded,
				l.DurationMs, l.CreatedAt.Local().Format("2006-01-02 15:04"))
		}
		return w.Flush()
	},
}

const defaultSleepThreshold = 10

func trySleepConsolidate(cmd *cobra.Command, modelFlag string) error {
	var opts []claudeAdapter.Option
	if modelFlag != "" {
		opts = append(opts, claudeAdapter.WithModel(modelFlag))
	}
	agent := claudeAdapter.New(opts...)

	svc := consolidation.NewService(appStore, appKB, agent)

	sleepResult, err := svc.SleepConsolidate(cmd.Context(), defaultSleepThreshold)
	if err != nil {
		return err
	}
	if sleepResult == nil {
		return nil
	}

	fmt.Printf("\nSleep consolidation triggered (%d notes processed).\n", sleepResult.NotesProcessed)
	if sleepResult.NotesReplayed > 0 && sleepResult.NotesReplayed != sleepResult.NotesProcessed {
		fmt.Printf("  Notes replayed after dedupe: %d (merged duplicates: %d)\n", sleepResult.NotesReplayed, sleepResult.DuplicatesMerged)
	}
	fmt.Printf("  Summary: %s\n", sleepResult.Summary)
	if len(sleepResult.KBFilesUpdated) > 0 {
		fmt.Printf("  KB files updated: %v\n", sleepResult.KBFilesUpdated)
	}
	if sleepResult.TasksCreated > 0 {
		fmt.Printf("  Tasks created: %d\n", sleepResult.TasksCreated)
	}
	return nil
}

func init() {
	syncRunCmd.Flags().String("model", "", "Claude model to use")
	syncEnableCmd.Flags().String("interval", "30m", "Sync interval (e.g. 30m, 1h, 6h)")
	syncLogCmd.Flags().Int("limit", 10, "Number of log entries to show")

	syncCmd.AddCommand(syncRunCmd, syncEnableCmd, syncDisableCmd, syncStatusCmd, syncLogCmd)
	rootCmd.AddCommand(syncCmd)
}
