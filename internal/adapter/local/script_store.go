package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// ScriptStore はローカルファイルシステムのディレクトリからストーリースクリプトを読み込む。
type ScriptStore struct {
	root string
}

// NewScriptStore は指定ディレクトリを使用する ScriptStore を構築する。
func NewScriptStore(root string) *ScriptStore {
	return &ScriptStore{root: root}
}

// ReadScript はローカルファイルシステムから指定キーのスクリプトを読み込む。
func (s *ScriptStore) ReadScript(_ context.Context, key string) (string, error) {
	fullPath := filepath.Join(s.root, key)
	data, err := os.ReadFile(fullPath)
	if err == nil {
		return string(data), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: local root=%q path=%q",
			port.ErrScriptNotFound, s.root, fullPath)
	}
	return "", fmt.Errorf("%w: local read (root=%q path=%q): %v",
		port.ErrScriptInfra, s.root, fullPath, err)
}
