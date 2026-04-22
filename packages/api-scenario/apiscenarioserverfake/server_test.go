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

// 各 endpoint は Fn が未設定なら既定応答を返す。consumer テストが setup を
// 書かなくても最低限の HTTP 応答が得られる契約を固定する。
func TestServer_DefaultResponses(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "ListEpisodes 既定は空配列",
			method:     http.MethodGet,
			path:       "/internal/v1/players/p-1/scenarios",
			wantStatus: http.StatusOK,
			wantBody:   `{"episodes":[]}` + "\n",
		},
		{
			name:       "GetScript 既定は空スクリプト",
			method:     http.MethodGet,
			path:       "/internal/v1/players/p-1/scenarios/ep-1/script",
			wantStatus: http.StatusOK,
			wantBody:   `{"episode_id":"ep-1","script":""}` + "\n",
		},
		{
			name:       "CompleteEpisode 既定は 204 No Content",
			method:     http.MethodPost,
			path:       "/internal/v1/players/p-1/scenarios/ep-1/complete",
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
}

// Fn callback が path variable と query を受け取って応答を制御できる。
// consumer テストが「どの player で呼ばれたか」を検証したり、ケース毎に
// 異なる応答 (成功 / 失敗) を返すための API。
func TestServer_ListEpisodesFn_ReceivesPathAndQuery(t *testing.T) {
	srv := apiscenarioserverfake.NewServer()
	defer srv.Close()

	var gotPlayerID, gotLang string
	srv.ListEpisodesFn = func(playerID, lang string) (int, any) {
		gotPlayerID = playerID
		gotLang = lang
		return http.StatusOK, apiscenarioserverfake.ListEpisodesResponse{
			Episodes: []apiscenario.EpisodeWithStatus{
				{EpisodeID: "ep-1", Title: "Prologue"},
			},
		}
	}

	resp, err := http.Get(srv.URL() + "/internal/v1/players/p-1/scenarios?lang=ja")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "p-1", gotPlayerID)
	assert.Equal(t, "ja", gotLang)

	var decoded apiscenarioserverfake.ListEpisodesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	require.Len(t, decoded.Episodes, 1)
	assert.Equal(t, "ep-1", decoded.Episodes[0].EpisodeID)
}

// Fn callback から error 応答 (status code + body=nil) を返せる。scenarioclient
// は 404/403 を status code だけで判定するので body 無しで十分という契約を固定する。
func TestServer_GetScriptFn_CanReturnError(t *testing.T) {
	srv := apiscenarioserverfake.NewServer()
	defer srv.Close()

	srv.GetScriptFn = func(_, _, _ string) (int, any) {
		return http.StatusForbidden, nil
	}

	resp, err := http.Get(srv.URL() + "/internal/v1/players/p-1/scenarios/ep-locked/script")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, body, "body=nil の場合は空応答")
}

// CompleteEpisode の Fn が path variable を受け取って応答を決定できる。
func TestServer_CompleteEpisodeFn_ReceivesPath(t *testing.T) {
	srv := apiscenarioserverfake.NewServer()
	defer srv.Close()

	var gotPlayerID, gotEpisodeID string
	srv.CompleteEpisodeFn = func(playerID, episodeID string) (int, any) {
		gotPlayerID = playerID
		gotEpisodeID = episodeID
		return http.StatusNoContent, nil
	}

	req, _ := http.NewRequest(http.MethodPost,
		srv.URL()+"/internal/v1/players/p-9/scenarios/ep-9/complete", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "p-9", gotPlayerID)
	assert.Equal(t, "ep-9", gotEpisodeID)
}
