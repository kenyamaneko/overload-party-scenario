package port

import "errors"

// ErrNotFound は対象行が存在しない場合にリポジトリが返すエラー。
var ErrNotFound = errors.New("not found")

// ErrScriptNotFound は要求言語のストーリースクリプトがストアに存在しない場合を示す。
var ErrScriptNotFound = errors.New("story script not found")

// ErrScriptInfra はストーリースクリプトストアが "not found" 以外のエラーを返した場合を示す。
var ErrScriptInfra = errors.New("story script infrastructure error")
