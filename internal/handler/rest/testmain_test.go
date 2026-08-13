//go:build integration

package rest

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	apiaccount "github.com/kenyamaneko/overload-party-account/packages/api-account"
	"github.com/kenyamaneko/overload-party-account/packages/api-account/apiaccountserverfake"

	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
)

var sharedPg *postgrestest.Postgres

// TestMain は scenario schema を適用した PostgreSQL コンテナをパッケージ共有で起動する。
func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	))
}

// newAccountFake は account を HTTP 境界で偽サーバに差し替え、プレイヤーの到達状況を固定応答にする。
func newAccountFake(t *testing.T, level int64) *apiaccountserverfake.Server {
	t.Helper()
	srv := apiaccountserverfake.NewServer()
	srv.GetPlayerFn = func() (int, any) {
		return http.StatusOK, apiaccount.PlayerResponse{PlayerID: contractPlayerID, Level: level}
	}
	srv.ListFactionsFn = func() (int, any) {
		return http.StatusOK, apiaccount.FactionListing{Factions: []string{}}
	}
	t.Cleanup(srv.Close)
	return srv
}

// seedEpisode は 1 エピソードをシードする。
func seedEpisode(t *testing.T, episodeID string, requiredLevel int64, scriptPath string, isActive bool) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.scenario_episodes
		   (episode_id, category, faction, episode_number, title_ja, title_en,
		    required_level, required_episodes, script_path, sort_order, is_active)
		 VALUES ($1, 'main', NULL, 1, 'タイトル', 'Title', $2, '{}', $3, 1, $4)`,
		episodeID, requiredLevel, scriptPath, isActive)
	require.NoError(t, err)
}
