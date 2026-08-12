package story

import (
	"context"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

type storyMarkCompleteCall struct {
	playerID  string
	episodeID string
}

type fakeStoryRepo struct {
	activeEpisodes    []*domain.Episode
	activeErr         error
	findEpisode       *domain.Episode
	findErr           error
	completedErr      error
	markCompleteErr   error
	markCompleteCalls []storyMarkCompleteCall
}

func (f *fakeStoryRepo) ListActiveEpisodes(ctx context.Context) ([]*domain.Episode, error) {
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	return f.activeEpisodes, nil
}

func (f *fakeStoryRepo) FindEpisodeByID(ctx context.Context, episodeID string) (*domain.Episode, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findEpisode, nil
}

func (f *fakeStoryRepo) GetCompletedEpisodeIDs(ctx context.Context, playerID string) ([]string, error) {
	if f.completedErr != nil {
		return nil, f.completedErr
	}
	return nil, nil
}

func (f *fakeStoryRepo) MarkComplete(ctx context.Context, playerID, episodeID string) error {
	if f.markCompleteErr != nil {
		return f.markCompleteErr
	}
	f.markCompleteCalls = append(f.markCompleteCalls, storyMarkCompleteCall{playerID: playerID, episodeID: episodeID})
	return nil
}

type fakeScriptStore struct {
	body       string
	err        error
	calledKeys []string
}

func (f *fakeScriptStore) ReadScript(ctx context.Context, key string) (string, error) {
	f.calledKeys = append(f.calledKeys, key)
	if f.err != nil {
		return "", f.err
	}
	return f.body, nil
}

type fakePlayerProgressReader struct {
	progress port.PlayerProgress
	err      error
}

func (f *fakePlayerProgressReader) GetPlayerProgress(ctx context.Context) (port.PlayerProgress, error) {
	if f.err != nil {
		return port.PlayerProgress{}, f.err
	}
	return f.progress, nil
}
