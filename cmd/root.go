package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/kb"
	"github.com/urugus/second-brain/internal/store"
)

var (
	dbPath string
	kbDir  string

	appStore *store.Store
	appKB    *kb.KB
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".second-brain"
	}
	return filepath.Join(home, ".second-brain")
}

var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "Second Brain - personal knowledge management CLI",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "version" {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
		if err := os.MkdirAll(kbDir, 0o755); err != nil {
			return fmt.Errorf("failed to create knowledge directory: %w", err)
		}

		s, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		appStore = s

		appKB = kb.New(kbDir)
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if appStore != nil {
			appStore.Close()
		}
	},
}

func init() {
	dataDir := defaultDataDir()
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", filepath.Join(dataDir, "brain.db"), "SQLite database path")
	rootCmd.PersistentFlags().StringVar(&kbDir, "kb-dir", filepath.Join(dataDir, "knowledge"), "Knowledge base directory")
}

func Execute() error {
	return rootCmd.Execute()
}
