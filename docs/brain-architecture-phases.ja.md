# 脳構造近似ロードマップ（サブエージェント実装前提）

## 目的
`second-brain` を「記録ツール」から「学習する記憶システム」へ段階的に拡張する。  
人間の脳を厳密再現するのではなく、以下を機能近似する。

- 作業記憶と長期記憶の分離
- 想起で強化される記憶強度
- 時間経過による忘却
- 連想ネットワークによる想起
- 睡眠統合（replay/抽象化/重複統合）
- 予測誤差ベースの学習

## 運用ルール（重要）
- 実装は各フェーズごとにサブエージェントへ委譲する。
- 1フェーズ=1PRを原則にする（混在禁止）。
- 各フェーズで `make test` を必須にする。
- 破壊的変更は Feature Flag で段階リリースする。

---

## フェーズ一覧

| Phase | ゴール | 主な変更領域 | 完了条件 |
|---|---|---|---|
| 0 | 設計固定と計測導入 | `docs/`, `internal/model/` | 設計合意、指標定義、移行方針確定 |
| 1 | 記憶強度と忘却 | `internal/store/`, `internal/model/`, `cmd/` | 強度更新と減衰が動作、回帰テスト追加 |
| 2 | 連想ネットワーク | `internal/store/`, `internal/kb/`, `internal/mcp/` | 関連想起APIが利用可能 |
| 3 | 睡眠統合の高度化 | `internal/consolidation/`, `internal/sync/` | replay→統合→強度更新が自動実行 |
| 4 | 予測誤差学習と行動選択 | `internal/sync/`, `internal/consolidation/`, `cmd/` | 誤差ログと優先順位更新が動作 |
| 5 | 運用安定化 | 全体 | 指標安定、ロールバック手順確立 |

---

## 永続化方針決定表（DB vs Markdown）

### 判定ルール
- 機械的に集計・検索・結合する情報は DB を正本にする。
- 人間が継続的に読む知識本文は Markdown を正本にする。
- Markdown を正本にする場合でも、検索/重複判定に必要な索引は DB に持つ。

### 決定表（現時点の基準）

| 情報カテゴリ | 例 | 正本 | 理由 |
|---|---|---|---|
| セッション状態 | active/completed、開始終了時刻、summary | DB | トランザクション整合性が必要 |
| タスク | status/priority/session紐付け | DB | 更新頻度が高くクエリ主体 |
| ノート本文・メタ | content/tags/source | DB | 収集経路が多く一元管理が必要 |
| 記憶強度パラメータ | strength/decay_rate/recall_count/salience | DB | 数値更新と時系列処理が中心 |
| 連想エッジ | note間weight/evidence | DB | グラフ探索と重み更新が必要 |
| イベント履歴 | session.started/task.status_changed 等 | DB | 監査・再構成に必要 |
| 同期/統合ログ | sync_log/consolidation_log | DB | 運用監視・失敗解析が目的 |
| KB本文（長期知識） | `knowledge/**/*.md` の本文 | Markdown | 人間可読性・差分レビュー性が高い |
| KB索引メタ | hash/最終同期時刻/主要タグ（将来追加） | DB + Markdown | 本文はMarkdown、索引はDBが最適 |
| 設定値 | 閾値・重み係数・Feature Flag | DB（将来は設定ファイル併用可） | 実行時切替と追跡が必要 |

### 同期規約（SoTの扱い）
- DB正本の情報をMarkdownへ自動反映しない（重複正本を避ける）。
- Markdown正本のKB本文は、更新時にDB索引を再計算する。
- 競合時は「正本側を優先」し、非正本側は再生成する。

### 未決定事項（Phase 0で確定）
- KB索引メタをどの粒度でDBに保持するか（ファイル単位/セクション単位）。
- 設定値をDBのみで持つか、YAML/TOMLを併用するか。
- Markdown外部編集を検知する方式（mtime監視/ハッシュ比較）。

---

## Phase 0: 設計固定（実装前）

### 実行パケット
- 設計仕様: [phase0-memory-architecture-spec.ja.md](/Users/urugus/dev/second-brain/docs/phase0-memory-architecture-spec.ja.md)
- サブエージェント依頼書: [subagent-phase0-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-phase0-brief.ja.md)

### サブエージェント依頼内容
- 記憶モデル設計を確定する。
- DBマイグレーション方針（v4以降）を定義する。
- 評価指標を決める。

### 期待成果物
- 設計ドキュメント（ER図/テーブル定義/更新規則）
- 「強化」「忘却」「想起」の式またはルール
- 移行時の後方互換ポリシー
- 永続化方針決定表（DB/Markdown/併用）と競合解決規約

### 完了判定
- 既存機能（session/task/note/sync/consolidate）との衝突がない
- 移行手順がロールバック可能
- 新規データ種別が追加されても保存先判定に迷いがない

---

## Phase 1: 記憶強度と忘却

### 実行パケット
- 実装チケット分解: [phase1-implementation-tickets.ja.md](/Users/urugus/dev/second-brain/docs/phase1-implementation-tickets.ja.md)
- P1-001依頼書: [subagent-p1-001-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-001-brief.ja.md)
- P1-002依頼書: [subagent-p1-002-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-002-brief.ja.md)
- P1-003依頼書: [subagent-p1-003-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-003-brief.ja.md)
- P1-004依頼書: [subagent-p1-004-brief.ja.md](/Users/urugus/dev/second-brain/docs/subagent-p1-004-brief.ja.md)

### スコープ
- `notes` に記憶強度系カラムを追加
- 想起時に強度上昇、定期処理で減衰
- CLI/MCPで想起操作を可能にする

### 最小実装案
- 新規カラム候補:
  - `strength` (REAL)
  - `decay_rate` (REAL)
  - `last_recalled_at` (TEXT)
  - `recall_count` (INTEGER)
  - `salience` (REAL)
- 新規ストア関数:
  - `RecallNote(id)`
  - `DecayMemories(now)`

### 完了判定
- 想起で `strength` が上がる
- 減衰ジョブで `strength` が下がる
- テストで時間依存の挙動を再現できる

### 検証
```bash
make test
go test ./internal/store -run Memory -v
```

---

## Phase 2: 連想ネットワーク

### スコープ
- `memory_edges`（ノート間関連）を追加
- 関連想起APIを実装
- `kb search` と組み合わせたハイブリッド想起を追加

### 最小実装案
- 新規テーブル候補:
  - `memory_edges(from_note_id, to_note_id, weight, updated_at, evidence)`
- 新規関数:
  - `LinkNotes(a, b, weight, evidence)`
  - `RelatedNotes(seedID, depth, topK)`

### 完了判定
- seed note から関連ノートを重み順に取得できる
- エッジ更新の回帰テストがある

### 検証
```bash
make test
go test ./internal/store ./internal/mcp -run Related -v
```

---

## Phase 3: 睡眠統合の高度化

### スコープ
- 既存 `SleepConsolidate` を拡張
- replay（再生）→抽象化→KB統合→強度更新
- 重複ノート統合ロジックを導入

### 完了判定
- 未統合ノート群から重複削減されたKB更新が生成される
- 処理後に `consolidated_at` と `strength` が一貫更新される

### 検証
```bash
make test
go test ./internal/consolidation -run Sleep -v
```

---

## Phase 4: 予測誤差学習 + 行動選択

### スコープ
- 予測（次に必要な情報/タスク）を記録
- 実測との差分（prediction error）を学習に反映
- タスク優先度の自動補正

### 完了判定
- 誤差ログが蓄積される
- 誤差に応じて優先度または想起順序が変化する

### 検証
```bash
make test
go test ./internal/sync ./internal/consolidation -run Prediction -v
```

---

## Phase 5: 運用安定化

### スコープ
- メトリクス可視化
- チューニングパラメータ外部化
- ロールバック手順確立

### 完了判定
- 次のKPIが2週間安定:
  - 重複ノート率低下
  - 有用タスク生成率向上
  - KB更新の手戻り率低下

---

## サブエージェント依頼テンプレート（コピペ用）

```text
あなたは second-brain の実装サブエージェントです。
担当フェーズ: <Phase番号と名称>

目的:
<このフェーズのゴールを1-2行で記載>

作業範囲:
- 変更してよいファイル: <paths>
- 変更禁止: それ以外

必須要件:
1. 既存機能を壊さない
2. 単体テストを追加/更新する
3. make test を通す
4. 変更理由をPR説明に明記する

受け入れ条件:
- <DoDを列挙>

出力:
- 変更ファイル一覧
- 実装内容の要約
- 実行した検証コマンドと結果
```

---

## 直近の実行順（推奨）
1. Phase 0 をサブエージェントAへ依頼（設計固定）
2. Phase 1 をサブエージェントBへ依頼（DB + store + test）
3. Phase 1完了後に統合レビュー
4. 問題なければ Phase 2 へ進む
