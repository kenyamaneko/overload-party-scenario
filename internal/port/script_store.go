package port

import "context"

// ScriptStore はストレージからストーリースクリプトファイルを読み込む。
type ScriptStore interface {
	// ReadScript は指定キーのスクリプト本文を読み込む (不在は ErrScriptNotFound、それ以外は ErrScriptInfra)。
	ReadScript(ctx context.Context, key string) (string, error)
}
