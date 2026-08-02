package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
)

// StoryRepo は scenario が所有するストーリーエピソードとプレイヤー進行の永続化を抽象化する。
type StoryRepo interface {
	ListActiveEpisodes(ctx context.Context) ([]*domain.Episode, error)
	FindEpisodeByID(ctx context.Context, episodeID string) (*domain.Episode, error)
	GetCompletedEpisodeIDs(ctx context.Context, playerID string) ([]string, error)
	MarkComplete(ctx context.Context, playerID, episodeID string) error
}

// GameConfigRepo はゲーム設定値の読み取りを抽象化する (キー不在時は ErrNotFound)。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
