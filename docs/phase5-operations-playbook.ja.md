# Phase 5 運用プレイブック（メトリクス/パラメータ/ロールバック）

## 1. KPI 可視化

運用 KPI は次のコマンドで確認する。

```bash
sb sync metrics --days 14
```

出力指標:
- Duplicate note rate（重複ノート率）
- Useful task generation rate（有用タスク生成率）
- KB rework rate（KB 手戻り率）

## 2. 外部化されたパラメータ

以下の環境変数で運用時チューニングを行う。

| 変数 | デフォルト | 用途 |
|---|---:|---|
| `SB_SLEEP_THRESHOLD` | `10` | sleep consolidation 発火閾値 |
| `SB_SYNC_PREDICTION_WINDOW` | `5` | sync 予測に使う履歴件数 |
| `SB_PRIORITY_ADJUST_LIMIT` | `5` | 優先度自動補正の対象タスク上限 |
| `SB_TASK_PRIORITY_MAX` | `5` | タスク優先度の上限 |
| `SB_SLEEP_REPLAY_ALPHA` | `0.18` | sleep replay による強度更新係数 |
| `SB_SLEEP_DUPLICATE_REPLAY_WEIGHT` | `0.35` | 重複ノートの replay 重み |
| `SB_MEMORY_EDGE_AUTOLINK_WEIGHT` | `0.12` | consolidation 時に自動生成する memory edge の重み |
| `SB_MEMORY_EDGE_AUTOLINK_MAX_PAIRS` | `24` | 1 回の consolidation で自動生成するノート対の上限 |
| `SB_MEMORY_EDGE_CREATE_AUTOLINK_WEIGHT` | `0.20` | note 作成時の自動生成 memory edge 重み係数 |
| `SB_MEMORY_EDGE_CREATE_AUTOLINK_MIN_SCORE` | `0.34` | note 作成時の自動生成 memory edge の最小スコア閾値 |
| `SB_MEMORY_EDGE_CREATE_AUTOLINK_CANDIDATES` | `80` | note 作成時の類似候補探索件数 |
| `SB_MEMORY_EDGE_CREATE_AUTOLINK_MAX_LINKS` | `3` | note 1件作成あたりの自動生成リンク上限 |
| `SB_MEMORY_EDGE_DECAY_RATE` | `0.010` | sync 実行時に適用する memory edge 減衰率 |
| `SB_MEMORY_EDGE_MIN_WEIGHT` | `0.02` | memory edge 減衰後の最小重み |
| `SB_MEMORY_EDGE_FEEDBACK_ALPHA` | `0.12` | recall 時の context 一致 edge 強化係数 |
| `SB_MEMORY_EDGE_FEEDBACK_DECAY` | `0.05` | recall 時の context 不一致 edge 減衰率 |
| `SB_MEMORY_EDGE_FEEDBACK_MAX_EDGES` | `10` | recall 1回でフィードバック更新する edge 数上限 |
| `SB_METRICS_WINDOW_DAYS` | `14` | `sync metrics` の既定集計期間 |

## 3. Feature Flag とロールバック

段階ロールバックは feature flag を OFF にして行う。

| 変数 | 既定 | OFF 時の挙動 |
|---|---|---|
| `SB_FEATURE_PREDICTION_LEARNING` | `1` | prediction error 記録と優先度自動補正を停止 |
| `SB_FEATURE_SLEEP_REPLAY` | `1` | sleep replay の dedupe/強度更新を停止（従来の consolidated_at 更新のみ） |
| `SB_FEATURE_MEMORY_EDGE_AUTOLINK` | `1` | consolidation 時の note 間自動リンク生成を停止 |
| `SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK` | `0` | note 作成時の自動リンク生成を有効化/停止 |
| `SB_FEATURE_MEMORY_EDGE_DECAY` | `1` | sync 実行時の memory edge 減衰を停止 |
| `SB_FEATURE_MEMORY_EDGE_FEEDBACK` | `1` | recall 時の memory edge フィードバック学習を停止 |

### 緊急ロールバック手順
1. 実行環境で以下を設定する。
   - `SB_FEATURE_PREDICTION_LEARNING=0`
   - `SB_FEATURE_SLEEP_REPLAY=0`
   - `SB_FEATURE_MEMORY_EDGE_AUTOLINK=0`
   - `SB_FEATURE_MEMORY_EDGE_CREATE_AUTOLINK=0`
   - `SB_FEATURE_MEMORY_EDGE_DECAY=0`
   - `SB_FEATURE_MEMORY_EDGE_FEEDBACK=0`
2. `sb sync run` を 1 回実行し、処理が継続可能か確認する。
3. `sb sync metrics --days 7` で KPI の急変有無を確認する。
4. 必要に応じてパラメータを段階的に戻し、feature flag を再有効化する。
