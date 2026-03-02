# Phase 0 設計仕様: 記憶アーキテクチャ固定

## 1. 目的
Phase 1 以降の実装で仕様ぶれを起こさないために、以下を固定する。

- 記憶データの責務分離（作業記憶/エピソード記憶/意味記憶）
- DB スキーマ拡張方針（v4）
- 強化・忘却・連想更新の計算規則
- 移行・ロールバック・評価指標

この文書は「実装前の合意仕様」であり、Phase 1 以降の PR は本仕様との差分を明記する。

## 2. 記憶モデル

### 2.1 作業記憶（短期）
- 主体: `sessions`（active セッション） + 直近の `tasks` / `notes`
- 特徴: 更新頻度高、寿命短
- 正本: DB

### 2.2 エピソード記憶（出来事）
- 主体: `events`（時系列の行動ログ）
- 特徴: 監査・再構成に利用
- 正本: DB

### 2.3 意味記憶（長期知識）
- 主体: `knowledge/**/*.md` の KB 本文
- 特徴: 人間可読、差分レビュー可能
- 正本: Markdown（索引は DB）

## 3. DB v4 スキーマ提案

## 3.1 `notes` 拡張カラム
- `strength REAL NOT NULL DEFAULT 0.30 CHECK (strength >= 0 AND strength <= 1)`
- `decay_rate REAL NOT NULL DEFAULT 0.015 CHECK (decay_rate > 0 AND decay_rate <= 1)`
- `salience REAL NOT NULL DEFAULT 0.50 CHECK (salience >= 0 AND salience <= 1)`
- `recall_count INTEGER NOT NULL DEFAULT 0 CHECK (recall_count >= 0)`
- `last_recalled_at TEXT`

補足:
- `strength` は想起優先度の中核値
- `decay_rate` は日単位の減衰係数

### 3.2 新規テーブル: `memory_edges`
ノート間の連想を重み付きで保持する。

```sql
CREATE TABLE IF NOT EXISTS memory_edges (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  from_note_id      INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  to_note_id        INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  weight            REAL NOT NULL DEFAULT 0.10 CHECK (weight > 0 AND weight <= 1),
  evidence          TEXT NOT NULL DEFAULT '',
  reinforced_count  INTEGER NOT NULL DEFAULT 1 CHECK (reinforced_count >= 1),
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  CHECK (from_note_id <> to_note_id),
  UNIQUE (from_note_id, to_note_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_edges_from ON memory_edges(from_note_id);
CREATE INDEX IF NOT EXISTS idx_memory_edges_to ON memory_edges(to_note_id);
```

### 3.3 新規テーブル: `kb_index`
Markdown 本文の索引メタを保持する。

```sql
CREATE TABLE IF NOT EXISTS kb_index (
  path         TEXT PRIMARY KEY,
  content_hash TEXT NOT NULL,
  title        TEXT NOT NULL DEFAULT '',
  tags         TEXT NOT NULL DEFAULT '',
  token_count  INTEGER NOT NULL DEFAULT 0,
  updated_at   TEXT NOT NULL,
  synced_at    TEXT NOT NULL
);
```

### 3.4 新規テーブル: `memory_recall_log`
強化挙動を評価するための想起ログ。

```sql
CREATE TABLE IF NOT EXISTS memory_recall_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  note_id       INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  context       TEXT NOT NULL DEFAULT '',
  score_before  REAL NOT NULL,
  score_after   REAL NOT NULL,
  recalled_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memory_recall_note_time
  ON memory_recall_log(note_id, recalled_at);
```

## 4. 更新規則（固定ルール）

### 4.1 初期値計算（ノート作成時）
- `salience = clamp(0.35 + source_bonus + tag_bonus + session_bonus, 0, 1)`
- `source_bonus`: `manual=0.15`, `sync=0.05`, その他 `0.00`
- `tag_bonus`: `min(0.20, len(tags) * 0.03)`
- `session_bonus`: セッション紐付けありで `0.05`
- `strength` 初期値: `clamp(0.20 + 0.50 * salience, 0, 1)`

### 4.2 想起時の強化
- `delta = alpha * salience * (1 - strength)`
- `strength' = clamp(strength + delta, 0, 1)`
- 推奨 `alpha = 0.25`
- `recall_count += 1`, `last_recalled_at = now`

### 4.3 減衰
- `dt_days = elapsed_hours / 24`
- `strength' = max(min_strength, strength * exp(-decay_rate * dt_days))`
- 推奨 `min_strength = 0.05`

### 4.4 連想エッジ更新
- 既存エッジ: `weight' = clamp(weight + beta * (1 - weight), 0, 1)`
- 新規エッジ: `weight = clamp(0.18 * ((salience_a + salience_b) / 2), 0.05, 0.50)`
- 推奨 `beta = 0.12`

## 5. 想起スコア（Phase 2 実装指針）

```text
recall_score =
  0.45 * text_match +
  0.30 * strength +
  0.15 * edge_proximity +
  0.10 * recency_boost
```

補足:
- Phase 1 は `text_match + strength` までで開始可能
- Phase 2 で `edge_proximity` を有効化

## 6. マイグレーションとロールバック方針

### 6.1 マイグレーション（前進）
1. `migrateV4` を追加（`notes` 拡張 + 新規3テーブル）
2. 既存 `notes` に対して backfill を実行
3. 索引再構築ジョブで `kb_index` を初期化

### 6.2 後方互換
- 新カラムは全てデフォルト付きで追加
- 既存 CLI/MCP API は非互換変更を入れない

### 6.3 ロールバック
- 物理的な `DROP COLUMN/TABLE` はしない
- Feature Flag を `OFF` にして新ロジックを停止
- 新カラム/テーブルは未使用状態で残置

## 7. Feature Flag
- `SB_FEATURE_MEMORY_MODEL=0|1`（strength/decay 更新の有効化）
- `SB_FEATURE_MEMORY_EDGES=0|1`（連想エッジ更新の有効化）
- `SB_FEATURE_KB_INDEX=0|1`（KB 索引同期の有効化）

## 8. 評価指標（KPI）

### 8.1 品質
- 重複ノート率
- KB 更新の手戻り率（更新後に短期間で再更新された割合）
- 関連想起のヒット率（上位 k 件に目的ノートが含まれる割合）

### 8.2 学習挙動
- 想起後 24h の再利用率
- `strength` 分布の安定性（極端な 0/1 偏りの監視）
- 減衰ジョブ後の再強化回復速度

### 8.3 運用
- sync/consolidate の失敗率
- 追加ストレージ増分（DB サイズ増加）
- 主要クエリの P95 レイテンシ

## 9. 受け入れ条件（Phase 0 完了基準）
- 本仕様を基準に Phase 1 実装チケットを分割可能
- DB/Markdown の正本境界に曖昧さがない
- 移行とロールバック手順が文書化済み
- KPI の採取方法が定義済み

## 10. Phase 1 への引き渡し
- 本文書を唯一の仕様参照とする
- 実装差分は PR テンプレートで「仕様との差分」欄を必須化する
