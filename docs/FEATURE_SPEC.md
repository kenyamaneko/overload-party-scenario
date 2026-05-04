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
| オンボーディングシナリオ | 初回起動時に一度だけ読ませる。各ステップ完了で `onboarding-name-set` / `onboarding-faction-set` / `player-onboarded` を outbox publish し、業務データの永続化は account の subscriber に委ねる（§10、[ADR-026](../../overload-party-common/docs/adr/026-onboarding-status-as-account-responsibility.md)） |
| 初期 faction 選択 hand-off | オンボーディング完了時に `player-onboarded` Pub/Sub イベントの `initial_faction_id` で card の初期パック配布へ伝播する（§10。`selected_faction` への永続化は先行する `onboarding-faction-set` で account に書込済み） |

scenario は **scenario スキーマの DB 行と Firestore `game_config` を唯一の真実とし**、account スキーマ (`players.level` / `player_factions`) は cross-schema read のみで扱う。card / gateway を直接呼び出さない。account に対しては onboarding 内 REST 直叩き 2 経路（表示名 validate と Complete 時の faction 取得）に限り例外的に呼び出す（[ADR-025](../../overload-party-common/docs/adr/025-onboarding-name-via-rest-and-cross-service-http.md) / [ADR-026](../../overload-party-common/docs/adr/026-onboarding-status-as-account-responsibility.md)）。

### 非対象

| 機能 | 取り扱い |
|---|---|
| クライアント認証 | ClusterIP 到達性のみを信頼境界とし、URL 内の `playerId` を無条件で信頼する（gateway が Firebase Auth で確定した ID を forward する前提） |
| スクリプト本文の変換・レンダリング | ストアから読み出した文字列を変換せずそのまま返す |
| 言語別テキストの DB 保持 | `title_ja` / `title_en` のみ DB に持つ。本文は `{lang}` テンプレートでパス切替した外部ストアに委ねる |
| 言語フォールバック | 行わない。要求言語のスクリプトが欠けていれば 404 を返す（§7 参照） |
| `player-onboarded` 以外のイベント発行 | 行わない |
| Pub/Sub イベントの subscribe | 行わない。scenario は発信のみ、購読は account / card / gateway が担う |

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
2. **アンロック判定**: プレイヤーの `StoryUnlockContext` を取得し `Episode.LockReasons` を評価。未達条件があれば `ErrEpisodeLocked`
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
- **副作用の追加は現時点ではなし**: エピソード完了からは Pub/Sub イベントを発行しない（オンボーディング完了のみが `player-onboarded` を発行する。§8 参照）

### エラー分類

| 失敗 | エラー | HTTP |
|---|---|---|
| エピソードが存在しない / 非アクティブ | `ErrEpisodeNotFound` | 404 |
| アンロック条件未達 | `ErrEpisodeLocked` | 403 |
| DB 一時障害 | 分類なし | 500 |

---

## 6. Pub/Sub 配信と Transactional Outbox

scenario は Pub/Sub publish を **必ず outbox 経由** で行う。ビジネステーブルの書き込みと event 行の INSERT を同一トランザクションに相乗りさせ、dual-write による部分失敗（DB だけ成功／publish だけ成功）を構造的に排除する。

### 6.1 配線

1. ユースケース層 (`OnboardingService.Complete`) が `OutboxEventBuilder.BuildPlayerOnboarded` で `OutboxEvent` を構築する
2. repository (`OnboardingRepo.MarkComplete`) が `scenario.player_onboarding` INSERT と `scenario.outbox_events` への 1 行 INSERT を **同一 tx で commit** する（PK 一意制約違反は `ErrAlreadyOnboarded` に昇格）
3. 別 goroutine の常駐 worker (`internal/handler/worker/outbox_ticker.go`) が未 publish 行を periodic に poll し、`adapter/pubsub.Publisher.Publish` を呼ぶ
4. publish 成功で `published_at` を UPDATE、失敗は `failure_count` / `last_error` / `last_attempted_at` を積む。閾値超過行は claim 対象から外れ監視アラートに委ねる

### 6.2 Transactional Outbox を持つ方針

scenario は shop と同型の Transactional Outbox を持つ。旧仕様書には「scenario は Transactional Outbox を持たない」と記していたが、[ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) で追加された **オンボーディング完了フロー**（`scenario.player_onboarding` への INSERT と Pub/Sub publish の atomic 保証が必要）によって方針を反転した。[ADR-022](../../overload-party-common/docs/adr/022-faction-selected-decomposition.md) で publish 対象は `player-onboarded` 1 イベントに縮退したが、DB と publish を同一トランザクションで束ねる必要性は変わらない。設計詳細と shop との差分は [ARCHITECTURE.md#scenario-の-outbox](ARCHITECTURE.md#scenario-の-outbox) 参照。

### 6.3 冪等性契約

- `event_id` は outbox 行の enqueue 時点で確定し、payload 内 `eventId` と一致する。worker が再試行しても同じ ID を送る (at-least-once)
- subscriber は `processed_events` / 複合 PK で重複適用を排除する（[ADR-012](../../overload-party-common/docs/adr/012-matchmaking-pubsub.md) の契約）
- `FOR UPDATE SKIP LOCKED` と visibility timeout により複数 Pod が同行を多重 publish しないことを保証する

### 6.4 publisher 実装

publish の入口は `internal/adapter/pubsub/publisher.go` に一本化される。topic 名 → `*pubsub.Topic` の map を持つ薄いラッパで、未登録 topic に対しては即エラーを返す（outbox 行の topic カラムに typo が入っても握りつぶさず `RecordFailure` に載せるため）。旧 `StoryService.NotifyInitialFactionSelected` は [ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) で削除され、`story.Service` は publisher 依存を持たない。

### エラー分類

| 失敗 | 影響 |
|---|---|
| DB commit 失敗 | ユースケース層にエラーを返す（クライアントへ 500）。outbox 行も巻き戻るため部分送信は発生しない |
| outbox worker の publish 失敗 | `failure_count` 加算。閾値までは自動再試行、閾値超過で運用アラート |
| 未登録 topic への publish | `adapter/pubsub.Publisher` が即エラーを返し `RecordFailure` 経路に載せる |

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
| `onboarding-name-set` | `OnboardingNameSetEvent {event_id, event_type, timestamp, player_id, name}` | `OnboardingService.UpdateName` が account の validate REST 成功後に outbox に積む。account が subscribe して `players.name` + `onboarding_status='name_set'` を 1 tx で UPDATE |
| `onboarding-faction-set` | `OnboardingFactionSetEvent {event_id, event_type, timestamp, player_id, initial_faction_id}` | `OnboardingService.SelectFaction` が `SelectableFactions` 検証成功後に outbox に積む。account が subscribe して `selected_faction` UPDATE + `player_factions` INSERT + `onboarding_status='faction_set'` を 1 tx で実行 |
| `player-onboarded` | `PlayerOnboardedEvent {event_id, event_type, timestamp, player_id, initial_faction_id}` | `OnboardingService.Complete` の DB commit 後（outbox worker が `scenario.outbox_events` 行を消費）。account: `onboarding_status='completed'` UPDATE のみ。card: 初期パック配布 |

scenario が publish する topic はこの 1 本のみ。[ADR-022](../../overload-party-common/docs/adr/022-faction-selected-decomposition.md) で旧 `faction-selected` は廃止され、初期 faction ハンドオフは `PlayerOnboardedEvent.initial_faction_id` に統合された。publish 契約の詳細は §6 および [ARCHITECTURE.md](ARCHITECTURE.md#pubsub-publisher) を参照。

---

## 9. 構造的安全性

scenario は「サイレント no-op で起動する」「設定欠損の publisher がログだけで成功扱いになる」「GCS 切り替えミスで起動時に気づけない」といった運用事故を **起動時のバリデーションで構造的に排除する**。

| 対象 | 保証 |
|---|---|
| `DATABASE_CONN` / `STORY_BUCKET` / `PUBSUB_PROJECT_ID` / `FIRESTORE_PROJECT_ID` / `ACCOUNT_BASE_URL` 未設定 | 起動拒否（`config.FromEnv` が error を返す） |
| `STORY_BUCKET=local:` (パスなし) | 起動拒否 |
| outbox worker の `BatchSize` / `FailureThreshold` / `VisibilityTimeout` が 0 | 起動拒否（設定欠損で publish が縮退することを防ぐ） |
| 未登録 topic への publish | `adapter/pubsub.Publisher` が明示的にエラー（outbox の `RecordFailure` 経路に載る） |
| `STORY_BUCKET=local:<path>` | GCS クライアントを一切初期化せずにローカル FS から読む（開発体験の確保） |

詳細と意図は [ARCHITECTURE.md](ARCHITECTURE.md#構造的安全性) を参照。

---

## 10. オンボーディングシナリオ

初回起動時に 1 度だけ読ませる業務フロー。各ステップ完了で対応する Pub/Sub event
(`onboarding-name-set` / `onboarding-faction-set` / `player-onboarded`) を outbox 経由で発行し、
業務データの永続化は account 側 subscriber に委ねる ([ADR-026](../../overload-party-common/docs/adr/026-onboarding-status-as-account-responsibility.md))。
account の REST 呼び出しは表示名のバリデーションと完了 publish 用の faction 取得に限定し、
scenario 側で account の業務カラムを直接書き換えない。
進行状態は `account.players.onboarding_status` カラムが SSoT で、クライアントは account の
`GET /internal/v1/players/:playerId` レスポンスから取得する。

### 10.1 ユースケース

1. **GET `/internal/v1/players/:playerId/onboarding/script?lang=ja|en`**: 本文取得
2. **PUT `/internal/v1/players/:playerId/onboarding/name`**: 表示名を受け取り、account に validate REST を依頼。成功時に `onboarding-name-set` event を outbox publish。account の 400 (`ErrInvalidName` 相当) はそのまま中継する
3. **POST `/internal/v1/players/:playerId/onboarding/faction`**: `initial_faction_id` を受け取り、`SelectableFactions` で scenario 側 validate。成功時に `onboarding-faction-set` event を outbox publish
4. **POST `/internal/v1/players/:playerId/onboarding/complete`**: scenario 読了時に呼ばれる。account の `GetPlayer` で `selected_faction` を取得して `player-onboarded` payload を組み立て、`scenario.player_onboarding` INSERT + `player-onboarded` publish を atomic に実行

オンボード進行状態取得 (`onboarding_status`) は account の `GET /internal/v1/players/:playerId` 経由で
クライアントが直接取得する。scenario 側に進行状態取得用エンドポイントは持たない。

API の完全なリクエスト／レスポンス仕様は [API_REFERENCE.md](API_REFERENCE.md) が SSoT（`data/endpoints.yaml` から codegen）。

### 10.2 一度きりセマンティクス

- `scenario.player_onboarding` の PK = `player_id` によって 2 度目の `INSERT` は一意制約違反となり、`ErrAlreadyOnboarded` → 409 に昇格する
- 完了後の `GET /onboarding/script` は **409 already_onboarded** を返す（本文を再配信しない）。既存エピソードが完了後も再読可能なのと対照的なセマンティクス
- scenario 側に保持するのは `player_id` / `completed_at` のみ。表示名 / faction_id は保持せず、publish を中継するだけで SSoT は account 側に残す

### 10.3 入力バリデーション

- 表示名: account の `internal/model/name.go`（`MaxNameRunes = 20`、空 / 全空白 / 制御文字 / 上限超で `ErrInvalidName`）が業務 SSoT。scenario 側はバリデーションを持たず、`POST /onboarding/name/validate` の 400 をそのまま中継する
- `initial_faction_id`: `overload-party-common/packages/game-design-constants.SelectableFactions` に対して membership 検証を scenario 側で実行。該当なしは `ErrInvalidFaction` → 400。`is_collectible=false` の `Neutral` は codegen フィルタ時点で除外されるため usecase 層で重ね書きしない
- 一意性は検査しない（表示名衝突は許容、playerID が identity の SSoT）

### 10.4 スクリプト配置

`scripts/onboarding/{lang}.ks` に配置する（既存 `stories/{lang}/*.ks` と別ツリー）。言語フォールバックは既存エピソードと同じく **行わない**（§7 / [ARCHITECTURE.md#言語フォールバックを行わない](ARCHITECTURE.md#言語フォールバックを行わない)）。`{lang}` 不在なら 404 `script_not_found`。

### 10.5 各ステップで publish するイベント

業務事実ごとに 3 トピックに分離する。各 event は scenario の outbox 経由で atomic に enqueue され、
account の subscriber が単一 tx で「業務データ永続化 + `onboarding_status` 遷移」を反映する。

| トピック | publisher | subscriber | payload | 副作用 |
|---|---|---|---|---|
| `onboarding-name-set` | scenario | account | `event_id` / `event_type` / `timestamp` / `player_id` / `name` | account: `players.name` + `onboarding_status='name_set'` を 1 tx で UPDATE |
| `onboarding-faction-set` | scenario | account | `event_id` / `event_type` / `timestamp` / `player_id` / `initial_faction_id` | account: `players.selected_faction` UPDATE + `player_factions` INSERT (`source='initial_selection'`) + `onboarding_status='faction_set'` を 1 tx で実行 |
| `player-onboarded` | scenario | account, card | `event_id` / `event_type` / `timestamp` / `player_id` / `initial_faction_id` | account: `onboarding_status='completed'` UPDATE。card: `GrantInitialPack(player_id, initial_faction_id)` で初期パック配布 |

subscriber は `event_id` を冪等性キーに `processed_events` で重複排除する。
state machine の一方向遷移性 (`not_started` → `name_set` → `faction_set` → `completed`) を活用した
条件付き UPDATE で out-of-order 配信に対しても整合性が保たれる。

### 10.6 エラー分類

| 失敗 | エラー | HTTP |
|---|---|---|
| 完了済みプレイヤーの `GET script` / `POST complete` | `ErrAlreadyOnboarded` | 409 |
| `POST /complete` で faction 選択ステップ未完了 (`selected_faction` が account に未設定) | `ErrFactionNotSelected` | 409 |
| `initial_faction_id` が `SelectableFactions` に非該当 | `ErrInvalidFaction` | 400 |
| `PUT /onboarding/name` で account の `ValidateName` に違反 | `ErrInvalidName` | 400 |
| `PUT /onboarding/name` / `POST /onboarding/complete` で account に Player が存在しない | `ErrPlayerNotFound` | 404 |
| 要求言語のスクリプト未配置 | `ErrScriptNotFound` | 404 |
| GCS / local FS のインフラ障害 | `ErrScriptInfra` | 500 |
| DB commit 失敗 / account 連携の障害 | 分類なし | 500（outbox 行も巻き戻るため部分送信は発生しない。account 側 5xx は中継する） |
