package port

import (
	"context"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// StoryRepo はストーリーエピソードの永続化を抽象化する。
type StoryRepo interface {
	ListActiveEpisodes(ctx context.Context) ([]*apiscenario.ScenarioEpisode, error)
	FindEpisodeByID(ctx context.Context, episodeID string) (*apiscenario.ScenarioEpisode, error)
	GetCompletedEpisodeIDs(ctx context.Context, playerID string) ([]string, error)
	GetUnlockContext(ctx context.Context, playerID string) (*apiscenario.StoryUnlockContext, error)
	MarkComplete(ctx context.Context, playerID, episodeID string) error
}

// GameConfigRepo はゲーム設定値の読み取りを抽象化するインターフェースです。
// キーが存在しない場合は ErrNotFound を返す（fail-fast）。
type GameConfigRepo interface {
	GetInt64(ctx context.Context, key string) (int64, error)
}
