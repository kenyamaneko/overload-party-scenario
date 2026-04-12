package port

import "context"

// ScriptStore はストレージ（GCS、ローカルファイルシステム等）からストーリースクリプト
// ファイルを読み込む。各実装はネイティブの "not found" を ErrScriptNotFound に、
// それ以外の障害を ErrScriptInfra にマッピングする。
type ScriptStore interface {
	// ReadScript は指定キーのスクリプト本文を読み込む。
	// オブジェクトが存在しない場合は ErrScriptNotFound を返す。
	// それ以外のストレージレベルのエラーは ErrScriptInfra を返す。
	ReadScript(ctx context.Context, key string) (string, error)
}
