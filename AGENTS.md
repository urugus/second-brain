# Repository Guidelines

## Project Structure & Module Organization
This repository is a Go CLI application (`sb`) built with Cobra.
- `main.go`: application entrypoint.
- `cmd/`: CLI command definitions (`task`, `note`, `session`, `sync`, `mcp`, etc.).
- `internal/model/`: domain models and shared types.
- `internal/store/`: SQLite persistence and migrations.
- `internal/kb/`: knowledge-base file operations.
- `internal/sync/`, `internal/consolidation/`, `internal/mcp/`, `internal/adapter/`: feature services and integrations.
- `testdata/`: test fixtures (sample knowledge files).
- `knowledge/`: placeholder KB directory in repo (`.gitkeep`).
- `dist/`: generated release binaries from cross-builds.

## Build, Test, and Development Commands
Use `make` targets as the default workflow:
- `make build`: build local `sb` binary with version ldflags.
- `make test`: run all unit tests (`go test ./...`).
- `make lint`: run static checks (`go vet ./...`).
- `make build-all`: cross-build binaries into `dist/`.
- `make clean`: remove local build artifacts.

For iterative CLI development, run commands directly:
```bash
go run . --help
go run . task --help
go run . --db /tmp/brain.db --kb-dir /tmp/knowledge note list
```

## Coding Style & Naming Conventions
Follow standard Go conventions:
- Format with `gofmt` (tabs for indentation, gofmt-import order).
- Keep package names short and lowercase (`store`, `sync`, `mcp`).
- Use `PascalCase` for exported identifiers and `camelCase` for internal helpers.
- Keep feature alignment across layers when possible (for example: `cmd/task.go`, `internal/store/task.go`, `internal/model/task.go`).

## Testing Guidelines
- Place tests next to source files using `*_test.go`.
- Prefer deterministic tests using `t.TempDir()` for DB/filesystem setup.
- Use table-driven tests for pure logic and edge cases.
- Add or update regression tests for behavior changes (especially sync, store, and MCP paths).
- Run `make test` before opening a PR.

## Commit & Pull Request Guidelines
Commit subjects in history are short, imperative, and capitalized (example: `Add MCP server for Claude Code integration`).
- Keep commits focused on one logical change.
- PRs should include: purpose, key changes, and verification steps run locally.
- Include issue links when applicable.
- If CLI behavior changes, provide example commands and expected output.

## Security & Configuration Tips
Runtime data defaults to `~/.second-brain` and may contain sensitive personal notes.
- Do not commit local SQLite DBs or private knowledge content.
- Use `--db` and `--kb-dir` to isolate experiments in temporary directories.
