package cmd

import (
	"context"
	"os/signal"
	"syscall"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	sbmcp "github.com/urugus/second-brain/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server operations",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server on stdio",
	Long: `Start a Model Context Protocol server communicating via stdin/stdout.
Configure Claude Code to use this as an MCP server to interact with your second brain.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		server := sbmcp.New(appStore, appKB)
		return server.Run(ctx, &gomcp.StdioTransport{})
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}
