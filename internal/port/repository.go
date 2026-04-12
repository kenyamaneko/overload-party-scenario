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
