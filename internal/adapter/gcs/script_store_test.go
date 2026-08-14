package gcs

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

const testBucketName = "tst-script-bucket"

func newTestGCSServer(t *testing.T, objects ...fakestorage.Object) *fakestorage.Server {
	t.Helper()
	// GCS クライアントのダウンロード経路は Host ヘッダが publicHost と一致するリクエストのみ受け付けるため、実アドレス確定後に publicHost を合わせる。
	srv, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		Scheme:         "http",
		InitialObjects: objects,
	})
	require.NoError(t, err)
	t.Cleanup(srv.Stop)

	hostport := strings.TrimPrefix(srv.URL(), "http://")
	req, err := http.NewRequest(http.MethodPut, srv.URL()+"/_internal/config",
		strings.NewReader(`{"publicHost":"`+hostport+`"}`))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	return srv
}

func newTestScriptStore(t *testing.T, srv *fakestorage.Server) *ScriptStore {
	t.Helper()
	t.Setenv("STORAGE_EMULATOR_HOST", srv.URL())
	client, err := storage.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return NewScriptStore(client, testBucketName)
}

func TestScriptStore(t *testing.T) {
	t.Run("[シナリオ]スクリプト読み込み", func(t *testing.T) {
		t.Run("指定キーのオブジェクトが存在するとき、その本文が返る", func(t *testing.T) {
			key := "sample/script.ks"
			srv := newTestGCSServer(t, fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: testBucketName, Name: key},
				Content:     []byte("*start\n"),
			})
			store := newTestScriptStore(t, srv)

			body, err := store.ReadScript(context.Background(), key)

			require.NoError(t, err)
			assert.Equal(t, "*start\n", body)
		})

		t.Run("指定キーのオブジェクトが存在しないとき、スクリプト未検出を表すエラーになる", func(t *testing.T) {
			srv := newTestGCSServer(t)
			srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: testBucketName})
			store := newTestScriptStore(t, srv)

			_, err := store.ReadScript(context.Background(), "sample/missing.ks")

			assert.ErrorIs(t, err, port.ErrScriptNotFound)
		})

		t.Run("オブジェクト読み込みが未検出以外の理由で失敗するとき、インフラエラーを表すエラーになる", func(t *testing.T) {
			key := "sample/script.ks"
			srv := newTestGCSServer(t, fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{BucketName: testBucketName, Name: key},
				Content:     []byte("*start\n"),
			})
			store := newTestScriptStore(t, srv)
			srv.Stop()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := store.ReadScript(ctx, key)

			assert.ErrorIs(t, err, port.ErrScriptInfra)
		})
	})
}
