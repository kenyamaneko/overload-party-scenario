package port

import "errors"

// ErrNotFound は対象行が存在しない場合にリポジトリが返すエラー。
var ErrNotFound = errors.New("not found")

// ErrScriptNotFound は要求言語のストーリースクリプトがストアに存在しない場合を示す。
// 代替言語へのフォールバックは行わない。ハンドラは HTTP 404 に変換する。
var ErrScriptNotFound = errors.New("story script not found")

// ErrScriptInfra はストーリースクリプトストア（GCS またはローカルファイルシステム）が
// "not found" 以外のエラー（ネットワーク障害、権限不足、読み取り中断等）を返した場合を示す。
// ハンドラは HTTP 500 に変換する。具体的な原因は %w でラップされログに記録される。
var ErrScriptInfra = errors.New("story script infrastructure error")
