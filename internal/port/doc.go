// Package port は usecase 層が依存する interface 群 (永続化リポジトリ・
// outbox イベント publisher・script ストア・account クライアント等) と
// 共有エラーセンチネルを定義する。
//
// Why: account 等の外部サービスへのアクセスは、用途別の狭い interface としてのみ公開する。
// 汎用クライアントを usecase 層に渡すと、用途を宣言しないアクセスを構造的に防げなくなるため。
package port
