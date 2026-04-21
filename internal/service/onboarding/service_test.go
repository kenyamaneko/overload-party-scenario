package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// fakeOnboardingRepo はテスト用のインメモリ OnboardingRepo 実装。
// MarkComplete の呼び出しを記録し、任意のエラーを注入できる。
type fakeOnboardingRepo struct {
	status       port.OnboardingStatus
	getStatusErr error

	markCompleteErr   error
	markCompleteCalls []markCompleteCall
}

type markCompleteCall struct {
	playerID string
	events   []port.OutboxEvent
}

func (r *fakeOnboardingRepo) GetStatus(_ context.Context, playerID string) (port.OnboardingStatus, error) {
	if r.getStatusErr != nil {
		return port.OnboardingStatus{}, r.getStatusErr
	}
	st := r.status
	st.PlayerID = playerID
	return st, nil
}

func (r *fakeOnboardingRepo) MarkComplete(_ context.Context, playerID string, events ...port.OutboxEvent) error {
	r.markCompleteCalls = append(r.markCompleteCalls, markCompleteCall{
		playerID: playerID,
		events:   append([]port.OutboxEvent(nil), events...),
	})
	return r.markCompleteErr
}

// fakeScriptStore はテスト用の ScriptStore 実装。指定キーに対して任意の body またはエラーを返す。
type fakeScriptStore struct {
	body string
	err  error
	last string
}

func (s *fakeScriptStore) ReadScript(_ context.Context, key string) (string, error) {
	s.last = key
	if s.err != nil {
		return "", s.err
	}
	return s.body, nil
}

// fakeEventBuilder は OutboxEventBuilder のテスト double。
// 呼び出し引数を記録し、BuildPlayerOnboarded / BuildFactionSelected ごとに
// 任意のエラーを返せる。
type fakeEventBuilder struct {
	onboardedEvent port.OutboxEvent
	onboardedErr   error
	onboardedCalls []onboardedArgs
	factionEvent   port.OutboxEvent
	factionErr     error
	factionCalls   []factionArgs
}

type onboardedArgs struct {
	playerID, displayName, initialFactionID string
}

type factionArgs struct {
	playerID, faction string
}

func (b *fakeEventBuilder) BuildPlayerOnboarded(playerID, displayName, initialFactionID string) (port.OutboxEvent, error) {
	b.onboardedCalls = append(b.onboardedCalls, onboardedArgs{playerID, displayName, initialFactionID})
	if b.onboardedErr != nil {
		return port.OutboxEvent{}, b.onboardedErr
	}
	return b.onboardedEvent, nil
}

func (b *fakeEventBuilder) BuildFactionSelected(playerID, faction string) (port.OutboxEvent, error) {
	b.factionCalls = append(b.factionCalls, factionArgs{playerID, faction})
	if b.factionErr != nil {
		return port.OutboxEvent{}, b.factionErr
	}
	return b.factionEvent, nil
}

func newTestEvent(topic string) port.OutboxEvent {
	return port.OutboxEvent{
		EventID: uuid.New(),
		Topic:   topic,
		Payload: []byte(`{"topic":"` + topic + `"}`),
	}
}

func TestGetStatus(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	dbErr := errors.New("db down")

	tests := []struct {
		name       string
		repoStatus port.OnboardingStatus
		repoErr    error
		verify     func(t *testing.T, got port.OnboardingStatus, err error)
	}{
		{
			name:       "未完了プレイヤーは Onboarded=false を返す",
			repoStatus: port.OnboardingStatus{Onboarded: false},
			verify: func(t *testing.T, got port.OnboardingStatus, err error) {
				require.NoError(t, err)
				assert.False(t, got.Onboarded)
				assert.Nil(t, got.CompletedAt)
				assert.Equal(t, "p1", got.PlayerID)
			},
		},
		{
			name:       "完了済みプレイヤーは Onboarded=true と CompletedAt を返す",
			repoStatus: port.OnboardingStatus{Onboarded: true, CompletedAt: &now},
			verify: func(t *testing.T, got port.OnboardingStatus, err error) {
				require.NoError(t, err)
				assert.True(t, got.Onboarded)
				require.NotNil(t, got.CompletedAt)
				assert.Equal(t, now, *got.CompletedAt)
			},
		},
		{
			name:    "repo エラーは wrap して伝播する",
			repoErr: dbErr,
			verify: func(t *testing.T, _ port.OnboardingStatus, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, dbErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeOnboardingRepo{status: tc.repoStatus, getStatusErr: tc.repoErr}
			svc := New(repo, nil, nil)

			got, err := svc.GetStatus(context.Background(), "p1")
			tc.verify(t, got, err)
		})
	}
}

func TestGetScript(t *testing.T) {
	dbErr := errors.New("db down")

	tests := []struct {
		name       string
		repoStatus port.OnboardingStatus
		repoErr    error
		storeBody  string
		storeErr   error
		lang       string
		verify     func(t *testing.T, body string, err error, store *fakeScriptStore)
	}{
		{
			name:       "未完了プレイヤーには本文を返す",
			repoStatus: port.OnboardingStatus{Onboarded: false},
			storeBody:  "@endofscript\n",
			lang:       "ja",
			verify: func(t *testing.T, body string, err error, store *fakeScriptStore) {
				require.NoError(t, err)
				assert.Equal(t, "@endofscript\n", body)
				assert.Equal(t, "scripts/onboarding/ja.ks", store.last)
			},
		},
		{
			name:       "en 言語は対応キーを読みに行く",
			repoStatus: port.OnboardingStatus{Onboarded: false},
			storeBody:  "english body",
			lang:       "en",
			verify: func(t *testing.T, body string, err error, store *fakeScriptStore) {
				require.NoError(t, err)
				assert.Equal(t, "english body", body)
				assert.Equal(t, "scripts/onboarding/en.ks", store.last)
			},
		},
		{
			name:       "完了済みプレイヤーは ErrAlreadyOnboarded",
			repoStatus: port.OnboardingStatus{Onboarded: true},
			lang:       "ja",
			verify: func(t *testing.T, _ string, err error, _ *fakeScriptStore) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrAlreadyOnboarded)
			},
		},
		{
			name:       "スクリプト不在は ErrScriptNotFound",
			repoStatus: port.OnboardingStatus{Onboarded: false},
			storeErr:   port.ErrScriptNotFound,
			lang:       "en",
			verify: func(t *testing.T, _ string, err error, store *fakeScriptStore) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrScriptNotFound)
				assert.Equal(t, "scripts/onboarding/en.ks", store.last)
			},
		},
		{
			name:    "GetStatus エラーは wrap して伝播する",
			repoErr: dbErr,
			lang:    "ja",
			verify: func(t *testing.T, _ string, err error, _ *fakeScriptStore) {
				require.Error(t, err)
				assert.ErrorIs(t, err, dbErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeOnboardingRepo{status: tc.repoStatus, getStatusErr: tc.repoErr}
			store := &fakeScriptStore{body: tc.storeBody, err: tc.storeErr}
			svc := New(repo, store, nil)

			body, err := svc.GetScript(context.Background(), "p1", tc.lang)
			tc.verify(t, body, err, store)
		})
	}
}

func TestComplete(t *testing.T) {
	validName := "プレイヤー太郎"
	validFaction := "SHE"

	buildErr := errors.New("simulated build failure")
	repoErr := errors.New("db down")

	tests := []struct {
		name             string
		displayName      string
		initialFactionID string
		builder          *fakeEventBuilder
		repo             *fakeOnboardingRepo
		verify           func(t *testing.T, err error, repo *fakeOnboardingRepo, builder *fakeEventBuilder)
	}{
		{
			name:             "正常系は builder の 2 イベントを順に outbox へ渡す",
			displayName:      validName,
			initialFactionID: validFaction,
			builder: &fakeEventBuilder{
				onboardedEvent: newTestEvent("player-onboarded"),
				factionEvent:   newTestEvent("faction-selected"),
			},
			repo: &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, builder *fakeEventBuilder) {
				require.NoError(t, err)
				require.Len(t, repo.markCompleteCalls, 1)
				call := repo.markCompleteCalls[0]
				assert.Equal(t, "p1", call.playerID)
				require.Len(t, call.events, 2)
				assert.Equal(t, "player-onboarded", call.events[0].Topic)
				assert.Equal(t, "faction-selected", call.events[1].Topic)

				require.Len(t, builder.onboardedCalls, 1)
				assert.Equal(t, validName, builder.onboardedCalls[0].displayName)
				assert.Equal(t, validFaction, builder.onboardedCalls[0].initialFactionID)

				require.Len(t, builder.factionCalls, 1)
				assert.Equal(t, validFaction, builder.factionCalls[0].faction)
			},
		},
		{
			name:             "空文字の display_name は ErrInvalidDisplayName",
			displayName:      "",
			initialFactionID: validFaction,
			builder:          &fakeEventBuilder{},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidDisplayName)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "全空白の display_name は ErrInvalidDisplayName",
			displayName:      "   \t  ",
			initialFactionID: validFaction,
			builder:          &fakeEventBuilder{},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidDisplayName)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "22 rune の display_name は ErrInvalidDisplayName",
			displayName:      "あいうえおかきくけこさしすせそたちつてとなに",
			initialFactionID: validFaction,
			builder:          &fakeEventBuilder{},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidDisplayName)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "21 rune の display_name は境界内で許容する",
			displayName:      "あいうえおかきくけこさしすせそたちつてとな",
			initialFactionID: validFaction,
			builder: &fakeEventBuilder{
				onboardedEvent: newTestEvent("player-onboarded"),
				factionEvent:   newTestEvent("faction-selected"),
			},
			repo: &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.NoError(t, err)
				require.Len(t, repo.markCompleteCalls, 1)
			},
		},
		{
			name:             "SelectableFactions 外の Neutral は ErrInvalidFaction",
			displayName:      validName,
			initialFactionID: "Neutral",
			builder:          &fakeEventBuilder{},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "不明な faction は ErrInvalidFaction",
			displayName:      validName,
			initialFactionID: "Mystery",
			builder:          &fakeEventBuilder{},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "二度目の完了は ErrAlreadyOnboarded に翻訳する",
			displayName:      validName,
			initialFactionID: validFaction,
			builder: &fakeEventBuilder{
				onboardedEvent: newTestEvent("player-onboarded"),
				factionEvent:   newTestEvent("faction-selected"),
			},
			repo: &fakeOnboardingRepo{markCompleteErr: port.ErrAlreadyOnboarded},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrAlreadyOnboarded)
			},
		},
		{
			name:             "BuildPlayerOnboarded のエラーは wrap して伝播する",
			displayName:      validName,
			initialFactionID: validFaction,
			builder:          &fakeEventBuilder{onboardedErr: buildErr},
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, buildErr)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "BuildFactionSelected のエラーは wrap して伝播する",
			displayName:      validName,
			initialFactionID: validFaction,
			builder: &fakeEventBuilder{
				onboardedEvent: newTestEvent("player-onboarded"),
				factionErr:     buildErr,
			},
			repo: &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, buildErr)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "repo の未分類エラーは wrap して伝播する",
			displayName:      validName,
			initialFactionID: validFaction,
			builder: &fakeEventBuilder{
				onboardedEvent: newTestEvent("player-onboarded"),
				factionEvent:   newTestEvent("faction-selected"),
			},
			repo: &fakeOnboardingRepo{markCompleteErr: repoErr},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo, _ *fakeEventBuilder) {
				require.Error(t, err)
				assert.ErrorIs(t, err, repoErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(tc.repo, nil, tc.builder)
			err := svc.Complete(context.Background(), "p1", tc.displayName, tc.initialFactionID)
			tc.verify(t, err, tc.repo, tc.builder)
		})
	}
}
