# サブエージェント依頼書: P1-002（Store記憶強度ロジック）

## 目的
`RecallNote` と `DecayMemories` を Store 層に実装し、想起強化と時間減衰を動作させる。

## 担当
- 推奨担当: サブエージェントC
- チケット: `P1-002`

## 前提
- `P1-001` が取り込み済みで、`notes` 拡張カラムが存在すること。

## 必読
1. [Phase 0 設計仕様](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)
2. [Phase 1 実装チケット分解](/Users/urugus/dev/second-brain/docs/phase1-implementation-tickets.ja.md)

## 作業範囲
- 変更してよい:
  - `internal/store/note.go`
  - `internal/model/note.go`
  - `internal/store/store_test.go`（または新規 `internal/store/memory_test.go`）
- 変更禁止:
  - `cmd/`
  - `internal/mcp/`

## 実装要件
1. 新規 API
- `RecallNote(id int64, now time.Time, context string) error`
- `DecayMemories(now time.Time) (affected int, error)`

2. `RecallNote` 実装
- `strength` を仕様式で上昇
- `recall_count` を +1
- `last_recalled_at` を更新
- 対象 note が存在しない場合はエラー

3. `DecayMemories` 実装
- `last_recalled_at` か `updated_at` を基準に経過時間を計算
- 指数減衰で `strength` を更新
- 下限 `min_strength` を下回らない

4. テスト
- 想起で `strength` が増える
- 減衰で `strength` が減る
- 下限保持
- 時刻固定で決定的に再現

## 受け入れ条件（DoD）
- `make test` 成功
- `RecallNote/DecayMemories` のテスト成功
- 既存 Note CRUD テストが退行しない

## 実行コマンド
```bash
make test
go test ./internal/store -run "Recall|Decay|Note|Memory" -v
```
