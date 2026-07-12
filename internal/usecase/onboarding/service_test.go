package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

// fakeOnboardingRepo は OnboardingRepo の最小スタブ。MarkComplete / PublishEvents の
// 呼び出しを記録し、任意のエラーを注入できる。
type fakeOnboardingRepo struct {
	markCompleteErr   error
	markCompleteCalls []markCompleteCall

	publishErr   error
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

// writeScript は実 local.ScriptStore が読むディレクトリ構造 (scripts/onboarding/<lang>.ks) に
// 本文を書き込む。lang ごとに異なる本文を置くことで、lang ルーティングの効果を実ファイル経由で観測できる。
func writeScript(t *testing.T, root, lang, body string) {
	t.Helper()
	path := filepath.Join(root, "scripts", "onboarding", lang+".ks")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestGetScript(t *testing.T) {
	t.Run("オンボーディングスクリプトの取得", func(t *testing.T) {
		t.Run("ja 指定のとき、ja の本文が返る", func(t *testing.T) {
			root := t.TempDir()
			writeScript(t, root, "ja", "@endofscript (ja)\n")
			writeScript(t, root, "en", "english body")
			svc := New(&fakeOnboardingRepo{}, local.NewScriptStore(root), nil, nil)

			body, err := svc.GetScript(context.Background(), "p1", "ja")
			require.NoError(t, err)
			assert.Equal(t, "@endofscript (ja)\n", body)
		})

		t.Run("en 指定のとき、en の本文が返る", func(t *testing.T) {
			root := t.TempDir()
			writeScript(t, root, "ja", "@endofscript (ja)\n")
			writeScript(t, root, "en", "english body")
			svc := New(&fakeOnboardingRepo{}, local.NewScriptStore(root), nil, nil)

			body, err := svc.GetScript(context.Background(), "p1", "en")
			require.NoError(t, err)
			assert.Equal(t, "english body", body)
		})

		t.Run("スクリプトが不在のとき、ErrScriptNotFound に翻訳する", func(t *testing.T) {
			root := t.TempDir()
			svc := New(&fakeOnboardingRepo{}, local.NewScriptStore(root), nil, nil)

			body, err := svc.GetScript(context.Background(), "p1", "en")
			require.ErrorIs(t, err, ErrScriptNotFound)
			assert.Empty(t, body)
		})
	})
}

func TestUpdateName(t *testing.T) {
	t.Run("表示名の更新", func(t *testing.T) {
		t.Run("正常系: validate 成功後に onboarding-name-set を outbox publish する", func(t *testing.T) {
			repo := &fakeOnboardingRepo{}
			validator := &fakeNameValidator{}
			svc := New(repo, nil, validator, nil)

			err := svc.UpdateName(context.Background(), "p1", "Kenya")
			require.NoError(t, err)
			require.Len(t, validator.calls, 1)
			assert.Equal(t, "Kenya", validator.calls[0].name)
			require.Len(t, repo.publishCalls, 1)
			require.Len(t, repo.publishCalls[0].events, 1)
			ev := repo.publishCalls[0].events[0]
			assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, ev.EventType)
			var decoded apiscenario.OnboardingNameSetEvent
			require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
			assert.Equal(t, "p1", decoded.PlayerID)
			assert.Equal(t, "Kenya", decoded.Name)
			assert.Equal(t, ev.EventID.String(), decoded.EventID)
		})

		t.Run("validate 失敗のときは publish せず、対応する sentinel に翻訳する", func(t *testing.T) {
			transientErr := errors.New("account 5xx")
			tests := []struct {
				name      string
				injectErr error
				input     string
				wantErr   error
			}{
				{
					name:      "account の ErrInvalidName のとき、ErrInvalidName に翻訳する",
					injectErr: port.ErrInvalidName,
					input:     "",
					wantErr:   ErrInvalidName,
				},
				{
					name:      "account の ErrPlayerNotFound のとき、ErrPlayerNotFound に翻訳する",
					injectErr: port.ErrPlayerNotFound,
					input:     "Alice",
					wantErr:   ErrPlayerNotFound,
				},
				{
					name:      "それ以外の account エラーのとき、wrap して伝播する",
					injectErr: transientErr,
					input:     "Bob",
					wantErr:   transientErr,
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					repo := &fakeOnboardingRepo{}
					validator := &fakeNameValidator{err: tt.injectErr}
					svc := New(repo, nil, validator, nil)

					err := svc.UpdateName(context.Background(), "p1", tt.input)
					require.Error(t, err)
					assert.ErrorIs(t, err, tt.wantErr)
					assert.Empty(t, repo.publishCalls, "validate 失敗時に publish しない")
				})
			}
		})

		t.Run("outbox publish 失敗のとき、wrap して伝播する", func(t *testing.T) {
			publishErr := errors.New("outbox down")
			repo := &fakeOnboardingRepo{publishErr: publishErr}
			svc := New(repo, nil, &fakeNameValidator{}, nil)

			err := svc.UpdateName(context.Background(), "p1", "Kenya")
			require.Error(t, err)
			assert.ErrorIs(t, err, publishErr)
		})
	})
}

func TestSelectFaction(t *testing.T) {
	t.Run("初期 faction の選択", func(t *testing.T) {
		t.Run("正常系: SelectableFactions 内なら onboarding-faction-set を outbox publish する", func(t *testing.T) {
			repo := &fakeOnboardingRepo{}
			svc := New(repo, nil, nil, nil)

			err := svc.SelectFaction(context.Background(), "p1", "SHE")
			require.NoError(t, err)
			require.Len(t, repo.publishCalls, 1)
			require.Len(t, repo.publishCalls[0].events, 1)
			ev := repo.publishCalls[0].events[0]
			assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, ev.EventType)
			var decoded apiscenario.OnboardingFactionSetEvent
			require.NoError(t, json.Unmarshal(ev.Payload, &decoded))
			assert.Equal(t, "p1", decoded.PlayerID)
			assert.Equal(t, "SHE", decoded.InitialFactionID)
		})

		t.Run("SelectableFactions 外のときは publish せず、ErrInvalidFaction になる", func(t *testing.T) {
			tests := []struct {
				name             string
				initialFactionID string
			}{
				{name: "Neutral のとき、ErrInvalidFaction になる", initialFactionID: "Neutral"},
				{name: "不明な faction のとき、ErrInvalidFaction になる", initialFactionID: "Mystery"},
				{name: "空文字の faction のとき、ErrInvalidFaction になる", initialFactionID: ""},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					repo := &fakeOnboardingRepo{}
					svc := New(repo, nil, nil, nil)

					err := svc.SelectFaction(context.Background(), "p1", tt.initialFactionID)
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrInvalidFaction)
					assert.Empty(t, repo.publishCalls)
				})
			}
		})

		t.Run("outbox publish 失敗のとき、wrap して伝播する", func(t *testing.T) {
			publishErr := errors.New("outbox down")
			repo := &fakeOnboardingRepo{publishErr: publishErr}
			svc := New(repo, nil, nil, nil)

			err := svc.SelectFaction(context.Background(), "p1", "SHE")
			require.Error(t, err)
			assert.ErrorIs(t, err, publishErr)
		})
	})
}

func TestComplete(t *testing.T) {
	t.Run("オンボーディングの完了", func(t *testing.T) {
		t.Run("正常系: player-onboarded イベント 1 本を outbox へ渡す", func(t *testing.T) {
			reader := &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}}
			repo := &fakeOnboardingRepo{}
			svc := New(repo, nil, nil, reader)

			err := svc.Complete(context.Background(), "p1")
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
		})

		t.Run("initial_faction が未確定のときは記録せず、ErrFactionNotSelected になる", func(t *testing.T) {
			// フロー違反 (faction 未選択のまま完了) を弾く。
			tests := []struct {
				name           string
				initialFaction *string
			}{
				{name: "initial_faction が nil のとき、ErrFactionNotSelected になる", initialFaction: nil},
				{name: "initial_faction が空文字のとき、ErrFactionNotSelected になる", initialFaction: strPtr("")},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					reader := &fakePlayerReader{player: port.AccountPlayer{InitialFaction: tt.initialFaction}}
					repo := &fakeOnboardingRepo{}
					svc := New(repo, nil, nil, reader)

					err := svc.Complete(context.Background(), "p1")
					require.Error(t, err)
					assert.ErrorIs(t, err, ErrFactionNotSelected)
					assert.Empty(t, repo.markCompleteCalls)
				})
			}
		})

		t.Run("依存のエラーは対応する sentinel に翻訳・伝播する", func(t *testing.T) {
			repoErr := errors.New("db down")
			accountErr := errors.New("account down")
			tests := []struct {
				name    string
				reader  *fakePlayerReader
				repo    *fakeOnboardingRepo
				wantErr error
			}{
				{
					name:    "account に Player が存在しないとき、ErrPlayerNotFound になる",
					reader:  &fakePlayerReader{err: port.ErrPlayerNotFound},
					repo:    &fakeOnboardingRepo{},
					wantErr: ErrPlayerNotFound,
				},
				{
					name:    "二度目の完了のとき、ErrAlreadyOnboarded に翻訳する",
					reader:  &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}},
					repo:    &fakeOnboardingRepo{markCompleteErr: port.ErrAlreadyOnboarded},
					wantErr: ErrAlreadyOnboarded,
				},
				{
					name:    "repo の未分類エラーのとき、wrap して伝播する",
					reader:  &fakePlayerReader{player: port.AccountPlayer{InitialFaction: strPtr("SHE")}},
					repo:    &fakeOnboardingRepo{markCompleteErr: repoErr},
					wantErr: repoErr,
				},
				{
					name:    "account の未分類エラーのとき、wrap して伝播する",
					reader:  &fakePlayerReader{err: accountErr},
					repo:    &fakeOnboardingRepo{},
					wantErr: accountErr,
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					svc := New(tt.repo, nil, nil, tt.reader)
					err := svc.Complete(context.Background(), "p1")
					require.Error(t, err)
					assert.ErrorIs(t, err, tt.wantErr)
				})
			}
		})
	})
}
