package apiscenarioclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kenyamaneko/overload-party-scenario/packages/api-scenario/apiscenarioclient"
	"github.com/kenyamaneko/overload-party-scenario/packages/api-scenario/apiscenarioserverfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StatusMapping 群は、SDK の固有責務である「OpenAPI spec で宣言された 4xx/5xx status を
// sentinel error に変換する」契約を endpoint ごとに検証する。GetHealth は 4xx/5xx 宣言が
// 無いため省略。GetOnboardingScript / UpdateOnboardingName / SelectOnboardingFaction /
// CompleteOnboarding は apiscenarioserverfake に handler が無く、status 変換 logic は他
// endpoint で十分カバーされるため省略。

func TestClient_ListEpisodes_StatusMapping(t *testing.T) {
	t.Run("ListEpisodes のステータスマッピング", func(t *testing.T) {
		t.Run("401 を受けたとき、ErrUnauthorized になる", func(t *testing.T) {
			srv := apiscenarioserverfake.NewServer()
			defer srv.Close()
			srv.ListEpisodesFn = func(_ string) (int, any) { return http.StatusUnauthorized, nil }

			c := newTestClient(t, srv.URL())
			_, err := c.ListEpisodes(context.Background(), "ja")
			assertSentinel(t, err, apiscenarioclient.ErrUnauthorized)
		})
	})
}

func TestClient_GetEpisodeScript_StatusMapping(t *testing.T) {
	t.Run("GetEpisodeScript のステータスマッピング", func(t *testing.T) {
		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apiscenarioclient.ErrUnauthorized,
			},
			{
				name:       "403 を受けたとき、ErrForbidden になる",
				status:     http.StatusForbidden,
				wantTarget: apiscenarioclient.ErrForbidden,
			},
			{
				name:       "404 を受けたとき、ErrNotFound になる",
				status:     http.StatusNotFound,
				wantTarget: apiscenarioclient.ErrNotFound,
			},
			{
				name:       "500 を受けたとき、ErrInternalServer になる",
				status:     http.StatusInternalServerError,
				wantTarget: apiscenarioclient.ErrInternalServer,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apiscenarioserverfake.NewServer()
				defer srv.Close()
				srv.GetScriptFn = func(_, _ string) (int, any) { return tc.status, nil }

				c := newTestClient(t, srv.URL())
				_, err := c.GetEpisodeScript(context.Background(), "ep1", "ja")
				assertSentinel(t, err, tc.wantTarget)
			})
		}
	})
}

func TestClient_CompleteEpisode_StatusMapping(t *testing.T) {
	t.Run("CompleteEpisode のステータスマッピング", func(t *testing.T) {
		cases := []struct {
			name       string
			status     int
			wantTarget error
		}{
			{
				name:       "401 を受けたとき、ErrUnauthorized になる",
				status:     http.StatusUnauthorized,
				wantTarget: apiscenarioclient.ErrUnauthorized,
			},
			{
				name:       "403 を受けたとき、ErrForbidden になる",
				status:     http.StatusForbidden,
				wantTarget: apiscenarioclient.ErrForbidden,
			},
			{
				name:       "404 を受けたとき、ErrNotFound になる",
				status:     http.StatusNotFound,
				wantTarget: apiscenarioclient.ErrNotFound,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				srv := apiscenarioserverfake.NewServer()
				defer srv.Close()
				srv.CompleteEpisodeFn = func(_ string) (int, any) { return tc.status, nil }

				c := newTestClient(t, srv.URL())
				_, err := c.CompleteEpisode(context.Background(), "ep1")
				assertSentinel(t, err, tc.wantTarget)
			})
		}
	})
}

func TestClient_RequestEditor(t *testing.T) {
	t.Run("リクエストエディタの適用", func(t *testing.T) {
		t.Run("WithRequestEditorFn で渡した editor が全リクエストに適用される", func(t *testing.T) {
			// X-Internal-Auth header 注入の接続点として SDK が機能することを担保する。
			var gotHeader string
			spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Get("X-Internal-Auth")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"episodes":[]}`))
			}))
			defer spy.Close()

			c, err := apiscenarioclient.New(spy.URL,
				apiscenarioclient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
					req.Header.Set("X-Internal-Auth", "test-token")
					return nil
				}),
			)
			require.NoError(t, err)

			_, err = c.ListEpisodes(context.Background(), "ja")
			require.NoError(t, err)
			assert.Equal(t, "test-token", gotHeader)
		})
	})
}

func newTestClient(t *testing.T, baseURL string) *apiscenarioclient.Client {
	t.Helper()
	c, err := apiscenarioclient.New(baseURL)
	require.NoError(t, err)
	return c
}

func assertSentinel(t *testing.T, gotErr, wantTarget error) {
	t.Helper()
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, wantTarget)
}
