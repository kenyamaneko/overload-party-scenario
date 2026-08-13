//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// countProgress は指定プレイヤー・エピソードの scenario.player_story_progress 行数を返す。
func countProgress(t *testing.T, playerID, episodeID string) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.player_story_progress WHERE player_id = $1 AND episode_id = $2`,
		playerID, episodeID,
	).Scan(&n))
	return n
}

// strPtr は string リテラルへのポインタを返す短縮ヘルパ。
func strPtr(s string) *string { return &s }

// seedOutboxEvent は writeOutboxEvent が非公開のため、claim 対象判定の境界を直接 INSERT で再現するヘルパ。
func seedOutboxEvent(
	t *testing.T,
	eventID uuid.UUID,
	eventType string,
	payload []byte,
	createdAt time.Time,
	lastAttemptedAt *time.Time,
	failureCount int,
	publishedAt *time.Time,
) {
	t.Helper()
	_, err := sharedPg.Pool.Exec(context.Background(),
		`INSERT INTO scenario.outbox_events
		   (event_id, event_type, payload, created_at, last_attempted_at, failure_count, published_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		eventID, eventType, payload, createdAt, lastAttemptedAt, failureCount, publishedAt)
	require.NoError(t, err)
}

// outboxEventRow は scenario.outbox_events 1 行分の読み取り結果。
type outboxEventRow struct {
	EventType       string
	Payload         []byte
	FailureCount    int
	PublishedAt     *time.Time
	LastAttemptedAt *time.Time
	LastError       *string
}

// findOutboxEvent は event_id を指定して scenario.outbox_events の 1 行を取得する。
func findOutboxEvent(t *testing.T, eventID uuid.UUID) (outboxEventRow, bool) {
	t.Helper()
	var row outboxEventRow
	err := sharedPg.Pool.QueryRow(context.Background(),
		`SELECT event_type, payload, failure_count, published_at, last_attempted_at, last_error
		   FROM scenario.outbox_events WHERE event_id = $1`,
		eventID,
	).Scan(&row.EventType, &row.Payload, &row.FailureCount, &row.PublishedAt, &row.LastAttemptedAt, &row.LastError)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outboxEventRow{}, false
		}
		require.NoError(t, err)
	}
	return row, true
}

// countOutboxEvents は scenario.outbox_events の全行数を返す。
func countOutboxEvents(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scenario.outbox_events`).Scan(&n))
	return n
}

// isOnboarded は指定プレイヤーの scenario.player_onboarding 行が存在するかを返す。
func isOnboarded(t *testing.T, playerID string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, sharedPg.Pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM scenario.player_onboarding WHERE player_id = $1)`,
		playerID,
	).Scan(&exists))
	return exists
}
