# セットアップ

## ローカル開発

スクリプトファイルの配信元は `STORY_BUCKET` で切り替え可能で、本番は GCS バケット名、開発は `local:<path>` 形式でローカルファイルシステムを指す。

`make run` はアプリ本体とインフラ (Postgres / Firestore / Pub/Sub emulator) を compose 内で起動する。
インフラはホストへ公開せず内部ネットワークのサービス名 DNS で参照するため、他リポのローカル
スタックやホスト上の他アプリとポートが衝突しない。ホストへ出るのは scenario の API ポート 9007 のみ。

```bash
make run      # アプリ + インフラを compose で起動（ソースをバインドマウント）
make down     # 停止して volume を削除
make test     # Testcontainers でテスト実行（Docker 必須）
```

アプリはコンテナ内で `go run` する。ソースを編集して `docker compose restart scenario` すれば、
イメージを作り直さずに反映される。プライベートモジュールはホストのモジュールキャッシュを読み取り専用でマウント
して解決するため、`make run` は先にホスト側で `go mod download` を実行する。

onboarding フローは account の内部 REST を呼ぶため、単体スタックでは story 系のみ動作する
(onboarding を通すには e2e スタックで account を含めて起動する)。
