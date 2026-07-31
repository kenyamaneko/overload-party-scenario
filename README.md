# overload-party-scenario

ストーリーエピソード・進行管理・スクリプト配信を担う内部マイクロサービス。ポート 9007 で起動する。

詳細は [機能仕様書](docs/FEATURE_SPEC.md) / [サービス設計書](docs/ARCHITECTURE.md) / [REST契約 (SSoT)](data/openapi.yaml) / [Pub/Sub契約 (SSoT)](data/asyncapi.yaml) / [データ設計書](docs/DATA_DESIGN.md) を参照。

[テスト観点カタログ](https://kenyamaneko.github.io/overload-party-scenario/): テスト名から生成した、テスト済みの観点の一覧。

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

`make run` はアプリ本体とインフラ (Postgres / Firestore / Pub/Sub emulator) を compose 内で起動する。
インフラはホストへ publish せず内部ネットワークのサービス名 DNS で参照するため、他リポのローカル
スタックやホスト上の他アプリとポートが衝突しない。ホストへ出るのは scenario の API ポート 9007 のみ。

```bash
make run      # アプリ + インフラを compose で起動（ソース bind-mount）
make down     # 停止して volume を削除
make test     # Testcontainers でテスト実行（Docker 必須）
```

アプリはコンテナ内で `go run` する。ソースを編集して `docker compose restart scenario` すれば、
イメージを作り直さずに反映される。private module は host の module cache を読み取り専用でマウント
して解決するため、`make run` は先に host 側で `go mod download` を実行する。

onboarding フローは account の internal REST を呼ぶため、単体スタックでは story 系のみ動作する
(onboarding を通すには e2e スタックで account を含めて起動する)。

## 公開パッケージ

[packages/api-scenario/](packages/api-scenario/) に REST / Pub/Sub 契約型を公開している。[data/openapi.yaml](data/openapi.yaml) または [data/asyncapi.yaml](data/asyncapi.yaml) を編集後に以下で再生成する。

```bash
make generate-types   # oapi-codegen + asyncapi-codegen を呼ぶ薄い wrapper
```
