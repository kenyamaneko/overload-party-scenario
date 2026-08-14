package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

func TestScriptStore(t *testing.T) {
	t.Run("[シナリオ]スクリプト読み込み", func(t *testing.T) {
		t.Run("指定キーのファイルが存在するとき、その本文が返る", func(t *testing.T) {
			root := t.TempDir()
			key := "sample/script.ks"
			require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(key)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, key), []byte("*start\n"), 0o644))
			store := NewScriptStore(root)

			body, err := store.ReadScript(context.Background(), key)

			require.NoError(t, err)
			assert.Equal(t, "*start\n", body)
		})

		t.Run("指定キーのファイルが存在しないとき、スクリプト未検出を表すエラーになる", func(t *testing.T) {
			root := t.TempDir()
			store := NewScriptStore(root)

			_, err := store.ReadScript(context.Background(), "sample/missing.ks")

			assert.ErrorIs(t, err, port.ErrScriptNotFound)
		})

		t.Run("ファイル読み込みが未検出以外の理由で失敗するとき、インフラエラーを表すエラーになる", func(t *testing.T) {
			root := t.TempDir()
			key := "sample/directory-as-file"
			require.NoError(t, os.MkdirAll(filepath.Join(root, key), 0o755))
			store := NewScriptStore(root)

			_, err := store.ReadScript(context.Background(), key)

			assert.ErrorIs(t, err, port.ErrScriptInfra)
		})
	})
}
