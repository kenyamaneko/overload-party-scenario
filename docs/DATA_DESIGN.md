# scenario スキーマ - データ設計

> **DDL の SSoT:** `db/schema.sql`

## 設計概要

scenario スキーマはストーリーエピソードのマスターデータとプレイヤーの進行状況を管理する。各エピソードは陣営に紐づき、アンロック条件（レベル・前提エピソード・必要陣営）を持つ。スクリプト本体は GCS に配置し、パステンプレートを DB で管理する。

プレイヤーのレベルと所有陣営は account が SSoT であり、scenario はスキーマを持たずアンロック判定のたびに account の API から取得する。表示名・所持陣営・オンボード進行状態も同様に account 側の SSoT で、scenario は完了フラグ (player_onboarding) とスクリプトだけを保持する。

---

## テーブル構成

### scenario_episodes

エピソードマスター。

- **PK:** `episode_id` (VARCHAR(50))
- **INDEX:** `idx_scenario_episodes_sort` ON `(sort_order)`
- **TRIGGER:** `updated_at` 自動更新
- **CHECK:** `category IN ('main', 'side', 'event')`
- **CHECK:** `faction IS NULL OR faction IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')`

<!-- BEGIN GENERATED: scenario_episodes -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `episode_id` | VARCHAR(50) | No | エピソードID（例: she_ep1, final） |
| `category` | VARCHAR(20) | No | エピソード種別 (main / side / event) |
| `faction` | VARCHAR(20) | Yes | 所属陣営（NULL: 全陣営共通） |
| `episode_number` | BIGINT | No | 陣営内の章番号 |
| `title_ja` | VARCHAR(200) | No | 日本語タイトル |
| `title_en` | VARCHAR(200) | No | 英語タイトル |
| `required_level` | BIGINT | No | アンロックに必要なレベル |
| `required_episodes` | TEXT[] | No | アンロックに必要な完了済みエピソード |
| `script_path` | VARCHAR(500) | No | スクリプトパステンプレート（{lang} を言語コードに置換） |
| `thumbnail_path` | VARCHAR(500) | Yes | サムネイル画像パス |
| `sort_order` | BIGINT | No | 表示順 |
| `is_active` | BOOLEAN | No | 公開フラグ |
| `created_at` | TIMESTAMPTZ | No | 作成日時 |
| `updated_at` | TIMESTAMPTZ | No | 更新日時 |
<!-- END GENERATED: scenario_episodes -->

**設計判断:**
- `required_episodes` を TEXT 配列にしているのは、前提エピソードの数が可変であり、別テーブルに正規化するほどの複雑さがないため。チェックはアプリ層で `player_story_progress` と突き合わせる
- `script_path` にテンプレートを持たせることで、多言語対応を DB 変更なしで実現する

### episode_required_factions

エピソードのアンロックに必要な陣営の中間テーブル。

- **PK:** `(episode_id, faction_id)`
- **FK:** `episode_id` → `scenario_episodes` (CASCADE)
- **CHECK:** `faction_id IN ('SHE', 'Tenki', 'Sugar', 'Tuners', 'Neutral')`

<!-- BEGIN GENERATED: episode_required_factions -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `episode_id` | VARCHAR(50) | No | エピソード参照 |
| `faction_id` | VARCHAR(20) | No | 必要陣営 |
<!-- END GENERATED: episode_required_factions -->

**設計判断:**
- `scenario_episodes.faction` は「このエピソードが属する陣営」であるのに対し、`episode_required_factions` は「このエピソードをプレイするために必要な陣営の所持」を表す。後者は複数陣営を要求するケースがあるため別テーブルに正規化している

### player_story_progress

プレイヤーの進行状況。完了記録のみ（進行中の状態はクライアント側で管理）。

- **PK:** `(player_id, episode_id)`
- **FK:** `episode_id` → `scenario_episodes` (RESTRICT)
- `player_id` は `account.players` へのクロススキーマ参照（FK 無し）

<!-- BEGIN GENERATED: player_story_progress -->
| カラム名 | 型 | Nullable | 説明 |
|---|---|---|---|
| `player_id` | UUID | No | 所有プレイヤー (cross-schema reference to account.players; app-level integrity, not enforced by FK) |
| `episode_id` | VARCHAR(50) | No | 完了したエピソードID |
| `completed_at` | TIMESTAMPTZ | No | 完了日時 |
<!-- END GENERATED: player_story_progress -->

**設計判断:**
- 完了記録は冪等（`ON CONFLICT DO NOTHING`）。同じエピソードを再読了してもレコードは増えない
- `episode_id` の FK に `ON DELETE RESTRICT` を使用しているのは、進行状況のあるエピソードを誤って削除することを防ぐため

---

## テーブル間リレーション

```
scenario_episodes (PK: episode_id)
  │
  ├── 1:N ── episode_required_factions (FK: episode_id, CASCADE)
  └── 1:N ── player_story_progress     (FK: episode_id, RESTRICT)

[account.players] ─ ─ ─ (cross-schema, app-level)
  │
  └── 1:N ── player_story_progress (PK: player_id, episode_id)
```

---

## インデックス戦略

| インデックス | 対象 | 用途 |
|---|---|---|
| `idx_scenario_episodes_sort` | `scenario_episodes(sort_order)` | エピソード一覧の表示順取得 |
