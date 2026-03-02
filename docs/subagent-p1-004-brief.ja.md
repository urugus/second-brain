# サブエージェント依頼書: P1-004（テスト強化・回帰確認）

## 目的
Phase 1 変更全体の回帰を防ぎ、Phase 2 へ進める品質ラインを作る。

## 担当
- 推奨担当: サブエージェントE
- チケット: `P1-004`

## 前提
- `P1-001`〜`P1-003` が取り込み済みであること。

## 必読
1. [Phase 1 実装チケット分解](/Users/urugus/dev/second-brain/docs/phase1-implementation-tickets.ja.md)

## 作業範囲
- 変更してよい:
  - `internal/store/*_test.go`
  - `internal/mcp/server_test.go`
  - 必要に応じて軽微な本体修正（テストを通す最小限）
- 変更禁止:
  - 機能仕様の拡張
  - 新規大規模機能の追加

## 実装要件
1. migration 回帰テスト
- v4 を含む `Open()` idempotency
- 既存DB移行の再現

2. memory ロジックテスト
- 想起強化
- 減衰
- 下限保持
- 境界条件（対象0件、存在しないID）

3. CLI/MCP 回帰
- `recall_note` の正常/異常系
- 既存 note/task/session 操作に退行がない

## 受け入れ条件（DoD）
- `make test` 成功
- 以下の推奨コマンドが成功

```bash
go test ./internal/store -run "Open|Migrate|Recall|Decay|Memory" -v
go test ./internal/mcp -run "Recall|Note|Session|Task" -v
```

## 返却フォーマット
1. 追加/更新テスト一覧
2. 失敗を再現したケースと修正内容
3. 実行コマンド結果
