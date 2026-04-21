# Scenario サービス設計

本ドキュメントは **コードを読んでも一見しては分からない設計意図** だけを残す。実装詳細 (フロー順序・アンロック条件の評価順・エラー → HTTP ステータス変換・環境変数の一覧) は各ファイルの実装とコメントを一次情報とする。

サービス概要・起動手順は [../README.md](../README.md)、機能仕様は [FEATURE_SPEC.md](FEATURE_SPEC.md)、エンドポイントは [API_REFERENCE.md](API_REFERENCE.md)、テーブル定義は [DATA_DESIGN.md](DATA_DESIGN.md) を参照。

## Scenario の責務境界 (SSoT と cross-schema read)

scenario は **ストーリーエピソード定義とプレイヤー進行** の single source of truth だが、**プレイヤーのレベルや所有 faction** の authoritative owner ではない。

| 種別 | SSoT | scenario 側の扱い |
|---|---|---|
| エピソードマスター | `scenario.scenario_episodes` | 自サービス内で完結 |
| エピソード必須 faction | `scenario.episode_required_factions` | 自サービス内で完結 |
| プレイヤー進行 | `scenario.player_story_progress` | 自サービス内で完結 |
| ゲーム設定値 (`game_config`) | Firestore (プロジェクト共通) | scenario は read のみ |
| プレイヤーレベル | `account.players.level` | cross-schema read でアンロック判定に使用 |
| プレイヤー所有 faction | `account.player_factions` | cross-schema read でアンロック判定に使用 |

account と scenario は同一 DB クラスタ内の別スキーマに分かれており、現状は scenario の PostgreSQL プール経由で `players` / `player_factions` を直接 JOIN している (`postgres.StoryRepository.GetUnlockContext`)。ADR-014 の方針では scenario と account の DB 分離が予定されており、その段階で account クライアント経由 (HTTP / gRPC) に置換する。

scenario は他サービスを **直接呼ばない**。副作用を他サービスに伝えるのは `faction-selected` Pub/Sub publish のみで、account / card / gateway は自分の read model を購読で更新する。

## レイヤー分割

shop と揃えた Clean Architecture レイヤーに沿う。handler（transport） → service（use case） → port（抽象） → adapter / repository（具体実装）の片方向依存を守る。

```
cmd/server/main.go
  │
  ├─ internal/handler/rest/*          # Gin ハンドラ、HTTP ↔ sentinel 変換 (story / onboarding)
  ├─ internal/handler/worker/*        # 常駐 worker エントリ (OutboxTicker)
  │     │
  │     ├─ internal/service/story       # ユースケース (ListEpisodes / GetScript / CompleteEpisode)
  │     ├─ internal/service/onboarding  # ユースケース (GetStatus / GetScript / Complete)
  │     └─ internal/service/outbox      # outbox 消費ユースケース (RunOnce)
  │           │
  │           └─ internal/port/*      # interface 定義 (StoryRepo / OnboardingRepo / ScriptStore / OutboxStore / OutboxEventBuilder / RawEventPublisher / GameConfigRepo)
  │                 ▲
  │                 │ implements
  │                 │
  │     ┌───────────┴───────────┬───────────────────────┬──────────────────────────────┐
  │     │                       │                       │                              │
  │  internal/repository/      internal/repository/   internal/adapter/             internal/adapter/
  │  postgres                  firestore (GameConfig) gcs, local                    pubsub
  │  (StoryRepo /               (ScriptStore)           (Publisher = RawEventPublisher /
  │   OnboardingRepo /                                   EventBuilder = OutboxEventBuilder)
  │   OutboxStore)
```

依存方向:

- `handler/rest` → `service/*` → `port` にのみ向く
- `handler/worker` → `service/outbox` → `port` にのみ向く
- `service/*` は `postgres` / `firestore` / `gcs` / `local` / `pubsub` の具体型を知らない
- `port/mock_story_repo.go` が `service/story` のユニットテスト用 mock を保持する（旧 `internal/repository/mock_story_repo.go` は shop 準拠のレイアウトに合わせて port 直下に移動）

この並びは shop とほぼ同型で、layer のリファクタは shop 側の構造にサービス間で揃えるためのもの（ADR にある「サービス横断のテンプレート化」方針）。

## スクリプトストアの抽象化

スクリプト配信元は `STORY_BUCKET` 環境変数で **起動時に一度だけ決定** される。リクエストごとの切り替えは行わない。

| STORY_BUCKET の値 | 配信元 | ScriptStore 実装 |
|---|---|---|
| `op-scenario-scripts-prod` (GCS バケット名) | GCS | `internal/adapter/gcs.ScriptStore` |
| `local:./testdata/stories` | ローカル FS | `internal/adapter/local.ScriptStore` |

`cmd/server/main.go.buildScriptStore` が `config.Config.IsLocalStory()` を見て 2 実装から 1 つだけを返す。起動後は handler / service からは `port.ScriptStore` interface 越しにしか見えず、判定ロジックが service 層に漏れない。

**意図**: 開発マシンで GCS 認証なしに動かしたい、CI の e2e テストでテストデータを placement したい、という開発運用要件を、本番コードの静的配線で吸収する。リクエスト単位の切替（例: GCS 失敗時にローカルにフォールバック）は意図的に採用していない。「起動時に 1 度だけ決める」ことで、本番で誤ってローカルデータを読む事故を構造的に排除する。

### `{lang}` テンプレート解決

`scenario_episodes.script_path` にはテンプレート (例: `stories/{lang}/she_ep1.ks`) を格納する。`service.story.readScript` はリクエスト時の `lang` で `{lang}` を `ReplaceAll` 置換し、得られたキーで `ScriptStore.ReadScript` を呼ぶ。

変換ロジックは scripted store 手前に閉じ、ストア実装は「キー → バイト列」の純粋な read しか知らない。

## 言語フォールバックを行わない

### 旧仕様

以前は「要求言語のスクリプトが存在しない場合 `ja` のスクリプトへフォールバックする」という仕様が存在した。

### 現仕様（フォールバック廃止）

要求言語のスクリプトが存在しなければそのまま `ErrScriptNotFound` (404) を返す。`ja` への自動差し替えは **一切しない**。

### 廃止した理由

1. **「プレイヤーが選んだ言語」と「実配信データ」の一致を単純化するため**
   フォールバックありだと、UI は「この画面は ja / en のどちらで表示されているか」をクライアント側で別途判定する必要が生じる（サーバは言語を返さない）。実配信と要求の一致を API 側で保証するほうが、UI 側の状態設計が単純になる。

2. **運用側のデータ整備漏れを隠すより、明示的に失敗させるほうが健全**
   `ja` で上書きすると翻訳欠落が長期間気づかれないまま残る。404 を返して監視アラートに出すほうが、結果的に整合性を保ちやすい（CLAUDE.md「エラーは握りつぶさない」）。

3. **契約の明文化**
   クライアントは「`lang=en` を指定して 404 が来たら `ja` で再取得する」戦略を採るかどうかを UX の観点で選べる。サーバが勝手にフォールバックすると、この選択権がクライアント側から奪われる。

4. **GCS の非 not-found エラーとの混同を避けるため**
   旧フォールバック実装では `ErrObjectNotExist` のみをフォールバック条件にしていたが、ネットワーク / 権限エラーが「フォールバック対象ではない」ことを常に意識する必要があった。フォールバック自体を無くすことで、この実装上の繊細さが不要になる。

### `ErrObjectNotExist` / `os.ErrNotExist` の扱い

フォールバックを廃止しても、**not-found と infra エラーの区別は依然として重要**。クライアント向けにも「データが無い（404）」と「一時障害（500）」は別のハンドリングを要するため、adapter 層で以下のように分ける:

| ストア | 分類関数 | sentinel | HTTP |
|---|---|---|---|
| GCS | `err == storage.ErrObjectNotExist` | `ErrScriptNotFound` | 404 |
| GCS | その他（net, permission, I/O） | `ErrScriptInfra` | 500 |
| local | `errors.Is(err, os.ErrNotExist)` | `ErrScriptNotFound` | 404 |
| local | その他 | `ErrScriptInfra` | 500 |

CLAUDE.md の「GCS エラーを握りつぶさない。日本語へのフォールバックは `ErrObjectNotExist` のみ。パーミッション / トランスポートエラーには適用しない」は、フォールバック廃止後も **「not-found と infra を混同しない」という意味で有効** である。現状コードはフォールバック自体を実装しないため、この制約は分類ロジックに対する不変条件として残っている。

## アンロック判定

エピソードごとに 3 種類の条件を複合 AND で評価する。

| 条件 | データソース | LockReason.type |
|---|---|---|
| プレイヤーレベル | `players.level` (account スキーマ) | `level` |
| 所有ファクション | `player_factions` (account スキーマ) | `faction` |
| 前提エピソード完了 | `player_story_progress` (scenario スキーマ) | `episode` |

`StoryUnlockContext` は `StoryRepo.GetUnlockContext` で 1 リクエストあたり 1 回だけ pull する（`ListEpisodes` 内では全エピソードで共有、`GetScript` / `CompleteEpisode` では `validateUnlock` から単発で取得）。キャッシュはしない。

**意図**: 未達条件を 1 つでも返せば即 `ErrEpisodeLocked` にする実装は `GetScript` / `CompleteEpisode` の validate 側だけで、`ListEpisodes` は全未達条件を reasons として返す。これは UX 差（一覧では「なぜロックされているか」を全部見せたい、取得時は 403 を返して終わり）をサービス層で直接表現するため。

## Pub/Sub publisher

### 契約

| 項目 | 値 |
|---|---|
| トピック | `faction-selected` / `player-onboarded`（いずれも `*_TOPIC` env で上書き可、クロスプロジェクト検証用） |
| ペイロード型 | `FactionSelectedEvent` / `PlayerOnboardedEvent` (`overload-party-common/packages/pubsub-events`) |
| `faction-selected.source` | `scenario_initial` 固定（shop は `shop_purchase`） |
| publish 経路 | `internal/adapter/pubsub/publisher.go`（topic 名 → `*pubsub.Topic` の map ラッパ）を outbox worker が呼ぶ |
| 配信保証 | at-least-once（subscriber 側で `event_id` ベースに重複排除する前提） |

### トリガー

scenario の live publish は全て outbox 経由で起きる。具体的には `OnboardingService.Complete` が `scenario.player_onboarding` への INSERT と `scenario.outbox_events` への 2 行挿入を同一トランザクションで commit し、常駐 worker (`internal/handler/worker/outbox_ticker.go`) が未配信行を claim して `adapter/pubsub.Publisher.Publish` を呼ぶ。旧 `StoryService.NotifyInitialFactionSelected` は [ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) で削除されたため、`story.Service` は publisher 依存を持たない。

### subscriber 側の責務

scenario は publish して終わり。以下の副作用は全て subscriber に委ねる:

- account: `players.display_name` 更新（`player-onboarded`）、`player_factions` INSERT + `players.selected_faction` UPDATE（`faction-selected`）
- card: `player_cards` に faction + Neutral のカードを付与（`faction-selected`）
- gateway: WS push で完了通知（`faction-selected`）

### scenario の Outbox

scenario は shop と同型の **Transactional Outbox** を持つ。DB commit と Pub/Sub publish を同一トランザクションに相乗りさせて、dual-write による部分失敗（完了記録は入ったが publish に失敗、またはその逆）を構造的に排除するための機構で、shop の [イベント配信モデル (Transactional Outbox)](../../overload-party-shop/docs/ARCHITECTURE.md#イベント配信モデル-transactional-outbox) と設計思想・SQL・port 契約を揃えている。

本節は以前「scenario が Outbox を持たない理由」として atomic publish 不要という立場を記していたが、[ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) で追加された「オンボーディング完了」ユースケースは **`scenario.player_onboarding` への INSERT と 2 つの Pub/Sub publish を atomic に揃える必要がある** 配線であり、旧節が予告していた導入条件にそのまま合致する。そのため scenario 側にも `scenario.outbox_events` を新設し、旧方針は本 ADR 採用をもって反転させた。

shop との差分は「何を積むか」だけで、インフラ側は共通化している:

| 項目 | shop | scenario |
|---|---|---|
| 積む event kind | `BuildFactionSelected(source=shop_purchase)` / `BuildPremiumUpdated` | `BuildPlayerOnboarded` / `BuildFactionSelected(source=scenario_initial` 固定`)` |
| テーブルスキーマ | `shop.outbox_events` | `scenario.outbox_events`（カラム・インデックス同一） |
| port 契約 (`OutboxStore` / `OutboxEventBuilder`) | shop | scenario でも同一インターフェース |
| poller 実装 | `internal/service/outbox_publisher.go` + `internal/handler/worker/outbox_ticker.go` | 同名パスで同一構造 |
| 配信保証 | at-least-once + visibility timeout + `failure_count` 閾値 | 同一 |

結果として scenario の publisher は「`adapter/pubsub.Publisher` が topic 名から `*pubsub.Topic` を引いて送出する」薄いラッパに退避し、atomic 性の責務は outbox 側に集約されている。運用観測メトリクス名もサービスプレフィックスのみ差し替えた同一体系（`scenario_outbox_unpublished_gauge` 等）を採る。

## オンボーディングシナリオ

[ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) により追加された「一度きり読了で display_name と初期 faction を集める」ユースケースは、既存 `ScenarioEpisode` 配管と **サービス層・テーブル・API・イベントのいずれも分離** する。詳細仕様は [FEATURE_SPEC.md](FEATURE_SPEC.md) と ADR-021 に委ね、ここでは別フロー化した設計観点だけ残す。

### なぜ別ユースケースにしたか

既存 `ScenarioEpisode` は unlock 判定（level / 所有 faction / 前提エピソード）を入口に持ち、完了後も本文再読が可能で、完了の副作用は進行マーカーのみという前提で組まれている。オンボはこのすべてが逆転する:

- faction もレベルも無い状態で最初に走るため、既存 unlock モデルに条件を注入できない
- 「一度きり」セマンティクスのため、完了後は本文を返さず 409 で弾く必要がある
- 完了時に identity 副作用（display_name 書き込み + 初期 faction hand-off）を atomic に伴う

これらを既存エピソードに載せると `checkUnlock` / `GetScript` / `CompleteEpisode` の全てに「オンボ時だけ違う」横串分岐が増え、「一つの関数に複数の責務を負わせない」に反する。したがって `internal/service/onboarding/` として独立 service を構え、`scenario.player_onboarding`（PK = `player_id` で 2 度目の INSERT が一意制約違反になる形で「一度きり」を保証）と上記 outbox を通じて 2 イベントを atomic publish する配線に分離した。

### SSoT 分離

scenario は「オンボ完了フラグ」と「スクリプト」の SSoT を持つが、display_name や所持 faction は **保持しない**。publish を中継するだけで、identity の SSoT は account 側に残す。これにより scenario は将来 display_name 変更機能などが account 側に入っても影響を受けない。

## 構造的安全性

scenario は「静かに no-op で起動する」「nil publisher がログだけで成功扱いになる」「GCS 設定ミスが本番で初めて顕在化する」といった運用事故を、**起動時のバリデーションとコード配線で構造的に排除する**。

### 起動時 fail-fast

`internal/config/config.go.FromEnv` が下記を検証し、欠損 / 不正があれば即 error を返し main が exit 1 する:

- `DATABASE_URL` 必須
- `STORY_BUCKET` 必須
- `STORY_BUCKET=local:<path>` のとき `<path>` 非空
- `PUBSUB_PROJECT_ID` 必須
- `FIRESTORE_PROJECT_ID` 必須（現在 runtime からは未参照だが、起動時にプロジェクト ID の典型的タイポを検出する目的で必須化）

### outbox worker 構築時のゼロ値拒否と unknown topic 明示エラー

publisher 関連の fail-fast は [ADR-021](../../overload-party-common/docs/adr/021-onboarding-scenario.md) で `story.Service.NotifyInitialFactionSelected` の nil チェックから outbox 側 2 点に移った。どちらも「設定ミスが本番で初めて観測される」状態を構造的に塞ぐための配線。

1. **worker config のゼロ値拒否**: `internal/handler/worker/outbox_ticker.go` の起動時に `BatchSize` / `FailureThreshold` / `VisibilityTimeout` などが 0（env 未設定や typo）なら即 error を返し main が exit 1 する。ゼロ値で起動してしまうと claim クエリが縮退して「publish しない worker」がサイレントに常駐するため。
2. **unknown topic のエラー返却**: `adapter/pubsub.Publisher.Publish(topic, payload)` は内部 topic map に無い topic 名に対して即エラーを返す。outbox 行の `topic` カラムに誤った値が書かれた場合、握りつぶさず `RecordFailure` 経路に載せて `failure_count` 閾値 → アラートに辿れるようにする。

**意図**: shop の `getVerifier` が `ErrUnsupportedPlatform` を返すのと同じ発想で、nil 依存・設定欠損・列挙漏れは実行時に **明示的に** 検出し、ログのみで成功扱いになるサイレント退行を構造的に防ぐ。

### `IsLocalStory` 判定の閉じ込め

`config.Config.IsLocalStory()` と `StoryLocalPath()` はストア判定ロジックを config 層に閉じ込める。service / handler からは `port.ScriptStore` 越しにしか見えず、`local:` プレフィクスの扱いが散らばらない。

## テスト戦略

shop の方針に準拠し、以下の 3 層でテストする。

| 対象 | 手段 |
|---|---|
| `service/story` / `service/onboarding` のユースケースロジック | `internal/port` 配下の mock / stub（`MockStoryRepository` / `ScriptStore` / `OnboardingRepo` / `OutboxEventBuilder` 等）でユニットテスト |
| `repository/postgres` | testcontainers で実 PostgreSQL を起動し、`schema.sql` を流した上で CRUD を検証（整備予定） |
| `repository/firestore` | Firestore emulator に対する統合テスト (`repository/firestore/game_config_repo_test.go`) |

GCS adapter は interface レベルで mock される（現状 `adapter/gcs` の直接テストは持たない。testcontainers 化の優先度は低い）。

## 運用

### 環境変数

環境変数の一覧と必須条件は [internal/config/config.go](../internal/config/config.go) が SSoT。運用上の注意点:

- **`STORY_BUCKET`**: 本番は GCS バケット名を直接設定。dev / ローカルは `local:<path>` で FS 切替。誤って本番に `local:` を設定すると起動拒否になる（正常）
- **`PUBSUB_PROJECT_ID`** / **`FACTION_SELECTED_TOPIC`** / **`PLAYER_ONBOARDED_TOPIC`**: 本番環境と一致する Google Cloud project を指定。topic は Terraform (`modules/pubsub`) で事前作成されている前提。未作成トピック・未登録 topic 名への publish は outbox worker の `RecordFailure` 経路に載るため `failure_count` アラートで検出できる
- **`OUTBOX_POLL_INTERVAL` / `OUTBOX_BATCH_SIZE` / `OUTBOX_FAILURE_THRESHOLD` / `OUTBOX_VISIBILITY_TIMEOUT`**: outbox worker の挙動を env 経由で調整する。負荷試験やインシデント時にデプロイなしで可変。shop と同じ名前・意味で揃えている
- **`FIRESTORE_PROJECT_ID`**: `game_config` の読み取り先。ローカル / CI では `FIRESTORE_EMULATOR_HOST` で emulator 接続に差し替え可能

### Pub/Sub トピックと subscriber

| トピック | 発行契機 | subscriber |
|---|---|---|
| `faction-selected` | `OnboardingService.Complete` の DB commit 後（outbox worker が `scenario.outbox_events` 行を消費） | account, card, gateway |
| `player-onboarded` | 同上（同一トランザクションで 2 行が outbox に積まれる） | account |

subscriber 列はこのリポジトリからは導けないので、変更時は各サービスの購読状況も確認すること。
