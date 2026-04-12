# Scenario サービス設計

このドキュメントは scenario サービスの内部動作を説明する。サービスの概要・エンドポイント・環境変数は [README.md](../README.md) を参照。

## GCS / ローカルファイルシステム切り替え

スクリプト配信元は `STORY_BUCKET` 環境変数で起動時に決定される。リクエストごとの切り替えは行わない。

| STORY_BUCKET の値 | 配信元 | ScriptStore 実装 |
|---|---|---|
| `op-scenario-scripts-prod` | GCS バケット | `adapter/gcs.ScriptStore` |
| `local:./testdata/stories` | ローカルファイルシステム | `adapter/local.ScriptStore` |

### スクリプト解決

`scenario_episodes.script_path` にはテンプレート (例: `stories/{lang}/she_ep1.ks`) が格納されている。

1. `{lang}` をリクエストの `lang` パラメータで置換
2. 配信元 (GCS or ローカル) からファイルを読み出す
3. ファイルが見つからなければフォールバック (後述)

## 言語フォールバック

リクエストされた言語のスクリプトが存在しない場合、`ja` にフォールバックする。

```
readScript(path_template, lang)
  │
  ├─ lang のファイルが存在 → 返却
  ├─ lang のファイルが不在 + lang == "ja" → ErrScriptNotFound (404)
  └─ lang のファイルが不在 + lang != "ja"
       ├─ "ja" のファイルが存在 → 返却 (フォールバック)
       └─ "ja" のファイルも不在 → ErrScriptNotFound (404)
```

フォールバックしたことをクライアントに通知するフラグは現時点では付与しない。

## エラー区別: ErrScriptNotFound vs ErrScriptInfra

スクリプト読み出しエラーは 2 つの sentinel error で区別する:

| sentinel | 意味 | HTTP | 発生条件 |
|---|---|---|---|
| `ErrScriptNotFound` | スクリプトファイルが存在しない | 404 | GCS: `storage.ErrObjectNotExist`、ローカル: `os.ErrNotExist` |
| `ErrScriptInfra` | 配信元のインフラ障害 | 500 | ネットワーク障害、権限不足、読み取り途中のエラー |

GCS モードでは `storage.ErrObjectNotExist` のみがフォールバックの対象。それ以外の GCS エラー (ネットワーク、権限) は即座に `ErrScriptInfra` として返し、フォールバックを試みない。

ローカルモードでは `os.ErrNotExist` のみがフォールバックの対象。

## アンロック判定

エピソードごとに 3 種類の条件をチェックする:

| 条件 | データソース | LockReason type |
|---|---|---|
| プレイヤーレベル | `players.level` (account の StoryUnlockContext 経由) | `level` |
| 所有ファクション | `player_factions` (account の StoryUnlockContext 経由) | `faction` |
| 前提エピソード完了 | `player_story_progress` (自スキーマ) | `episode` |

`StoryUnlockContext` は `StoryRepo.GetUnlockContext` で取得する。1 リクエストごとに 1 回 pull し、キャッシュはしない。

未達条件が 1 つでもあれば `LockReason` のリストを返す。全条件を満たせばアンロック済み。

## Pub/Sub publisher: faction-selected

### 契約

| 項目 | 値 |
|---|---|
| トピック | `faction-selected` |
| ペイロード型 | `FactionSelectedEvent` (`overload-party-common/packages/pubsub-events`) |
| source | `scenario_initial` (固定) |
| 配信保証 | Exactly-Once (subscriber 側) |

### トリガー

`StoryService.NotifyInitialFactionSelected(ctx, playerID, factionID)` が呼び出されたとき。チュートリアルのファクション確定エピソードの完了に紐づく (現在は `CompleteEpisode` 内の TODO(faction-handoff) として配線待ち)。

### publish 動作

- `publishJSON` で JSON 化し、`topic.Publish` で Pub/Sub に送信
- `result.Get(ctx)` で ACK を待つ (同期)。失敗は REST handler に伝搬し、クライアントのリトライ予算に委ねる
- 各 publish で新しい `event_id` (UUID) を生成。subscriber 側の `processed_events` で重複を排除

### subscriber 側の動作

- account: `player_factions` INSERT + `players.selected_faction` UPDATE
- card: `player_cards` にファクション + Neutral のカードを付与
- gateway: WS push で完了通知

scenario はこれらの subscriber を待たない。

## エラーハンドリング

- `DATABASE_URL` / `STORY_BUCKET` / `PUBSUB_PROJECT_ID` 未設定: 起動拒否 (fail-fast)
- `STORY_BUCKET=local:` (パス部分が空): 起動拒否
- Pub/Sub トピックが存在しない: 起動拒否
- `factionPublisher` が nil の状態で `NotifyInitialFactionSelected` が呼ばれた場合: 明示的なエラーを返す
- スクリプト配信元の障害: `ErrScriptInfra` -> 500 (フォールバックで継続しない)
- アンロック判定に必要な情報が取得できない: エラーを返す (フォールバック値で継続しない)
