package apiscenarioserverfake_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/packages/api-scenario/apiscenarioserverfake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("サーバフェイク", func(t *testing.T) {
		t.Run("Fn未設定のendpointは既定応答を返す", func(t *testing.T) {
			// consumer テストが setup を書かなくても最低限の HTTP 応答が得られる契約。
			tests := []struct {
				name       string
				method     string
				path       string
				wantStatus int
				wantBody   string
			}{
				{
					name:       "ListEpisodesは200で空配列を返す",
					method:     http.MethodGet,
					path:       "/api/v1/scenarios/episodes",
					wantStatus: http.StatusOK,
					wantBody:   `{"episodes":[]}` + "\n",
				},
				{
					name:       "GetScriptは200で空スクリプトを返す",
					method:     http.MethodGet,
					path:       "/api/v1/scenarios/episodes/ep-1/script",
					wantStatus: http.StatusOK,
					wantBody:   `{"episode_id":"ep-1","script":""}` + "\n",
				},
				{
					name:       "CompleteEpisodeは204 No Contentを返す",
					method:     http.MethodPost,
					path:       "/api/v1/scenarios/episodes/ep-1/complete",
					wantStatus: http.StatusNoContent,
					wantBody:   "",
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					srv := apiscenarioserverfake.NewServer()
					defer srv.Close()

					req, _ := http.NewRequest(tt.method, srv.URL()+tt.path, nil)
					resp, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					defer resp.Body.Close()

					body, _ := io.ReadAll(resp.Body)
					assert.Equal(t, tt.wantStatus, resp.StatusCode)
					assert.Equal(t, tt.wantBody, string(body))
				})
			}
		})

		t.Run("ListEpisodesFnはlang queryを受け取って応答を制御できる", func(t *testing.T) {
			srv := apiscenarioserverfake.NewServer()
			defer srv.Close()

			var gotLang string
			srv.ListEpisodesFn = func(lang string) (int, any) {
				gotLang = lang
				return http.StatusOK, apiscenarioserverfake.ListEpisodesResponse{
					Episodes: []apiscenario.EpisodeWithStatus{
						{EpisodeID: "ep-1", Title: "Prologue"},
					},
				}
			}

			resp, err := http.Get(srv.URL() + "/api/v1/scenarios/episodes?lang=ja")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "ja", gotLang)

			var decoded apiscenarioserverfake.ListEpisodesResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
			require.Len(t, decoded.Episodes, 1)
			assert.Equal(t, "ep-1", decoded.Episodes[0].EpisodeID)
		})

		t.Run("GetScriptFnはstatus codeだけでerror応答 (bodyなし)を返せる", func(t *testing.T) {
			// scenarioclient は 404/403 を status code だけで判定するため body 無しで十分。
			srv := apiscenarioserverfake.NewServer()
			defer srv.Close()

			srv.GetScriptFn = func(_, _ string) (int, any) {
				return http.StatusForbidden, nil
			}

			resp, err := http.Get(srv.URL() + "/api/v1/scenarios/episodes/ep-locked/script")
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
			assert.Empty(t, body, "body=nil の場合は空応答")
		})

		t.Run("CompleteEpisodeFnはpath variableを受け取って応答を決定する", func(t *testing.T) {
			srv := apiscenarioserverfake.NewServer()
			defer srv.Close()

			var gotEpisodeID string
			srv.CompleteEpisodeFn = func(episodeID string) (int, any) {
				gotEpisodeID = episodeID
				return http.StatusNoContent, nil
			}

			req, _ := http.NewRequest(http.MethodPost,
				srv.URL()+"/api/v1/scenarios/episodes/ep-9/complete", nil)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			assert.Equal(t, "ep-9", gotEpisodeID)
		})
	})
}
