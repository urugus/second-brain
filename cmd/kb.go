package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
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

func init() {
	kbCmd.AddCommand(kbListCmd, kbShowCmd, kbSearchCmd)
	rootCmd.AddCommand(kbCmd)
}
