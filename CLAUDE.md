# CLAUDE.md - overload-party-scenario

## 行動制約

- エラーは握りつぶさない
- git tag を手動で打たない（CI が自動作成する）
- TODO スタブを追加しない
- クライアント認証を行わない（ClusterIP のみ、URL の playerId を信頼する）
- GCS エラーを握りつぶさない。日本語へのフォールバックは `ErrObjectNotExist` のみ。パーミッション / トランスポートエラーには適用しない
- 型変更時は `data/models.yaml` → `python3 scripts/generate_types.py` を実行する
