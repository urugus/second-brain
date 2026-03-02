# サブエージェント依頼書: P1-001（DBマイグレーション）

## 目的
Phase 1 の前提として、記憶強度モデルの永続化スキーマを安全に追加する。

## 担当
- 推奨担当: サブエージェントB
- チケット: `P1-001`

## 必読
1. [Phase 0 設計仕様](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)
2. [Phase 1 実装チケット分解](/Users/urugus/dev/second-brain/docs/phase1-implementation-tickets.ja.md)

## 作業範囲
- 変更してよい:
  - `internal/store/store.go`
  - `internal/model/note.go`（必要な場合のみ）
  - `internal/store/store_test.go`（移行テスト追加）
- 変更禁止:
  - `cmd/`
  - `internal/mcp/`
  - `internal/consolidation/`
  - `internal/sync/`

## 実装要件
1. `migrateV4` を追加する
- `notes` へ以下カラムを追加:
  - `strength REAL NOT NULL DEFAULT 0.30 CHECK (strength >= 0 AND strength <= 1)`
  - `decay_rate REAL NOT NULL DEFAULT 0.015 CHECK (decay_rate > 0 AND decay_rate <= 1)`
  - `salience REAL NOT NULL DEFAULT 0.50 CHECK (salience >= 0 AND salience <= 1)`
  - `recall_count INTEGER NOT NULL DEFAULT 0 CHECK (recall_count >= 0)`
  - `last_recalled_at TEXT`

2. マイグレーション列に `migrateV4` を接続する
- 既存の `v1-v3` に影響を与えないこと

3. 既存DB移行時の安全性を担保する
- 既存レコードに対して NOT NULL 制約違反が起きないこと
- `Open()` が既存DBでも成功すること

4. テストを追加/更新する
- 新規作成DBで v4 カラムが存在すること
- 既存DB（v3相当）からの再オープンで移行されること
- 2回目以降の `Open()` でも壊れないこと（idempotent）

## 受け入れ条件（DoD）
- `make test` が成功
- 追加テストで v4 カラム存在が確認できる
- 既存機能テスト（session/task/note/event）が退行しない

## 実行コマンド
```bash
make test
go test ./internal/store -run "Open|Migrate|Note|Session|Task" -v
```

## 返却フォーマット
1. 変更ファイル一覧
2. 実装概要（5行以内）
3. 追加/更新したテストケース
4. 実行コマンドと結果
5. 残課題（あれば）

## サブエージェント投入文（コピペ用）
```text
あなたは second-brain の実装サブエージェントです。P1-001（DBマイグレーション）を担当してください。

目的:
記憶強度モデルの永続化スキーマを v4 マイグレーションとして追加し、既存DBを安全に移行する。

必読:
- docs/phase0-memory-architecture-spec.ja.md
- docs/phase1-implementation-tickets.ja.md
- docs/subagent-p1-001-brief.ja.md

変更可能:
- internal/store/store.go
- internal/model/note.go（必要時）
- internal/store/store_test.go

必須:
1. migrateV4追加（notes拡張カラム追加）
2. migrations配列へ接続
3. 既存DB移行安全性の担保
4. migration idempotencyテスト追加
5. make test 成功

返却:
- 変更ファイル一覧
- 実装概要
- テスト追加内容
- 実行コマンド結果
```
