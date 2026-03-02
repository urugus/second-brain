# サブエージェント依頼書: P1-003（CLI/MCP想起操作）

## 目的
ユーザーとエージェントが明示的に想起を実行できる入口を追加する。

## 担当
- 推奨担当: サブエージェントD
- チケット: `P1-003`

## 前提
- `P1-002` が取り込み済みで `RecallNote` API が利用可能であること。

## 必読
1. [Phase 1 実装チケット分解](/Users/urugus/dev/second-brain/docs/phase1-implementation-tickets.ja.md)
2. [Phase 0 設計仕様](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)

## 作業範囲
- 変更してよい:
  - `cmd/note.go`（または新規 `cmd/memory.go`）
  - `internal/mcp/note.go`（または新規 `internal/mcp/memory.go`）
  - `internal/mcp/server.go`
  - `internal/mcp/server_test.go`
- 変更禁止:
  - DBマイグレーション関連
  - `internal/consolidation/`, `internal/sync/`

## 実装要件
1. CLI 追加
- 例: `sb note recall <id>`
- 実行結果として `strength` 変化（before/after）を表示

2. MCP ツール追加
- 例: `recall_note`
- 入力: `note_id`（必須）
- 出力: 更新後の想起関連情報（strength, recall_count, last_recalled_at）

3. エラー処理
- 存在しない note_id
- 不正入力
- Store エラー

4. テスト
- CLI 相当の処理経路
- MCP `recall_note` 正常系/異常系

## 受け入れ条件（DoD）
- CLI で想起操作可能
- MCP でも同等操作可能
- `make test` 成功

## 実行コマンド
```bash
make test
go test ./internal/mcp -run Recall -v
```
