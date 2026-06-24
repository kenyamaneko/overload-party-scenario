package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedEpisode は scenario.scenario_episodes に最低限のレコードを投入する。
// faction は NULL 可 (全陣営共通)、required_episodes は空配列デフォルト。
func seedEpisode(
	t *testing.T,
	episodeID string,
	faction *string,
	episodeNumber int64,
	titleJa, titleEn string,
	requiredLevel int64,
	requiredEpisodes []string,
	scriptPath string,
	sortOrder int64,
	isActive bool,
) {
	t.Helper()
	// NOT NULL 制約のある TEXT[] カラムに nil スライスを渡すと NULL になるため
	// 必ず空スライスに正規化する。
	reqEps := requiredEpisodes
	if reqEps == nil {
		reqEps = []string{}
	}
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.scenario_episodes
		   (episode_id, category, faction, episode_number, title_ja, title_en,
		    required_level, required_episodes, script_path, sort_order, is_active)
		 VALUES ($1, 'main', $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		episodeID, faction, episodeNumber, titleJa, titleEn,
		requiredLevel, reqEps, scriptPath, sortOrder, isActive)
	require.NoError(t, err)
}

// seedRequiredFaction は scenario.episode_required_factions に必須陣営を登録する。
func seedRequiredFaction(t *testing.T, episodeID, factionID string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.episode_required_factions (episode_id, faction_id)
		 VALUES ($1, $2)`,
		episodeID, factionID)
	require.NoError(t, err)
}

// seedProgress は scenario.player_story_progress に完了済みフラグを立てる。
func seedProgress(t *testing.T, playerID, episodeID string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_story_progress (player_id, episode_id)
		 VALUES ($1, $2)`,
		playerID, episodeID)
	require.NoError(t, err)
}

// seedPlayer は account stub の scenario.players / player_progression にレコードを投入する。
func seedPlayer(t *testing.T, playerID string, level int64) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.players (player_id) VALUES ($1)`, playerID)
	require.NoError(t, err)
	_, err = sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_progression (player_id, level) VALUES ($1, $2)`,
		playerID, level)
	require.NoError(t, err)
}

// seedPlayerFaction は account stub の scenario.player_factions にレコードを投入する。
func seedPlayerFaction(t *testing.T, playerID, faction string) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.player_factions (player_id, faction) VALUES ($1, $2)`,
		playerID, faction)
	require.NoError(t, err)
}

// strPtr は string リテラルへのポインタを返す短縮ヘルパ。
func strPtr(s string) *string { return &s }
