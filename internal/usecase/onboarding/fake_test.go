package onboarding

import (
	"context"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

type onboardingMarkCompleteCall struct {
	playerID string
	events   []port.OutboxEvent
}

type fakeOnboardingRepo struct {
	publishErr        error
	markCompleteErr   error
	publishedEvents   []port.OutboxEvent
	markCompleteCalls []onboardingMarkCompleteCall
}

func (f *fakeOnboardingRepo) PublishEvents(ctx context.Context, events ...port.OutboxEvent) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.publishedEvents = append(f.publishedEvents, events...)
	return nil
}

func (f *fakeOnboardingRepo) MarkComplete(ctx context.Context, playerID string, events ...port.OutboxEvent) error {
	if f.markCompleteErr != nil {
		return f.markCompleteErr
	}
	f.markCompleteCalls = append(f.markCompleteCalls, onboardingMarkCompleteCall{playerID: playerID, events: events})
	return nil
}

type fakeScriptStore struct {
	body string
	err  error
}

func (f *fakeScriptStore) ReadScript(ctx context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.body, nil
}

type fakeNameValidator struct {
	err error
}

func (f *fakeNameValidator) ValidateOnboardingName(ctx context.Context, name string) error {
	return f.err
}

type fakePlayerReader struct {
	player port.AccountPlayer
	err    error
}

func (f *fakePlayerReader) GetOnboardingPlayer(ctx context.Context) (port.AccountPlayer, error) {
	if f.err != nil {
		return port.AccountPlayer{}, f.err
	}
	return f.player, nil
}

func ptr[T any](v T) *T {
	return &v
}
