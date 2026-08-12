//go:build integration

package firestore_test

import (
	"context"
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

const gameConfigTestProjectID = "overload-party-test"

func TestGameConfigRepository(t *testing.T) {
	ctx := context.Background()

	resetFirestoreEmulator(t, os.Getenv("FIRESTORE_EMULATOR_HOST"), gameConfigTestProjectID)

	client, err := firestore.NewClient(ctx, gameConfigTestProjectID)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	repo := scenariofirestore.NewGameConfigRepository(client)

	t.Run("[ゲーム設定]ゲーム設定値の取得", func(t *testing.T) {
		t.Run("設定が存在するキーを指定したとき、保存された値が返る", func(t *testing.T) {
			_, err := client.Collection("game_config").Doc("TST-0001").Set(ctx, map[string]any{"value": int64(40)})
			require.NoError(t, err)

			got, err := repo.GetInt64(ctx, "TST-0001")
			require.NoError(t, err)
			assert.Equal(t, int64(40), got)
		})

		t.Run("設定が存在しないキーを指定したとき、未検出を表すエラーになる", func(t *testing.T) {
			_, err := repo.GetInt64(ctx, "TST-0002")
			require.Error(t, err)
			assert.ErrorIs(t, err, port.ErrNotFound)
		})

		t.Run("ドキュメントの値が数値としてパースできない形式のとき、ゲーム設定値のパースエラーになる", func(t *testing.T) {
			_, err := client.Collection("game_config").Doc("TST-0003").Set(ctx, map[string]any{"value": "not-a-number"})
			require.NoError(t, err)

			_, err = repo.GetInt64(ctx, "TST-0003")
			require.Error(t, err)
			assert.ErrorContains(t, err, "parse game config")
		})

		t.Run("Firestoreへのアクセス自体が未検出以外のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			_, err := repo.GetInt64(cancelCtx, "TST-0004")
			require.Error(t, err)
			assert.ErrorContains(t, err, "context canceled")
		})
	})
}

func resetFirestoreEmulator(t *testing.T, host, projectID string) {
	t.Helper()
	url := fmt.Sprintf("http://%s/emulator/v1/projects/%s/databases/(default)/documents", host, projectID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			t.Fatalf("firestore emulator reset failed: status=%d (failed to read body: %v)", resp.StatusCode, readErr)
		}
		t.Fatalf("firestore emulator reset failed: status=%d body=%s", resp.StatusCode, string(body))
	}
}
