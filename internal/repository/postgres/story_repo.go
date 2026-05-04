package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

var _ port.StoryRepo = (*StoryRepository)(nil)

// StoryRepository は PostgreSQL で StoryRepo を実装する。
type StoryRepository struct {
	pool *pgxpool.Pool
}

// NewStoryRepository は pgxpool.Pool を受け取り StoryRepository を構築する。
func NewStoryRepository(pool *pgxpool.Pool) *StoryRepository {
	return &StoryRepository{pool: pool}
}

// ListActiveEpisodes はアクティブなエピソード一覧を返す。
func (r *StoryRepository) ListActiveEpisodes(ctx context.Context) ([]*domain.Episode, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT e.episode_id, e.faction, e.episode_number, e.title_ja, e.title_en,
		        e.required_level,
		        COALESCE(ARRAY(
		          SELECT faction_id FROM episode_required_factions
		          WHERE episode_id = e.episode_id ORDER BY faction_id
		        ), '{}') AS required_factions,
		        e.required_episodes,
		        e.script_path, e.thumbnail_path, e.sort_order, e.is_active, e.created_at
		 FROM scenario_episodes e
		 WHERE e.is_active = true
		 ORDER BY e.sort_order`)
	if err != nil {
		return nil, fmt.Errorf("query episodes: %w", err)
	}
	defer rows.Close()

	var episodes []*domain.Episode
	for rows.Next() {
		var ep domain.Episode
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
func (r *StoryRepository) FindEpisodeByID(ctx context.Context, episodeID string) (*domain.Episode, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT e.episode_id, e.faction, e.episode_number, e.title_ja, e.title_en,
		        e.required_level,
		        COALESCE(ARRAY(
		          SELECT faction_id FROM episode_required_factions
		          WHERE episode_id = e.episode_id ORDER BY faction_id
		        ), '{}') AS required_factions,
		        e.required_episodes,
		        e.script_path, e.thumbnail_path, e.sort_order, e.is_active, e.created_at
		 FROM scenario_episodes e
		 WHERE e.episode_id = $1`,
		episodeID)

	var ep domain.Episode
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
func (r *StoryRepository) GetCompletedEpisodeIDs(ctx context.Context, playerID string) ([]string, error) {
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
func (r *StoryRepository) GetUnlockContext(ctx context.Context, playerID string) (*domain.UnlockContext, error) {
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

	return &domain.UnlockContext{
		PlayerLevel:       level,
		OwnedFactions:     factionSet,
		CompletedEpisodes: episodeSet,
	}, nil
}

// MarkComplete はエピソードの完了を記録する (重複は ON CONFLICT でべき等)。
func (r *StoryRepository) MarkComplete(ctx context.Context, playerID, episodeID string) error {
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
