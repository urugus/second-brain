# Phase 1 実装チケット分解（サブエージェント委譲用）

## 前提
- 参照仕様: [phase0-memory-architecture-spec.ja.md](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)
- 対象フェーズ: Phase 1（記憶強度と忘却）
- 完了条件:
  - 想起で `strength` が上がる
  - 減衰ジョブで `strength` が下がる
  - 時間依存テストが再現可能

## 依存関係
1. `P1-001` (migration) 完了
2. `P1-002` (store) 着手
3. `P1-003` (CLI/MCP) 着手
4. `P1-004` (test hardening) で統合検証

---

## P1-001: DBマイグレーション実装

### 実行パケット
- サブエージェント依頼書: [subagent-p1-001-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-001-brief.ja.md)

### 目的
`notes` 拡張カラムと Phase 1 で必要な最小テーブルを追加し、既存データを安全に移行する。

### 変更範囲
- `internal/store/store.go`
- 必要に応じて `internal/model/note.go`

### 実装要件
- `migrateV4` を追加し、以下を実施:
  - `notes` に `strength/decay_rate/salience/recall_count/last_recalled_at` を追加
  - 既存 `notes` の backfill（デフォルト値）を実行
  - 既存データで移行失敗しないこと
- 非破壊（既存 CLI/MCP の動作に影響しない）

### 受け入れ条件
- 新規DB作成時と既存DB移行時の双方で起動成功
- `make test` が通る

---

## P1-002: Store層の記憶強度ロジック実装

### 実行パケット
- サブエージェント依頼書: [subagent-p1-002-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-002-brief.ja.md)

### 目的
想起時強化と時間減衰を Store API として実装する。

### 変更範囲
- `internal/store/note.go`
- `internal/store/store_test.go`（または専用 `internal/store/memory_test.go`）
- `internal/model/note.go`

### 実装要件
- 新規関数:
  - `RecallNote(id int64, now time.Time, context string) error`
  - `DecayMemories(now time.Time) (affected int, error)`
- `RecallNote`:
  - `strength` を仕様式に従って増加
  - `recall_count` 増分
  - `last_recalled_at` 更新
- `DecayMemories`:
  - 経過時間に応じて `strength` を減衰
  - 下限 `min_strength` を下回らない

### 受け入れ条件
- 想起で `strength` が単調増加する
- 減衰で `strength` が単調減少する（ただし下限保持）
- 時刻固定テストで決定的に再現できる

---

## P1-003: CLI/MCP 想起操作の追加

### 実行パケット
- サブエージェント依頼書: [subagent-p1-003-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-003-brief.ja.md)

### 目的
ユーザー/エージェントが明示的に想起をトリガーできる入口を追加する。

### 変更範囲
- `cmd/note.go`（または新規 `cmd/memory.go`）
- `internal/mcp/note.go` または新規 `internal/mcp/memory.go`
- `internal/mcp/server.go`

### 実装要件
- CLI:
  - 例: `sb note recall <id>`
  - 実行結果に `strength` 変化を表示
- MCP:
  - 例: `recall_note` ツールを追加
  - 呼び出し時に Store の `RecallNote` を実行
- 既存 `note add/list/show` の互換性維持

### 受け入れ条件
- CLI で想起実行できる
- MCP からも同等操作が可能
- エラーハンドリング（存在しない ID 等）が一貫

---

## P1-004: テスト強化・回帰確認

### 実行パケット
- サブエージェント依頼書: [subagent-p1-004-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-004-brief.ja.md)

### 目的
Phase 1 変更による退行を防ぎ、今後の Phase 2 への足場を固める。

### 変更範囲
- `internal/store/*_test.go`
- `internal/mcp/server_test.go`
- 必要に応じて `cmd/*` の結合テスト

### 実装要件
- 追加テスト:
  - マイグレーション idempotency（v4 含む）
  - `RecallNote` 正常系/異常系
  - `DecayMemories` の境界条件（0件、閾値、下限）
  - CLI/MCP の想起操作
- 既存テスト破壊がないこと

### 受け入れ条件
- `make test` 成功
- 推奨追加確認:
```bash
go test ./internal/store -run "Memory|Recall|Decay|Migrate" -v
go test ./internal/mcp -run Recall -v
```

---

## サブエージェント割り当て案

1. サブエージェントB: `P1-001`
2. サブエージェントC: `P1-002`
3. サブエージェントD: `P1-003`
4. サブエージェントE: `P1-004`（最後にリベースして統合）

## マージ順
1. `P1-001`
2. `P1-002`
3. `P1-003`
4. `P1-004`

`P1-003` は `P1-002` に依存。`P1-004` は全PR取り込み後に最終調整。
