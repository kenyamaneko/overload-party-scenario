package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
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

// fakeNameUpdater はテスト用の OnboardingNameUpdater 実装。account 直叩きを
// 模し、任意のエラーを注入して呼び出しを記録できる。
type fakeNameUpdater struct {
	err   error
	calls []nameUpdateCall
}

type nameUpdateCall struct {
	playerID string
	name     string
}

func (u *fakeNameUpdater) UpdateOnboardingName(_ context.Context, playerID, name string) error {
	u.calls = append(u.calls, nameUpdateCall{playerID: playerID, name: name})
	return u.err
}

// fakePlayerReader はテスト用の OnboardingPlayerReader 実装。
type fakePlayerReader struct {
	player port.AccountPlayer
	err    error
}

func (r *fakePlayerReader) GetOnboardingPlayer(_ context.Context, playerID string) (port.AccountPlayer, error) {
	if r.err != nil {
		return port.AccountPlayer{}, r.err
	}
	p := r.player
	if p.PlayerID == "" {
		p.PlayerID = playerID
	}
	return p, nil
}

// strPtr は *string ヘルパ。テストの可読性のために置く。
func strPtr(s string) *string { return &s }

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
			svc := New(repo, nil, nil, nil)

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
			svc := New(repo, store, nil, nil)

			body, err := svc.GetScript(context.Background(), "p1", tc.lang)
			tc.verify(t, body, err, store)
		})
	}
}

func TestComplete(t *testing.T) {
	validFaction := "SHE"

	repoErr := errors.New("db down")

	tests := []struct {
		name             string
		initialFactionID string
		repo             *fakeOnboardingRepo
		verify           func(t *testing.T, err error, repo *fakeOnboardingRepo)
	}{
		{
			name:             "正常系は player-onboarded イベント 1 本を outbox へ渡す",
			initialFactionID: validFaction,
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.NoError(t, err)
				require.Len(t, repo.markCompleteCalls, 1)
				call := repo.markCompleteCalls[0]
				assert.Equal(t, "p1", call.playerID)
				require.Len(t, call.events, 1)
				ev := call.events[0]
				assert.Equal(t, apiscenario.EventTypePlayerOnboarded, ev.EventType)
				assert.NotEqual(t, "", ev.EventID.String())

				// payload は apiscenario.PlayerOnboardedEvent として round-trip 可能で、
				// event_id は outbox 行の PK と一致する (subscriber 側の冪等性キー前提)。
				var decoded apiscenario.PlayerOnboardedEvent
				require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
				assert.Equal(t, apiscenario.EventTypePlayerOnboarded, decoded.EventType)
				assert.Equal(t, ev.EventID.String(), decoded.EventID)
				assert.Equal(t, "p1", decoded.PlayerID)
				assert.Equal(t, validFaction, decoded.InitialFactionID)
			},
		},
		{
			name:             "SelectableFactions 外の Neutral は ErrInvalidFaction",
			initialFactionID: "Neutral",
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "不明な faction は ErrInvalidFaction",
			initialFactionID: "Mystery",
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "空の faction は ErrInvalidFaction",
			initialFactionID: "",
			repo:             &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:             "二度目の完了は ErrAlreadyOnboarded に翻訳する",
			initialFactionID: validFaction,
			repo:             &fakeOnboardingRepo{markCompleteErr: port.ErrAlreadyOnboarded},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrAlreadyOnboarded)
			},
		},
		{
			name:             "repo の未分類エラーは wrap して伝播する",
			initialFactionID: validFaction,
			repo:             &fakeOnboardingRepo{markCompleteErr: repoErr},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, repoErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(tc.repo, nil, nil, nil)
			err := svc.Complete(context.Background(), "p1", tc.initialFactionID)
			tc.verify(t, err, tc.repo)
		})
	}
}

func TestUpdateName(t *testing.T) {
	transientErr := errors.New("account 5xx")

	tests := []struct {
		name    string
		updater *fakeNameUpdater
		input   string
		verify  func(t *testing.T, err error, updater *fakeNameUpdater)
	}{
		{
			name:    "正常系: account に表示名を中継する",
			updater: &fakeNameUpdater{},
			input:   "Kenya",
			verify: func(t *testing.T, err error, u *fakeNameUpdater) {
				require.NoError(t, err)
				require.Len(t, u.calls, 1)
				assert.Equal(t, "p1", u.calls[0].playerID)
				assert.Equal(t, "Kenya", u.calls[0].name)
			},
		},
		{
			name:    "account の ErrInvalidName は ErrInvalidName に翻訳して中継する",
			updater: &fakeNameUpdater{err: port.ErrInvalidName},
			input:   "",
			verify: func(t *testing.T, err error, _ *fakeNameUpdater) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidName)
			},
		},
		{
			name:    "account の ErrPlayerNotFound は ErrPlayerNotFound に翻訳する",
			updater: &fakeNameUpdater{err: port.ErrPlayerNotFound},
			input:   "Alice",
			verify: func(t *testing.T, err error, _ *fakeNameUpdater) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPlayerNotFound)
			},
		},
		{
			name:    "それ以外の account エラーは wrap して伝播する",
			updater: &fakeNameUpdater{err: transientErr},
			input:   "Bob",
			verify: func(t *testing.T, err error, _ *fakeNameUpdater) {
				require.Error(t, err)
				assert.ErrorIs(t, err, transientErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(nil, nil, tc.updater, nil)
			err := svc.UpdateName(context.Background(), "p1", tc.input)
			tc.verify(t, err, tc.updater)
		})
	}
}

func TestResume(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	repoErr := errors.New("db down")
	accountErr := errors.New("account down")

	tests := []struct {
		name   string
		repo   *fakeOnboardingRepo
		reader *fakePlayerReader
		verify func(t *testing.T, got Checkpoint, err error)
	}{
		{
			name:   "完了マークありなら CheckpointCompleted",
			repo:   &fakeOnboardingRepo{status: port.OnboardingStatus{Onboarded: true, CompletedAt: &now}},
			reader: &fakePlayerReader{},
			verify: func(t *testing.T, got Checkpoint, err error) {
				require.NoError(t, err)
				assert.Equal(t, CheckpointCompleted, got)
			},
		},
		{
			name:   "未完了 + Name=nil なら CheckpointStarted (account の状態が起点)",
			repo:   &fakeOnboardingRepo{},
			reader: &fakePlayerReader{player: port.AccountPlayer{Name: nil, SelectedFaction: nil}},
			verify: func(t *testing.T, got Checkpoint, err error) {
				require.NoError(t, err)
				assert.Equal(t, CheckpointStarted, got)
			},
		},
		{
			name:   "未完了 + Name 確定 + faction 未選択なら CheckpointNameSet",
			repo:   &fakeOnboardingRepo{},
			reader: &fakePlayerReader{player: port.AccountPlayer{Name: strPtr("Kenya"), SelectedFaction: nil}},
			verify: func(t *testing.T, got Checkpoint, err error) {
				require.NoError(t, err)
				assert.Equal(t, CheckpointNameSet, got)
			},
		},
		{
			name:   "未完了 + Name + faction 両方確定なら CheckpointFactionSet",
			repo:   &fakeOnboardingRepo{},
			reader: &fakePlayerReader{player: port.AccountPlayer{Name: strPtr("Kenya"), SelectedFaction: strPtr("Tenki")}},
			verify: func(t *testing.T, got Checkpoint, err error) {
				require.NoError(t, err)
				assert.Equal(t, CheckpointFactionSet, got)
			},
		},
		{
			name:   "account に Player が存在しなければ ErrPlayerNotFound",
			repo:   &fakeOnboardingRepo{},
			reader: &fakePlayerReader{err: port.ErrPlayerNotFound},
			verify: func(t *testing.T, _ Checkpoint, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPlayerNotFound)
			},
		},
		{
			name:   "scenario の repo エラーは wrap して伝播する",
			repo:   &fakeOnboardingRepo{getStatusErr: repoErr},
			reader: &fakePlayerReader{},
			verify: func(t *testing.T, _ Checkpoint, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, repoErr)
			},
		},
		{
			name:   "account の未分類エラーは wrap して伝播する",
			repo:   &fakeOnboardingRepo{},
			reader: &fakePlayerReader{err: accountErr},
			verify: func(t *testing.T, _ Checkpoint, err error) {
				require.Error(t, err)
				assert.ErrorIs(t, err, accountErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(tc.repo, nil, nil, tc.reader)
			got, err := svc.Resume(context.Background(), "p1")
			tc.verify(t, got, err)
		})
	}
}
