// Package apiscenarioserverfake は scenario サービスの HTTP 契約を実装する
// httptest.Server ラッパー。consumer (gateway 等) が scenarioclient を使う
// handler テストで、実 scenario サービスを起動せずに REST 呼び出しを検証するための
// テストダブルを提供する。
//
// 各 endpoint は Fn field (func callback) で status + response body を制御する。
// Fn が nil の endpoint は既定値 (200 OK + 空 body) を返す。
//
// JSON response shape は scenarioclient が期待する形式に合わせる。happy path の
// response 型 (ListEpisodesResponse, ScriptResponse) は本パッケージで export し、
// テスト側が型付きで response を組み立てられるようにする。
package apiscenarioserverfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// Server は scenario HTTP 契約を実装する httptest.Server wrapper。
// テスト側は NewServer で起動し、Fn field で endpoint 毎の応答を設定し、
// scenarioclient.New(server.URL()) を通じて検証を行う。
type Server struct {
	mu  sync.Mutex
	srv *httptest.Server

	// ListEpisodesFn は GET /internal/v1/players/{playerID}/scenarios の応答を
	// 決定する callback。返り値 (status, body) がそのまま HTTP response になる。
	// body が nil の場合 body 無し、それ以外は json.Marshal された bytes を返す。
	// nil (未設定) の場合は既定値 200 + {"episodes": []} を返す。
	ListEpisodesFn func(playerID, lang string) (int, any)

	// GetScriptFn は GET /internal/v1/players/{playerID}/scenarios/{episodeID}/script の応答を決定する。
	// nil の場合は既定値 200 + {"episode_id": episodeID, "script": ""} を返す。
	GetScriptFn func(playerID, episodeID, lang string) (int, any)

	// CompleteEpisodeFn は POST /internal/v1/players/{playerID}/scenarios/{episodeID}/complete の応答を決定する。
	// nil の場合は既定値 204 No Content を返す。
	CompleteEpisodeFn func(playerID, episodeID string) (int, any)
}

// ListEpisodesResponse は ListEpisodes endpoint の JSON envelope。
// scenarioclient 側の private 型と shape が一致している必要があるため、
// テストから typed で組み立てられるよう本パッケージで export する。
type ListEpisodesResponse struct {
	Episodes []apiscenario.EpisodeWithStatus `json:"episodes"`
}

// ScriptResponse は GetScript endpoint の JSON 形。
type ScriptResponse struct {
	EpisodeID string `json:"episode_id"`
	Script    string `json:"script"`
}

// NewServer は起動済み Server を返す。URL() で base URL を取得し
// scenarioclient.New(server.URL()) に渡して利用する。
// テスト終了時に Close() すること。
func NewServer() *Server {
	s := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/v1/players/{playerID}/scenarios", s.handleListEpisodes)
	mux.HandleFunc("GET /internal/v1/players/{playerID}/scenarios/{episodeID}/script", s.handleGetScript)
	mux.HandleFunc("POST /internal/v1/players/{playerID}/scenarios/{episodeID}/complete", s.handleCompleteEpisode)
	s.srv = httptest.NewServer(mux)
	return s
}

// URL は httptest.Server のベース URL を返す。
func (s *Server) URL() string { return s.srv.URL }

// Close は内部 httptest.Server を閉じる。
func (s *Server) Close() { s.srv.Close() }

func (s *Server) handleListEpisodes(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.ListEpisodesFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	lang := r.URL.Query().Get("lang")

	if fn == nil {
		writeJSON(w, http.StatusOK, ListEpisodesResponse{Episodes: []apiscenario.EpisodeWithStatus{}})
		return
	}
	status, body := fn(playerID, lang)
	writeJSON(w, status, body)
}

func (s *Server) handleGetScript(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.GetScriptFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	episodeID := r.PathValue("episodeID")
	lang := r.URL.Query().Get("lang")

	if fn == nil {
		writeJSON(w, http.StatusOK, ScriptResponse{EpisodeID: episodeID, Script: ""})
		return
	}
	status, body := fn(playerID, episodeID, lang)
	writeJSON(w, status, body)
}

func (s *Server) handleCompleteEpisode(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	fn := s.CompleteEpisodeFn
	s.mu.Unlock()

	playerID := r.PathValue("playerID")
	episodeID := r.PathValue("episodeID")

	if fn == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	status, body := fn(playerID, episodeID)
	writeJSON(w, status, body)
}

// writeJSON は status code を書き、body が非 nil なら Content-Type: application/json
// で JSON encode して送る。body が nil の場合は body 無しでレスポンスを終わる
// (scenarioclient は 404/403 等の body を読まないため、エラー応答では nil で良い)。
func writeJSON(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
