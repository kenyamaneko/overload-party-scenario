# overload-party-scenario

ストーリーエピソード・進行管理・スクリプト配信を担う内部マイクロサービス。ポート 9007 で起動する。

詳細は [機能仕様書](docs/FEATURE_SPEC.md) / [サービス設計書](docs/ARCHITECTURE.md) / [REST契約 (SSoT)](data/openapi.yaml) / [Pub/Sub契約 (SSoT)](data/asyncapi.yaml) / [データ設計書](docs/DATA_DESIGN.md) を参照。

## アーキテクチャ概要

```
Gateway
  └─ Scenario (:9007)
       ├─ PostgreSQL (scenario スキーマ)
       ├─ GCS または local: ファイルシステム (script 配信)
       ├─ Firestore (game_config 読み取り)
       └─ Pub/Sub
            ├─ onboarding-name-set     → account
            ├─ onboarding-faction-set  → account
            └─ player-onboarded        → account / card / gateway
```

サービス間の状態同期は Pub/Sub で fan-out し、scenario から他サービスを直接呼び出さない。スクリプトファイルの配信元は `STORY_BUCKET` で切り替え可能で、本番は GCS バケット名、開発は `local:<path>` 形式でローカルファイルシステムを指す。

## ローカル開発

```bash
make db-up    # postgres:16-alpine を起動
make run      # サーバー起動（db-upと環境変数の注入を含む）
make test     # Testcontainers でテスト実行（Docker 必須）
make db-down  # 停止
make db-reset # volume ごと削除して再作成
```

## 公開パッケージ

[packages/api-scenario/](packages/api-scenario/) に REST / Pub/Sub 契約型を公開している。[data/openapi.yaml](data/openapi.yaml) または [data/asyncapi.yaml](data/asyncapi.yaml) を編集後に以下で再生成する。

```bash
make generate-types   # oapi-codegen + asyncapi-codegen を呼ぶ薄い wrapper
```
