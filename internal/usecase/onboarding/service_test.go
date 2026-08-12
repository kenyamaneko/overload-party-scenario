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

func TestGetScript(t *testing.T) {
	t.Run("[オンボーディング]オンボーディングスクリプト取得", func(t *testing.T) {
		t.Run("要求言語のスクリプトが存在するとき、その本文が返る", func(t *testing.T) {
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{body: "dummy onboarding script body"}, &fakeNameValidator{}, &fakePlayerReader{})

			body, err := svc.GetScript(context.Background(), "TST-0001", "ja")

			require.NoError(t, err)
			assert.Equal(t, "dummy onboarding script body", body)
		})

		t.Run("要求言語のスクリプトが存在しないとき、スクリプト未検出を表すエラーになる", func(t *testing.T) {
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{err: port.ErrScriptNotFound}, &fakeNameValidator{}, &fakePlayerReader{})

			_, err := svc.GetScript(context.Background(), "TST-0001", "ja")

			require.ErrorIs(t, err, ErrScriptNotFound)
		})

		t.Run("スクリプトストアが未検出以外のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("script store boom")
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{err: errBoom}, &fakeNameValidator{}, &fakePlayerReader{})

			_, err := svc.GetScript(context.Background(), "TST-0001", "ja")

			require.ErrorIs(t, err, errBoom)
		})
	})
}

func TestUpdateName(t *testing.T) {
	t.Run("[オンボーディング]name入力ステップの更新", func(t *testing.T) {
		t.Run("表示名が有効なとき、name入力ステップ完了イベントがpublishされ、エラーなく完了する", func(t *testing.T) {
			repo := &fakeOnboardingRepo{}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, &fakePlayerReader{})

			err := svc.UpdateName(context.Background(), "TST-0001", "たろう")

			require.NoError(t, err)
			require.Len(t, repo.publishedEvents, 1)
			published := repo.publishedEvents[0]
			assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, published.EventType)

			var wire apiscenario.OnboardingNameSetEvent
			require.NoError(t, json.Unmarshal(published.Payload, &wire))
			assert.Equal(t, apiscenario.EventTypeOnboardingNameSet, wire.EventType)
			assert.Equal(t, "TST-0001", wire.PlayerID)
			assert.Equal(t, "たろう", wire.Name)
		})

		t.Run("表示名がaccount側のバリデーションで無効と判定されるとき、無効な名前を表すエラーになる", func(t *testing.T) {
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{err: port.ErrInvalidName}, &fakePlayerReader{})

			err := svc.UpdateName(context.Background(), "TST-0001", "たろう")

			require.ErrorIs(t, err, ErrInvalidName)
		})

		t.Run("対象プレイヤーがaccountに存在しないとき、プレイヤー未検出を表すエラーになる", func(t *testing.T) {
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{err: port.ErrPlayerNotFound}, &fakePlayerReader{})

			err := svc.UpdateName(context.Background(), "TST-0001", "たろう")

			require.ErrorIs(t, err, ErrPlayerNotFound)
		})

		t.Run("account側の検証がその他のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("validate onboarding name boom")
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{err: errBoom}, &fakePlayerReader{})

			err := svc.UpdateName(context.Background(), "TST-0001", "たろう")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("表示名が有効でも、outboxイベントのpublishが失敗するとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("publish onboarding-name-set boom")
			repo := &fakeOnboardingRepo{publishErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, &fakePlayerReader{})

			err := svc.UpdateName(context.Background(), "TST-0001", "たろう")

			require.ErrorIs(t, err, errBoom)
		})
	})
}

func TestSelectFaction(t *testing.T) {
	t.Run("[オンボーディング]faction選択ステップの更新", func(t *testing.T) {
		t.Run("選択可能な陣営IDを渡すと、faction選択ステップ完了イベントがpublishされ、陣営が入力値と一致する", func(t *testing.T) {
			cases := []struct {
				name      string
				factionID string
			}{
				{"陣営IDがSHEのとき", "SHE"},
				{"陣営IDがTenkiのとき", "Tenki"},
				{"陣営IDがSugarのとき", "Sugar"},
				{"陣営IDがTunersのとき", "Tuners"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					repo := &fakeOnboardingRepo{}
					svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, &fakePlayerReader{})

					err := svc.SelectFaction(context.Background(), "TST-0001", tc.factionID)

					require.NoError(t, err)
					require.Len(t, repo.publishedEvents, 1)
					published := repo.publishedEvents[0]
					assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, published.EventType)

					var wire apiscenario.OnboardingFactionSetEvent
					require.NoError(t, json.Unmarshal(published.Payload, &wire))
					assert.Equal(t, apiscenario.EventTypeOnboardingFactionSet, wire.EventType)
					assert.Equal(t, tc.factionID, wire.InitialFactionID)
				})
			}
		})

		t.Run("選択可能でない陣営IDを渡すと、無効な陣営を表すエラーになる", func(t *testing.T) {
			cases := []struct {
				name      string
				factionID string
			}{
				{"未知の陣営IDのとき", "TST-UNKNOWN-FACTION"},
				{"空文字のとき", ""},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{}, &fakePlayerReader{})

					err := svc.SelectFaction(context.Background(), "TST-0001", tc.factionID)

					require.ErrorIs(t, err, ErrInvalidFaction)
				})
			}
		})

		t.Run("選択可能な陣営IDを渡しても、outboxイベントのpublishが失敗するとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("publish onboarding-faction-set boom")
			repo := &fakeOnboardingRepo{publishErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, &fakePlayerReader{})

			err := svc.SelectFaction(context.Background(), "TST-0001", "SHE")

			require.ErrorIs(t, err, errBoom)
		})
	})
}

func TestComplete(t *testing.T) {
	t.Run("[オンボーディング]オンボーディング完了", func(t *testing.T) {
		t.Run("プレイヤーがまだ陣営未選択のとき、陣営未選択を表すエラーになる", func(t *testing.T) {
			cases := []struct {
				name           string
				initialFaction *string
			}{
				{"初期陣営が未設定 (nil) のとき", nil},
				{"初期陣営が空文字のとき", ptr("")},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					reader := &fakePlayerReader{player: port.AccountPlayer{PlayerID: "TST-0001", InitialFaction: tc.initialFaction}}
					svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{}, reader)

					err := svc.Complete(context.Background(), "TST-0001")

					require.ErrorIs(t, err, ErrFactionNotSelected)
				})
			}
		})

		t.Run("対象プレイヤーがaccountに存在しないとき、プレイヤー未検出を表すエラーになる", func(t *testing.T) {
			reader := &fakePlayerReader{err: port.ErrPlayerNotFound}
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{}, reader)

			err := svc.Complete(context.Background(), "TST-0001")

			require.ErrorIs(t, err, ErrPlayerNotFound)
		})

		t.Run("account側の取得がその他のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("get onboarding player boom")
			reader := &fakePlayerReader{err: errBoom}
			svc := New(&fakeOnboardingRepo{}, &fakeScriptStore{}, &fakeNameValidator{}, reader)

			err := svc.Complete(context.Background(), "TST-0001")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("既にオンボーディング完了済みのプレイヤーが完了しようとすると、オンボーディング済みを表すエラーになる", func(t *testing.T) {
			faction := "TST-FACTION"
			reader := &fakePlayerReader{player: port.AccountPlayer{PlayerID: "TST-0001", InitialFaction: &faction}}
			repo := &fakeOnboardingRepo{markCompleteErr: port.ErrAlreadyOnboarded}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, reader)

			err := svc.Complete(context.Background(), "TST-0001")

			require.ErrorIs(t, err, ErrAlreadyOnboarded)
		})

		t.Run("完了記録がオンボーディング済み以外のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			faction := "TST-FACTION"
			reader := &fakePlayerReader{player: port.AccountPlayer{PlayerID: "TST-0001", InitialFaction: &faction}}
			errBoom := errors.New("mark complete boom")
			repo := &fakeOnboardingRepo{markCompleteErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, reader)

			err := svc.Complete(context.Background(), "TST-0001")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("未完了かつ陣営選択済みのプレイヤーが完了すると、完了が記録され、player-onboardedイベントがoutboxに記録される", func(t *testing.T) {
			faction := "TST-FACTION"
			reader := &fakePlayerReader{player: port.AccountPlayer{PlayerID: "TST-0001", InitialFaction: &faction}}
			repo := &fakeOnboardingRepo{}
			svc := New(repo, &fakeScriptStore{}, &fakeNameValidator{}, reader)

			err := svc.Complete(context.Background(), "TST-0001")

			require.NoError(t, err)
			require.Len(t, repo.markCompleteCalls, 1)
			call := repo.markCompleteCalls[0]
			assert.Equal(t, "TST-0001", call.playerID)
			require.Len(t, call.events, 1)
			assert.Equal(t, apiscenario.EventTypePlayerOnboarded, call.events[0].EventType)

			var wire apiscenario.PlayerOnboardedEvent
			require.NoError(t, json.Unmarshal(call.events[0].Payload, &wire))
			assert.Equal(t, apiscenario.EventTypePlayerOnboarded, wire.EventType)
			assert.Equal(t, "TST-0001", wire.PlayerID)
			assert.Equal(t, faction, wire.InitialFactionID)
		})
	})
}
