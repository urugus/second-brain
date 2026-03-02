# サブエージェント依頼書: Phase 0（設計固定）

## 依頼先
- 担当: サブエージェントA
- フェーズ: Phase 0
- 目的: Phase 1 以降の実装仕様を固定し、実装着手可能な状態にする

## 必読ドキュメント
1. [脳構造近似ロードマップ](/Users/urugus/dev/second-brain/docs/brain-architecture-phases.ja.md)
2. [Phase 0 設計仕様](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)

## 作業範囲
- 変更してよい:
  - `docs/brain-architecture-phases.ja.md`
  - `docs/phase0-memory-architecture-spec.ja.md`
  - `docs/subagent-phase0-brief.ja.md`
- 変更禁止:
  - `cmd/`, `internal/`, `main.go`, `go.mod`, `go.sum`（コード変更禁止）

## タスク
1. Phase 0 設計仕様の穴を埋める
- 命名・型・制約が曖昧な箇所を明確化する
- 算式パラメータに根拠メモを付与する

2. 移行/ロールバック手順をチェックリスト化する
- 実行順
- 成功判定
- 失敗時の巻き戻し手順

3. KPI の採取方法を明記する
- どのテーブル/ログから集計するか
- 最低限の集計SQL例（擬似SQLでも可）

4. Phase 1 チケット分割案を作る
- 「マイグレーション」「store」「CLI/MCP」「テスト」の4分割で作成

## 完了条件（DoD）
- Phase 1 実装担当が追加質問なしで着手できる
- DB/Markdown の正本境界が一義的に読める
- ロールバック条件が明文化されている
- 仕様差分を判定できるチェック項目が存在する

## 検証
ドキュメント作業のみのためコードテストは不要。以下のみ実施:

```bash
rg -n "TODO|TBD|未決定" docs/phase0-memory-architecture-spec.ja.md docs/brain-architecture-phases.ja.md
```

`TODO/TBD/未決定` が残る場合は、残した理由を「未確定事項」節に集約すること。

## 出力フォーマット
1. 変更ファイル一覧
2. 追加・修正した仕様の要点（5行以内）
3. 未確定事項と、Phase 1 へ持ち越す判断理由
