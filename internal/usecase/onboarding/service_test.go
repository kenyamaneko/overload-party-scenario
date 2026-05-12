package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// fakeOnboardingRepo は OnboardingRepo の最小スタブ。MarkComplete / PublishEvents の
// 呼び出しを記録し、任意のエラーを注入できる。
type fakeOnboardingRepo struct {
	markCompleteErr error
	markCompleteCalls []markCompleteCall

	publishErr error
	publishCalls []publishCall
}

type markCompleteCall struct {
	playerID string
	events   []port.OutboxEvent
}

type publishCall struct {
	events []port.OutboxEvent
}

func (r *fakeOnboardingRepo) MarkComplete(_ context.Context, playerID string, events ...port.OutboxEvent) error {
	r.markCompleteCalls = append(r.markCompleteCalls, markCompleteCall{
		playerID: playerID,
		events:   append([]port.OutboxEvent(nil), events...),
	})
	return r.markCompleteErr
}

func (r *fakeOnboardingRepo) PublishEvents(_ context.Context, events ...port.OutboxEvent) error {
	r.publishCalls = append(r.publishCalls, publishCall{
		events: append([]port.OutboxEvent(nil), events...),
	})
	return r.publishErr
}

// fakeScriptStore はテスト用の ScriptStore 実装。
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

// fakeNameValidator は OnboardingNameValidator のスタブ。
type fakeNameValidator struct {
	err   error
	calls []nameValidateCall
}

type nameValidateCall struct {
	name string
}

func (v *fakeNameValidator) ValidateOnboardingName(_ context.Context, name string) error {
	v.calls = append(v.calls, nameValidateCall{name: name})
	return v.err
}

// fakePlayerReader は OnboardingPlayerReader のスタブ。
type fakePlayerReader struct {
	player port.AccountPlayer
	err    error
}

func (r *fakePlayerReader) GetOnboardingPlayer(_ context.Context) (port.AccountPlayer, error) {
	if r.err != nil {
		return port.AccountPlayer{}, r.err
	}
	return r.player, nil
}

func strPtr(s string) *string { return &s }

func TestGetScript(t *testing.T) {
	tests := []struct {
		name      string
		storeBody string
		storeErr  error
		lang      string
		verify    func(t *testing.T, body string, err error, store *fakeScriptStore)
	}{
		{
			name:      "正常系: 指定言語のキーから body を取得する",
			storeBody: "@endofscript\n",
			lang:      "ja",
			verify: func(t *testing.T, body string, err error, store *fakeScriptStore) {
				require.NoError(t, err)
				assert.Equal(t, "@endofscript\n", body)
				assert.Equal(t, "scripts/onboarding/ja.ks", store.last)
			},
		},
		{
			name:      "en 言語は対応キーを読みに行く",
			storeBody: "english body",
			lang:      "en",
			verify: func(t *testing.T, body string, err error, store *fakeScriptStore) {
				require.NoError(t, err)
				assert.Equal(t, "english body", body)
				assert.Equal(t, "scripts/onboarding/en.ks", store.last)
			},
		},
		{
			name:     "スクリプト不在は ErrScriptNotFound に翻訳する",
			storeErr: port.ErrScriptNotFound,
			lang:     "en",
			verify: func(t *testing.T, _ string, err error, store *fakeScriptStore) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrScriptNotFound)
				assert.Equal(t, "scripts/onboarding/en.ks", store.last)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeScriptStore{body: tc.storeBody, err: tc.storeErr}
			svc := New(&fakeOnboardingRepo{}, store, nil, nil)

			body, err := svc.GetScript(context.Background(), "p1", tc.lang)
			tc.verify(t, body, err, store)
		})
	}
}

func TestUpdateName(t *testing.T) {
	transientErr := errors.New("account 5xx")
	publishErr := errors.New("outbox down")

	tests := []struct {
		name       string
		validator  *fakeNameValidator
		publishErr error
		input      string
		verify     func(t *testing.T, err error, validator *fakeNameValidator, repo *fakeOnboardingRepo)
	}{
		{
			name:      "正常系: validate 成功後に onboarding-name-set を outbox publish",
			validator: &fakeNameValidator{},
			input:     "Kenya",
			verify: func(t *testing.T, err error, v *fakeNameValidator, repo *fakeOnboardingRepo) {
				require.NoError(t, err)
				require.Len(t, v.calls, 1)
				assert.Equal(t, "Kenya", v.calls[0].name)
				require.Len(t, repo.publishCalls, 1)
				require.Len(t, repo.publishCalls[0].events, 1)
				ev := repo.publishCalls[0].events[0]
				assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, ev.EventType)
				var decoded apiscenario.OnboardingNameSetEvent
				require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
				assert.Equal(t, "p1", decoded.PlayerID)
				assert.Equal(t, "Kenya", decoded.Name)
				assert.Equal(t, ev.EventID.String(), decoded.EventID)
			},
		},
		{
			name:      "account の ErrInvalidName は ErrInvalidName に翻訳して publish しない",
			validator: &fakeNameValidator{err: port.ErrInvalidName},
			input:     "",
			verify: func(t *testing.T, err error, _ *fakeNameValidator, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidName)
				assert.Empty(t, repo.publishCalls, "validate 失敗時に publish しない")
			},
		},
		{
			name:      "account の ErrPlayerNotFound は ErrPlayerNotFound に翻訳して publish しない",
			validator: &fakeNameValidator{err: port.ErrPlayerNotFound},
			input:     "Alice",
			verify: func(t *testing.T, err error, _ *fakeNameValidator, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPlayerNotFound)
				assert.Empty(t, repo.publishCalls)
			},
		},
		{
			name:      "それ以外の account エラーは wrap して publish しない",
			validator: &fakeNameValidator{err: transientErr},
			input:     "Bob",
			verify: func(t *testing.T, err error, _ *fakeNameValidator, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, transientErr)
				assert.Empty(t, repo.publishCalls)
			},
		},
		{
			name:       "outbox publish 失敗は wrap して伝播する",
			validator:  &fakeNameValidator{},
			publishErr: publishErr,
			input:      "Kenya",
			verify: func(t *testing.T, err error, _ *fakeNameValidator, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, publishErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeOnboardingRepo{publishErr: tc.publishErr}
			svc := New(repo, nil, tc.validator, nil)
			err := svc.UpdateName(context.Background(), "p1", tc.input)
			tc.verify(t, err, tc.validator, repo)
		})
	}
}

func TestSelectFaction(t *testing.T) {
	publishErr := errors.New("outbox down")

	tests := []struct {
		name             string
		initialFactionID string
		publishErr       error
		verify           func(t *testing.T, err error, repo *fakeOnboardingRepo)
	}{
		{
			name:             "正常系: SelectableFactions 内で onboarding-faction-set を outbox publish",
			initialFactionID: "SHE",
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.NoError(t, err)
				require.Len(t, repo.publishCalls, 1)
				require.Len(t, repo.publishCalls[0].events, 1)
				ev := repo.publishCalls[0].events[0]
				assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, ev.EventType)
				var decoded apiscenario.OnboardingFactionSetEvent
				require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
				assert.Equal(t, "p1", decoded.PlayerID)
				assert.Equal(t, "SHE", decoded.InitialFactionID)
			},
		},
		{
			name:             "SelectableFactions 外の Neutral は ErrInvalidFaction で publish しない",
			initialFactionID: "Neutral",
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.publishCalls)
			},
		},
		{
			name:             "不明な faction は ErrInvalidFaction で publish しない",
			initialFactionID: "Mystery",
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.publishCalls)
			},
		},
		{
			name:             "空の faction は ErrInvalidFaction で publish しない",
			initialFactionID: "",
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidFaction)
				assert.Empty(t, repo.publishCalls)
			},
		},
		{
			name:             "outbox publish 失敗は wrap して伝播する",
			initialFactionID: "SHE",
			publishErr:       publishErr,
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, publishErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeOnboardingRepo{publishErr: tc.publishErr}
			svc := New(repo, nil, nil, nil)
			err := svc.SelectFaction(context.Background(), "p1", tc.initialFactionID)
			tc.verify(t, err, repo)
		})
	}
}

func TestComplete(t *testing.T) {
	repoErr := errors.New("db down")
	accountErr := errors.New("account down")

	tests := []struct {
		name   string
		reader *fakePlayerReader
		repo   *fakeOnboardingRepo
		verify func(t *testing.T, err error, repo *fakeOnboardingRepo)
	}{
		{
			name:   "正常系は player-onboarded イベント 1 本を outbox へ渡す",
			reader: &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}},
			repo:   &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.NoError(t, err)
				require.Len(t, repo.markCompleteCalls, 1)
				call := repo.markCompleteCalls[0]
				assert.Equal(t, "p1", call.playerID)
				require.Len(t, call.events, 1)
				ev := call.events[0]
				assert.Equal(t, apiscenario.EventTypePlayerOnboarded, ev.EventType)
				var decoded apiscenario.PlayerOnboardedEvent
				require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
				assert.Equal(t, "p1", decoded.PlayerID)
				assert.Equal(t, "SHE", decoded.InitialFactionID)
				assert.Equal(t, ev.EventID.String(), decoded.EventID)
			},
		},
		{
			name:   "initial_faction が nil なら ErrFactionNotSelected (フロー違反)",
			reader: &fakePlayerReader{player: port.AccountPlayer{InitialFaction: nil}},
			repo:   &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrFactionNotSelected)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:   "initial_faction が空文字でも ErrFactionNotSelected",
			reader: &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("")}},
			repo:   &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, repo *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrFactionNotSelected)
				assert.Empty(t, repo.markCompleteCalls)
			},
		},
		{
			name:   "account に Player が存在しなければ ErrPlayerNotFound",
			reader: &fakePlayerReader{err: port.ErrPlayerNotFound},
			repo:   &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrPlayerNotFound)
			},
		},
		{
			name:   "二度目の完了は ErrAlreadyOnboarded に翻訳する",
			reader: &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}},
			repo:   &fakeOnboardingRepo{markCompleteErr: port.ErrAlreadyOnboarded},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrAlreadyOnboarded)
			},
		},
		{
			name:   "repo の未分類エラーは wrap して伝播する",
			reader: &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}},
			repo:   &fakeOnboardingRepo{markCompleteErr: repoErr},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, repoErr)
			},
		},
		{
			name:   "account の未分類エラーは wrap して伝播する",
			reader: &fakePlayerReader{err: accountErr},
			repo:   &fakeOnboardingRepo{},
			verify: func(t *testing.T, err error, _ *fakeOnboardingRepo) {
				require.Error(t, err)
				assert.ErrorIs(t, err, accountErr)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(tc.repo, nil, nil, tc.reader)
			err := svc.Complete(context.Background(), "p1")
			tc.verify(t, err, tc.repo)
		})
	}
}
