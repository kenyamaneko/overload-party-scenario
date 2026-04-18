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
  ├─ internal/handler/rest/*          # Gin ハンドラ、HTTP ↔ sentinel 変換
  │     │
  │     └─ internal/service/story     # ユースケース (ListEpisodes / GetScript / CompleteEpisode / NotifyInitialFactionSelected)
  │           │
  │           └─ internal/port/*      # interface 定義 (StoryRepo / ScriptStore / FactionPublisher / GameConfigRepo)
  │                 ▲
  │                 │ implements
  │                 │
  │     ┌───────────┴───────────┬───────────────────────┬──────────────────┐
  │     │                       │                       │                  │
  │  internal/repository/      internal/repository/   internal/adapter/  internal/adapter/
  │  postgres (StoryRepo)      firestore (GameConfig) gcs, local         pubsub
  │                             (ScriptStore)         (FactionPublisher)
```

依存方向:

- `handler/rest` → `service/story` → `port` にのみ向く
- `service/story` は `postgres` / `firestore` / `gcs` / `local` / `pubsub` の具体型を知らない
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

## Pub/Sub `faction-selected` publisher

### 契約

| 項目 | 値 |
|---|---|
| トピック | `faction-selected` (`FACTION_SELECTED_TOPIC` で上書き可、クロスプロジェクト検証用) |
| ペイロード型 | `FactionSelectedEvent` (`overload-party-common/packages/pubsub-events`) |
| source | `scenario_initial` 固定（shop は `shop_purchase`） |
| publish 同期性 | `result.Get(ctx)` で ACK を待つ同期 publish |
| 配信保証 | at-least-once（subscriber 側で `event_id` ベースに重複排除する前提） |

### トリガー

scenario は `StoryService.NotifyInitialFactionSelected(ctx, playerID, factionID)` で明示的に publish する。現在このメソッドは `CompleteEpisode` から自動呼び出しされていない (`TODO(faction-handoff)`) ため、外部配線作業が完了するまで publish は実測では駆動されない。

### subscriber 側の責務

scenario は publish して終わり。以下の副作用は全て subscriber に委ねる:

- account: `player_factions` INSERT + `players.selected_faction` UPDATE
- card: `player_cards` に faction + Neutral のカードを付与
- gateway: WS push で完了通知

### scenario が Outbox を持たない理由

shop は `shop.outbox_events` を使って「DB commit と Pub/Sub publish を同一トランザクションに相乗り」させる Transactional Outbox パターンを採用している（[shop/ARCHITECTURE.md](../../overload-party-shop/docs/ARCHITECTURE.md#イベント配信モデル-transactional-outbox)）。scenario はこれを採用しない。

理由:

1. **scenario の publish は DB 書き込みと atomic である必要がない**
   shop の `Purchase` は「購入レコード INSERT + 所有権 INSERT + event publish」が一致しないと subscriber 側の fan-out が壊れる（課金完了したのに faction が付かない等）。scenario の `NotifyInitialFactionSelected` は現状「DB 書き込み + publish」ではなく「publish のみ」であり、atomic に揃えるべき DB 行がそもそも存在しない。
2. **クライアント主導のリトライで十分**
   publish が失敗すればクライアントに 500 が返り、クライアントが UX 的な判断でリトライする。shop の webhook と違って外部ストアがリトライを強制する仕組みではないので、常駐 worker による at-most-once の publish 保証を用意する動機が弱い。
3. **subscriber 側で冪等性を担保する契約は shop と同じ**
   `event_id` は毎回新規採番され、subscriber は `processed_events` / 複合 PK で重複適用を防ぐ。at-least-once の保証は shop と同じ形で成立する（scenario 自身が重複 publish を発生させにくいだけ）。

この設計上の差は FEATURE_SPEC §6.2 で「scenario は Transactional Outbox を持たない」と明記する。将来 `CompleteEpisode` から自動 publish する配線に変えるときに、atomic 保証が必要になれば scenario 側にも Outbox を導入する選択肢を残しておく（その時点で shop と同型の構造を再利用できる）。

## 構造的安全性

scenario は「静かに no-op で起動する」「nil publisher がログだけで成功扱いになる」「GCS 設定ミスが本番で初めて顕在化する」といった運用事故を、**起動時のバリデーションとコード配線で構造的に排除する**。

### 起動時 fail-fast

`internal/config/config.go.FromEnv` が下記を検証し、欠損 / 不正があれば即 error を返し main が exit 1 する:

- `DATABASE_URL` 必須
- `STORY_BUCKET` 必須
- `STORY_BUCKET=local:<path>` のとき `<path>` 非空
- `PUBSUB_PROJECT_ID` 必須
- `FIRESTORE_PROJECT_ID` 必須（現在 runtime からは未参照だが、起動時にプロジェクト ID の典型的タイポを検出する目的で必須化）

### nil factionPublisher の明示的エラー

`StoryService.NotifyInitialFactionSelected` は `factionPublisher == nil` を明示チェックし、nil なら即 error を返す。

```go
if s.factionPublisher == nil {
    return fmt.Errorf("scenario: NotifyInitialFactionSelected called with nil factionPublisher")
}
```

**意図**: 将来テスト用コンストラクタや配線変更で factionPublisher が nil のまま service が構築されるパスが入り込んでも、publish されずに成功扱いになるサイレント退行を防ぐ。shop の `getVerifier` が `ErrUnsupportedPlatform` を返すのと同じ発想で、nil 依存は実行時に明示的に検出する。

### `IsLocalStory` 判定の閉じ込め

`config.Config.IsLocalStory()` と `StoryLocalPath()` はストア判定ロジックを config 層に閉じ込める。service / handler からは `port.ScriptStore` 越しにしか見えず、`local:` プレフィクスの扱いが散らばらない。

## テスト戦略

shop の方針に準拠し、以下の 3 層でテストする。

| 対象 | 手段 |
|---|---|
| `service/story` のユースケースロジック | `internal/port/mock_story_repo.go` の `MockStoryRepository` + stub の `ScriptStore` / `FactionPublisher` でユニットテスト (`service/story/service_test.go`) |
| `repository/postgres` | testcontainers で実 PostgreSQL を起動し、`schema.sql` を流した上で CRUD を検証（整備予定） |
| `repository/firestore` | Firestore emulator に対する統合テスト (`repository/firestore/game_config_repo_test.go`) |

GCS adapter は interface レベルで mock される（現状 `adapter/gcs` の直接テストは持たない。testcontainers 化の優先度は低い）。

## 運用

### 環境変数

環境変数の一覧と必須条件は [internal/config/config.go](../internal/config/config.go) が SSoT。運用上の注意点:

- **`STORY_BUCKET`**: 本番は GCS バケット名を直接設定。dev / ローカルは `local:<path>` で FS 切替。誤って本番に `local:` を設定すると起動拒否になる（正常）
- **`PUBSUB_PROJECT_ID`** と **`FACTION_SELECTED_TOPIC`**: 本番環境と一致する Google Cloud project を指定。topic は Terraform (`modules/pubsub`) で事前作成されている前提。未作成のトピックに publish すると runtime で失敗する
- **`FIRESTORE_PROJECT_ID`**: `game_config` の読み取り先。ローカル / CI では `FIRESTORE_EMULATOR_HOST` で emulator 接続に差し替え可能

### Pub/Sub トピックと subscriber

| トピック | 発行契機 | subscriber |
|---|---|---|
| `faction-selected` | `NotifyInitialFactionSelected` 呼び出し時（同期 publish） | account, card, gateway |

subscriber 列はこのリポジトリからは導けないので、変更時は各サービスの購読状況も確認すること。
