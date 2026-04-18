# Scenario 機能仕様書

このドキュメントは scenario サービスがビジネス要件として満たすべき振る舞いを定義する。実装方法ではなく **何を保証するか** を記述する。テストはこの仕様に従っていることを確認する観点で書く。

関連ドキュメント:
- 内部動作・配線・本番運用設定: [ARCHITECTURE.md](ARCHITECTURE.md)
- HTTP エンドポイント契約: [API_REFERENCE.md](API_REFERENCE.md)
- DB スキーマ: [DATA_DESIGN.md](DATA_DESIGN.md)

---

## 1. サービス責務

scenario は以下の機能ドメインを所有する。

| 機能 | 主要な責務 |
|---|---|
| エピソードメタデータ管理 | 公開中のストーリーエピソード定義を保持し、プレイヤー向けに提示する |
| プレイヤー進行管理 | プレイヤーごとのエピソード完了記録を冪等に永続化する |
| 多言語スクリプト配信 | `{lang}` テンプレートで切り替えた外部ストア (GCS または local FS) からスクリプト本文を配信する |
| アンロック条件判定 | プレイヤーレベル・所有 faction・前提エピソード完了の複合 AND でエピソードのロック状態を算出する |
| 初期 faction 選択 hand-off | チュートリアル確定時に `faction-selected` Pub/Sub イベントを発行する |

scenario は **scenario スキーマの DB 行と Firestore `game_config` を唯一の真実とし**、account スキーマ (`players.level` / `player_factions`) は cross-schema read のみで扱う。account / card / gateway を直接呼び出さない。

### 非対象

| 機能 | 取り扱い |
|---|---|
| クライアント認証 | ClusterIP 到達性のみを信頼境界とし、URL 内の `playerId` を無条件で信頼する（gateway が Firebase Auth で確定した ID を forward する前提） |
| スクリプト本文の変換・レンダリング | ストアから読み出した文字列を変換せずそのまま返す |
| 言語別テキストの DB 保持 | `title_ja` / `title_en` のみ DB に持つ。本文は `{lang}` テンプレートでパス切替した外部ストアに委ねる |
| 言語フォールバック | 行わない。要求言語のスクリプトが欠けていれば 404 を返す（§7 参照） |
| `faction-selected` 以外のイベント発行 | 行わない |
| `faction-selected` の subscribe | 行わない。scenario は発信のみ、購読は account / card / gateway が担う |

---

## 2. ドメインモデル

### 2.1 ScenarioEpisode

エピソードマスター。`scenario.scenario_episodes` に永続化される。

| フィールド | 用途 |
|---|---|
| `episode_id` | PK、外部参照キー |
| `faction` | 所属陣営（NULL は全陣営共通） |
| `episode_number` | 陣営内の章番号 |
| `title_ja` / `title_en` | UI 表示用タイトル |
| `required_level` | アンロック条件：プレイヤーレベル下限 |
| `required_factions` (via `episode_required_factions`) | アンロック条件：所有を要する faction の集合 |
| `required_episodes` | アンロック条件：前提として完了を要するエピソード ID 集合 |
| `script_path` | 本文の外部パステンプレート（`stories/{lang}/she_ep1.ks` 形式） |
| `thumbnail_path` | サムネイル画像パス |
| `sort_order` | 一覧表示順序 |
| `is_active` | 公開フラグ。`false` のエピソードは一覧にも出ず、詳細 API も 404 |

### 2.2 PlayerStoryProgress

プレイヤーのエピソード完了履歴。`scenario.player_story_progress` に永続化される。

| フィールド | 用途 |
|---|---|
| `player_id` | PK 構成要素。account サービスの `players.player_id` への論理参照（FK は張らない） |
| `episode_id` | PK 構成要素 |
| `completed_at` | 完了日時 |

`(player_id, episode_id)` を PK とし、ON CONFLICT DO NOTHING で冪等に INSERT される。

### 2.3 StoryUnlockContext

アンロック判定用にリクエスト時点で pre-fetch するスナップショット。DB には保持しない。

| フィールド | ソース |
|---|---|
| `PlayerLevel` | `account.players.level` |
| `OwnedFactions` | `account.player_factions` を集合化 |
| `CompletedEpisodes` | `scenario.player_story_progress` を集合化 |

現実装は `players` / `player_factions` / `player_story_progress` を結合する単一クエリで取得する。account と scenario の DB 分離後は account クライアント経由に置換される予定。

### 2.4 LockReason

単一の未達アンロック条件を表す値オブジェクト。

| フィールド | 用途 |
|---|---|
| `type` | `"level"` / `"faction"` / `"episode"` のいずれか |
| `required` | 要求値（レベル値、faction ID、エピソード ID） |
| `current` | 現在値（レベル時のみ意味を持つ。faction / episode では省略） |

---

## 3. エピソード一覧取得 (`ListEpisodes`)

**入力**: `playerID`, `lang`（`ja` / `en`、省略時 `ja`）
**出力**: `[]EpisodeWithStatus`

### 仕様
1. `is_active = true` のエピソードを `sort_order` 昇順で取得
2. プレイヤーの `StoryUnlockContext`（level + 所有 faction + 完了済み episode）を 1 回だけ取得
3. 各エピソードについてアンロック条件を評価し、`is_unlocked` と `lock_reasons` を埋める
4. `is_completed` は `CompletedEpisodes` に含まれるかで決まる
5. `title` は `lang == "en"` なら `title_en`、それ以外は `title_ja`
6. ページング・絞り込みは行わない（全件返却）

副作用なし。

### エラー分類

| 失敗 | エラー |
|---|---|
| DB 一時障害 | 500（分類無しのデフォルト） |

---

## 4. スクリプト取得 (`GetScript`)

**入力**: `playerID`, `episodeID`, `lang`
**出力**: スクリプト本文文字列

### 4.1 バリデーション順序（fail-fast）

1. **エピソード存在確認**: `episodeID` で取得 → 不在または `is_active = false` なら `ErrEpisodeNotFound`
2. **アンロック判定**: プレイヤーの `StoryUnlockContext` を取得し `checkUnlock` を評価。未達条件があれば `ErrEpisodeLocked`
3. **スクリプト読み出し**: `script_path` テンプレートの `{lang}` を要求言語で置換し、ScriptStore から読む
   - 要求言語が存在しない: `ErrScriptNotFound`
   - ネットワーク / 権限エラー等: `ErrScriptInfra`

### 4.2 言語フォールバック非対応

**要求言語のスクリプトが存在しなければそのまま 404 を返す**。旧仕様にあった「`ja` に自動フォールバック」は削除された。理由と設計判断は [ARCHITECTURE.md](ARCHITECTURE.md#言語フォールバックを行わない) を参照。

クライアントは `lang=en` を指定したときに「英訳が未配信ならその旨を UI に表示する」「再度 `ja` で取り直す」のいずれかを自力で選ぶ必要がある。

### エラー分類

| 失敗 | エラー | HTTP |
|---|---|---|
| エピソードが存在しない / 非アクティブ | `ErrEpisodeNotFound` | 404 |
| アンロック条件未達 | `ErrEpisodeLocked` | 403 |
| 要求言語のスクリプト未配置 | `ErrScriptNotFound` | 404 |
| GCS / local FS のインフラ障害 | `ErrScriptInfra` | 500（原因はログのみ） |

---

## 5. エピソード完了記録 (`CompleteEpisode`)

**入力**: `playerID`, `episodeID`
**出力**: `error`

### 5.1 仕様

1. エピソード存在確認（存在しないまたは `is_active = false` → `ErrEpisodeNotFound`）
2. アンロック判定（未達なら `ErrEpisodeLocked`）
3. `(player_id, episode_id)` を PK として `player_story_progress` に INSERT
4. ON CONFLICT DO NOTHING で冪等化。重複呼び出しはエラーにならない

### 5.2 冪等性契約

- **キー**: `(player_id, episode_id)`
- **保証**: 同一キーで複数回呼ばれても DB 行は 1 行しか存在しない
- **副作用の追加は現時点ではなし**: `faction-selected` は本ユースケースからは発行されない（§8 参照）

### エラー分類

| 失敗 | エラー | HTTP |
|---|---|---|
| エピソードが存在しない / 非アクティブ | `ErrEpisodeNotFound` | 404 |
| アンロック条件未達 | `ErrEpisodeLocked` | 403 |
| DB 一時障害 | 分類なし | 500 |

---

## 6. 初期 faction 選択通知 (`NotifyInitialFactionSelected`)

**入力**: `playerID`, `factionID`
**出力**: `error`

### 6.1 仕様

1. `factionPublisher` が `nil` なら即エラー（配線退行の fail-fast、[ARCHITECTURE.md#構造的安全性](ARCHITECTURE.md#構造的安全性) 参照）
2. `FactionSelectedEvent` を構築
   - `event_id`: 毎回新規 UUID
   - `timestamp`: publish 時刻 (UTC)
   - `source`: `scenario_initial` 固定
3. JSON marshal して Pub/Sub `faction-selected` トピックへ publish
4. `result.Get(ctx)` で ACK を待って同期返却

### 6.2 冪等性契約

- scenario は **Transactional Outbox を持たない**。shop の purchase と異なり、DB 書き込みと publish を同一 tx で atomic に揃える機構はない
- publish は常に新しい `event_id` を採番する。リトライすれば別 ID のイベントが流れる
- **subscriber 側で `event_id` ベースに `processed_events` / 複合 PK で重複排除する前提**。scenario は at-least-once を保証しない（scenario 自身が失敗したら単にクライアントにエラーが返り、クライアントがリトライする）
- この契約の根拠と shop との設計差については [ARCHITECTURE.md#scenario-が-outbox-を持たない理由](ARCHITECTURE.md#scenario-が-outbox-を持たない理由) を参照

### 6.3 現時点の配線状態

本ユースケースは **現時点では `CompleteEpisode` から自動呼び出しされていない** (TODO)。将来、チュートリアルの faction 確定エピソード完了に紐づいて scenario 内部から呼ばれる予定。外部から直接叩く REST エンドポイントも現状は公開していない。サービス内部関数としてのみ存在し、配線作業が未完である旨を明示する。

### エラー分類

| 失敗 | HTTP |
|---|---|
| `factionPublisher` が nil（配線退行） | 500 |
| Pub/Sub publish 失敗 | 500 |
| `playerID` / `factionID` が空 | 500（呼び出し元バグ扱い。REST 層まで到達しない想定） |

---

## 7. エラーセマンティクス

サービス層は HTTP ステータスを知らない。エラーはセンチネルとして返し、handler (`internal/handler/rest/errors.go`) が `errors.Is` ベースの分類関数で transport 層のステータスに変換する。

### 7.1 分類

| 分類関数 | 対象エラー | HTTP |
|---|---|---|
| `isNotFound` | `story.ErrEpisodeNotFound`, `port.ErrScriptNotFound` | 404 |
| `isLocked` | `story.ErrEpisodeLocked` | 403 |
| `isInfra` | `port.ErrScriptInfra` | 500（詳細はレスポンスに含めず "script store unavailable" 相当を返す） |
| (default) | DB 一時障害などの未分類エラー | 500 |

### 7.2 `ErrScriptNotFound` と `ErrScriptInfra` の区別

- `ErrScriptNotFound`: ストアが **明確に "オブジェクトなし" を返した** ケース
  - GCS: `storage.ErrObjectNotExist`
  - local: `os.ErrNotExist`
- `ErrScriptInfra`: それ以外の失敗（ネットワーク、権限、読み取り途中の I/O エラー等）

この区別は **フォールバック可否のためではなく**、クライアントが「データ整備漏れ（運用側で対応）」と「一時的な障害（リトライで解決）」を見分けるためにある。どちらの場合も代替言語への自動差し替えは行わない。

### 7.3 握りつぶし禁止

GCS / local FS / DB / Pub/Sub のエラーをログのみで握りつぶしてはならない。必ず呼び出し元に伝搬させる（CLAUDE.md「行動制約」参照）。日本語へのフォールバックは `ErrObjectNotExist` にも適用しない。

---

## 8. イベント発行

| トピック | ペイロード | 発行契機 |
|---|---|---|
| `faction-selected` | `FactionSelectedEvent {event_id, timestamp, player_id, faction, source="scenario_initial"}` | `NotifyInitialFactionSelected` 呼び出し時（現在は明示的に呼ばれる経路のみ、`CompleteEpisode` とは未配線） |

publish 契約の詳細は §6 および [ARCHITECTURE.md](ARCHITECTURE.md#pubsub-publisher) を参照。

---

## 9. 構造的安全性

scenario は「サイレント no-op で起動する」「nil publisher がログだけで成功扱いになる」「GCS 切り替えミスで起動時に気づけない」といった運用事故を **起動時のバリデーションで構造的に排除する**。

| 対象 | 保証 |
|---|---|
| `DATABASE_URL` / `STORY_BUCKET` / `PUBSUB_PROJECT_ID` / `FIRESTORE_PROJECT_ID` 未設定 | 起動拒否（`config.FromEnv` が error を返す） |
| `STORY_BUCKET=local:` (パスなし) | 起動拒否 |
| `factionPublisher == nil` で `NotifyInitialFactionSelected` が呼ばれた | 明示的エラー（サイレント成功を構造的に防ぐ） |
| `STORY_BUCKET=local:<path>` | GCS クライアントを一切初期化せずにローカル FS から読む（開発体験の確保） |

詳細と意図は [ARCHITECTURE.md](ARCHITECTURE.md#構造的安全性) を参照。
