package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

var _ port.StoryRepo = (*PgStoryRepository)(nil)

// PgStoryRepository は PostgreSQL で StoryRepo を実装する。
type PgStoryRepository struct {
	pool *pgxpool.Pool
}

// NewPgStoryRepository は pgxpool.Pool を受け取り PgStoryRepository を構築する。
func NewPgStoryRepository(pool *pgxpool.Pool) *PgStoryRepository {
	return &PgStoryRepository{pool: pool}
}

// ListActiveEpisodes はアクティ��なエピソード一覧を返す。
func (r *PgStoryRepository) ListActiveEpisodes(ctx context.Context) ([]*apiscenario.ScenarioEpisode, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT episode_id, faction, episode_number, title_ja, title_en,
		        required_level, required_factions, required_episodes,
		        script_path, thumbnail_path, sort_order, is_active, created_at
		 FROM scenario_episodes
		 WHERE is_active = true
		 ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("query episodes: %w", err)
	}
	defer rows.Close()

	var episodes []*apiscenario.ScenarioEpisode
	for rows.Next() {
		var ep apiscenario.ScenarioEpisode
		if err := rows.Scan(
			&ep.EpisodeID, &ep.Faction, &ep.EpisodeNumber, &ep.TitleJa, &ep.TitleEn,
			&ep.RequiredLevel, &ep.RequiredFactions, &ep.RequiredEpisodes,
			&ep.ScriptPath, &ep.ThumbnailPath, &ep.SortOrder, &ep.IsActive, &ep.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		episodes = append(episodes, &ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate episodes: %w", err)
	}
	return episodes, nil
}

// FindEpisodeByID は指定 ID のエピソードを返す。
func (r *PgStoryRepository) FindEpisodeByID(ctx context.Context, episodeID string) (*apiscenario.ScenarioEpisode, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT episode_id, faction, episode_number, title_ja, title_en,
		        required_level, required_factions, required_episodes,
		        script_path, thumbnail_path, sort_order, is_active, created_at
		 FROM scenario_episodes
		 WHERE episode_id = $1`,
		episodeID)

	var ep apiscenario.ScenarioEpisode
	err := row.Scan(
		&ep.EpisodeID, &ep.Faction, &ep.EpisodeNumber, &ep.TitleJa, &ep.TitleEn,
		&ep.RequiredLevel, &ep.RequiredFactions, &ep.RequiredEpisodes,
		&ep.ScriptPath, &ep.ThumbnailPath, &ep.SortOrder, &ep.IsActive, &ep.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("episode %s: %w", episodeID, port.ErrNotFound)
		}
		return nil, fmt.Errorf("query episode by id: %w", err)
	}
	return &ep, nil
}

// GetCompletedEpisodeIDs はプレイヤーの完了済みエピソード ID 一覧を返す。
func (r *PgStoryRepository) GetCompletedEpisodeIDs(ctx context.Context, playerID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT episode_id FROM player_story_progress WHERE player_id = $1`,
		playerID)
	if err != nil {
		return nil, fmt.Errorf("query completed episodes: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan episode id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate completed episodes: %w", err)
	}
	return ids, nil
}

// GetUnlockContext は players, player_factions, player_story_progress を結合して返す。
// 暫定: スキーマ分割後は accountclient（players + player_factions）とローカル repo
// （player_story_progress）に分離する必要がある。
func (r *PgStoryRepository) GetUnlockContext(ctx context.Context, playerID string) (*apiscenario.StoryUnlockContext, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT
		   p.level,
		   COALESCE(ARRAY(SELECT faction FROM player_factions WHERE player_id = $1), '{}'),
		   COALESCE(ARRAY(SELECT episode_id FROM player_story_progress WHERE player_id = $1), '{}')
		 FROM players p
		 WHERE p.player_id = $1`,
		playerID)

	var level int64
	var factions []string
	var episodes []string
	if err := row.Scan(&level, &factions, &episodes); err != nil {
		return nil, fmt.Errorf("query unlock context: %w", err)
	}

	factionSet := make(map[string]bool, len(factions))
	for _, f := range factions {
		factionSet[f] = true
	}
	episodeSet := make(map[string]bool, len(episodes))
	for _, e := range episodes {
		episodeSet[e] = true
	}

	return &apiscenario.StoryUnlockContext{
		PlayerLevel:       level,
		OwnedFactions:     factionSet,
		CompletedEpisodes: episodeSet,
	}, nil
}

// MarkComplete はエピソードの完了を記録する。重複は ON CONFLICT でべき等に処理する。
func (r *PgStoryRepository) MarkComplete(ctx context.Context, playerID, episodeID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO player_story_progress (player_id, episode_id)
		 VALUES ($1, $2)
		 ON CONFLICT (player_id, episode_id) DO NOTHING`,
		playerID, episodeID,
	)
	if err != nil {
		return fmt.Errorf("mark episode complete: %w", err)
	}
	return nil
}
