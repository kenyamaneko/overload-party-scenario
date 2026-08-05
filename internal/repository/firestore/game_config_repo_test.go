//go:build integration

package firestore_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	scenariofirestore "github.com/kenyamaneko/overload-party-scenario/internal/repository/firestore"
)

func TestGameConfigRepository(t *testing.T) {
	// emulator を前提とする integration test。FIRESTORE_EMULATOR_HOST が未設定ならスキップする。
	host := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if host == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; skipping Firestore integration test")
	}

	const projectID = "overload-party-test"
	ctx := context.Background()

	resetEmulator(t, host, projectID)

	client, err := firestore.NewClient(ctx, projectID)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	_, err = client.Collection("game_config").Doc("exp_win").Set(ctx, map[string]any{"value": int64(40)})
	require.NoError(t, err)

	repo := scenariofirestore.NewGameConfigRepository(client)

	t.Run("game_configの取得", func(t *testing.T) {
		t.Run("存在するキーexp_winのとき、値40を返す", func(t *testing.T) {
			got, err := repo.GetInt64(ctx, "exp_win")
			require.NoError(t, err)
			assert.Equal(t, int64(40), got)
		})

		t.Run("存在しないキーのとき、ErrNotFoundになる", func(t *testing.T) {
			_, err := repo.GetInt64(ctx, "does_not_exist")
			require.Error(t, err)
			assert.True(t, errors.Is(err, port.ErrNotFound), "expected ErrNotFound, got: %v", err)
		})

		t.Run("設定値が数値でないとき、エラーになる", func(t *testing.T) {
			_, err := client.Collection("game_config").Doc("not_a_number").Set(ctx, map[string]any{"value": "abc"})
			require.NoError(t, err)

			_, err = repo.GetInt64(ctx, "not_a_number")
			require.Error(t, err)
		})

		t.Run("呼び出しがキャンセル済みのとき、ErrNotFound以外のエラーになる", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			_, err := repo.GetInt64(cancelCtx, "exp_win")
			require.Error(t, err)
			assert.False(t, errors.Is(err, port.ErrNotFound))
		})
	})
}

func resetEmulator(t *testing.T, host, projectID string) {
	t.Helper()
	url := fmt.Sprintf("http://%s/emulator/v1/projects/%s/databases/(default)/documents", host, projectID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("firestore emulator reset failed: status=%d body=%s", resp.StatusCode, string(body))
	}
}
