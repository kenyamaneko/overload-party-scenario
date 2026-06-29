//go:build integration

package rest

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres/postgrestest"
)

var sharedPg *postgrestest.Postgres

// TestMain は scenario schema を適用した PostgreSQL コンテナをパッケージ共有で起動する。
func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchemaFile("internal/repository/postgres/testdata/account_stub.sql"),
		postgrestest.WithSchema("scenario"),
		postgrestest.WithSearchPath("scenario", "public"),
	))
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

// seedPlayer は 1 プレイヤーをシードする。
func seedPlayer(t *testing.T, playerID string, level int64) {
	t.Helper()
	ctx := context.Background()
	_, err := sharedPg.Pool.Exec(ctx, `INSERT INTO scenario.players (player_id) VALUES ($1)`, playerID)
	require.NoError(t, err)
	_, err = sharedPg.Pool.Exec(ctx,
		`INSERT INTO scenario.player_progression (player_id, level) VALUES ($1, $2)`, playerID, level)
	require.NoError(t, err)
}
