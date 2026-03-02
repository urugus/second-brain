package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommandExecutor abstracts subprocess execution for testability.
// All CLI-based agent adapters (Claude Code, Codex, etc.) use this interface.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DefaultExecutor runs real subprocesses.
type DefaultExecutor struct{}

func (e *DefaultExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	// Clear CLAUDECODE env var to allow launching claude from within a Claude Code session
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("command %q failed: %w", name, err)
	}
	return output, nil
}
