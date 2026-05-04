package story

import (
	"context"
	"fmt"
	"sync"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

var _ port.StoryRepo = (*mockStoryRepository)(nil)

// mockStoryRepository はテスト用の自己完結型インメモリ StoryRepo 実装。
type mockStoryRepository struct {
	mu            sync.Mutex
	episodes      []*domain.Episode
	completed     map[string]map[string]bool
	playerLevels  map[string]int64
	ownedFactions map[string]map[string]bool
}

// newMockStoryRepository はテスト用の mockStoryRepository を構築する。
func newMockStoryRepository() *mockStoryRepository {
	return &mockStoryRepository{
		completed:     make(map[string]map[string]bool),
		playerLevels:  make(map[string]int64),
		ownedFactions: make(map[string]map[string]bool),
	}
}

// SeedEpisodes はテスト用にエピソードデータをプリセットする。
func (r *mockStoryRepository) SeedEpisodes(episodes []*domain.Episode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.episodes = episodes
}

// SetPlayerLevel はテスト用にプレイヤーレベルを設定する。
func (r *mockStoryRepository) SetPlayerLevel(playerID string, level int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerLevels[playerID] = level
}

// GrantFaction はテスト用にプレイヤーへ faction 所有を付与する。
func (r *mockStoryRepository) GrantFaction(playerID, faction string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ownedFactions[playerID] == nil {
		r.ownedFactions[playerID] = make(map[string]bool)
	}
	r.ownedFactions[playerID][faction] = true
}

// ListActiveEpisodes はアクティブなエピソード一覧をインメモリから返す。
func (r *mockStoryRepository) ListActiveEpisodes(_ context.Context) ([]*domain.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*domain.Episode
	for _, ep := range r.episodes {
		if ep.IsActive {
			result = append(result, ep)
		}
	}
	return result, nil
}

// FindEpisodeByID は指定 ID のエピソードをインメモリから返す。
func (r *mockStoryRepository) FindEpisodeByID(_ context.Context, episodeID string) (*domain.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ep := range r.episodes {
		if ep.EpisodeID == episodeID {
			return ep, nil
		}
	}
	return nil, fmt.Errorf("episode %s: %w", episodeID, port.ErrNotFound)
}

// GetCompletedEpisodeIDs はプレイヤーの完了済みエピソード ID 一覧をインメモリから返す。
func (r *mockStoryRepository) GetCompletedEpisodeIDs(_ context.Context, playerID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []string
	for id := range r.completed[playerID] {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetUnlockContext はプレイヤーのアンロック判定コンテキストをインメモリから返す。
func (r *mockStoryRepository) GetUnlockContext(_ context.Context, playerID string) (*domain.UnlockContext, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	factionSet := make(map[string]bool)
	for f := range r.ownedFactions[playerID] {
		factionSet[f] = true
	}

	completedSet := make(map[string]bool, len(r.completed[playerID]))
	for id := range r.completed[playerID] {
		completedSet[id] = true
	}

	return &domain.UnlockContext{
		PlayerLevel:       r.playerLevels[playerID],
		OwnedFactions:     factionSet,
		CompletedEpisodes: completedSet,
	}, nil
}

// MarkComplete はエピソード完了をインメモリに記録する。
func (r *mockStoryRepository) MarkComplete(_ context.Context, playerID, episodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completed[playerID] == nil {
		r.completed[playerID] = make(map[string]bool)
	}
	r.completed[playerID][episodeID] = true
	return nil
}
