# overload-party-scenario

ストーリーエピソード / 進行管理 / スクリプト配信を担う内部マイクロサービス。

## サービス間連携

```
Gateway (唯一の呼び出し元)
  ├─ GET  /players/:playerId/scenarios
  ├─ GET  /players/:playerId/scenarios/:episodeId/script
  └─ POST /players/:playerId/scenarios/:episodeId/complete
                │
                ▼
Scenario (このサービス, :9007)
  ├─ PostgreSQL (scenario スキーマ所有)
  ├─ GCS or ローカルファイルシステム (スクリプト配信)
  └─ Pub/Sub publisher
       └─ faction-selected → account / card / gateway が subscribe
```

- 認証はしない。Gateway が playerId を forward する
- スクリプトファイルの配信元は `STORY_BUCKET` で切り替え: GCS バケット名 (本番) / `local:<path>` (開発)
- 初回ファクション選択時に faction-selected イベントを Pub/Sub に publish する

エンドポイント一覧は [docs/API_REFERENCE.md](docs/API_REFERENCE.md) を参照。

## 環境変数

**Secret:**

| 変数名 | 説明 |
|---|---|
| `DATABASE_URL` | PostgreSQL 接続文字列 |

**ConfigMap:**

| 変数名 | デフォルト | 説明 |
|---|---|---|
| `PORT` | `9007` | リッスンポート |
| `ENV` | `dev` | `dev` / `stg` / `prod` |
| `STORY_BUCKET` | (必須) | GCS バケット名、または `local:<path>` (開発時) |
| `PUBSUB_PROJECT_ID` | (必須) | Pub/Sub GCP プロジェクト |
| `FACTION_SELECTED_TOPIC` | `faction-selected` | faction-selected Pub/Sub トピック名 |

`DATABASE_URL` / `STORY_BUCKET` / `PUBSUB_PROJECT_ID` が未設定なら起動時に即 fail する。

## 公開パッケージ

| パッケージ | パス | 用途 |
|---|---|---|
| Go module | `packages/api-scenario/` | REST 契約型 (`apiscenario.EpisodeWithStatus` 等) |

SSoT: `data/models.yaml` -> `python3 scripts/generate_types.py` で再生成。

クライアント向け TypeScript 型は `@kenyamaneko/overload-party-api-gateway` に統合済み。
