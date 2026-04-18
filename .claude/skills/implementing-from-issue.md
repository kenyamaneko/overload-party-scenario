---
name: implementing-from-issue
description: GitHub Issue を起点に feature ブランチを切って実装・PR 作成まで行う手順。ユーザーが「Issue xx をやって」「この Issue を対応」と依頼したときに使う。
---

# Issue を元に実装する

GitHub Issue を起点に、feature ブランチを切り、テストファーストで実装し、develop への PR を作成するまでの手順。

## 手順

1. Issue の内容を確認する
   - 背景・受け入れ基準・関連リンクを読み、何が「完了」かを把握する
   - 対象エピソード / script / ファクション / 言語 / アンロック条件など、影響範囲を特定する
   - 方針に迷いがあれば**着手前に**ユーザーに相談する(実装後の手戻りを避ける)
2. `develop` から feature ブランチを切る
   - 命名: `feature/{issue番号}-{概要}` (例: `feature/12-episode-unlock-rule`)
   - 最新の `develop` を取得してから切る: `git fetch origin && git switch -c feature/{n}-{summary} origin/develop`
3. 必要に応じてドキュメントを更新する
   - 仕様変更: `docs/FEATURE_SPEC.md` / `docs/API_REFERENCE.md`
   - 設計レベルの意図: `docs/ARCHITECTURE.md`
   - データモデル: `data/models.yaml` (変更後 `python3 scripts/generate_types.py` で再生成)
4. テストファーストで実装する
   - 受け入れ基準を Given/When/Then に落としてテストを書いてから実装する
   - テスト方針・設計思想・コーディング方針は [CLAUDE.md](../../CLAUDE.md) に従う
   - GCS / Pub/Sub など外部依存はインターフェース越しに fake 差し替えできるようにする
5. コミット・プッシュする
   - コミットメッセージ規則は [CLAUDE.md](../../CLAUDE.md) の「ブランチ・Issue 運用」に従う
   - **コミット前にユーザーの承認を得る**: 変更内容(`git diff` の要点)とコミットメッセージ案をユーザーに提示し、修正内容が妥当か・メッセージが適切かを確認してから `git commit` を実行する
   - `git push -u origin feature/{n}-{summary}`
6. `develop` への PR を作成する
   - タイトルは 70 文字以内。本文は「Summary」「Test plan」を含める
   - Issue 番号を本文に記載する (例: `Closes #12`)
7. ユーザーに PR URL を通知する

## 確認事項

着手前に以下が不明なら**推測せず質問する**:

- Issue の受け入れ基準が読み取れない / 複数解釈できる
- 影響範囲が想定より広い(他サービス・GCS オブジェクト配置・Pub/Sub トピック・公開 API に及ぶ)
- ストーリー / script / アンロック条件の仕様変更でゲーム進行に影響するか判断できない
- ドキュメント更新の要否が判断できない
