package story

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

type testEnv struct {
	svc       *Service
	storyRepo *mockStoryRepository
	account   *fakePlayerProgressReader
}

func newTestEnv() *testEnv {
	storyRepo := newMockStoryRepository()
	account := &fakePlayerProgressReader{}
	return &testEnv{
		svc:       New(storyRepo, nil, account),
		storyRepo: storyRepo,
		account:   account,
	}
}

func (e *testEnv) setPlayerProgress(level int64, ownedFactions ...string) {
	e.account.progress = port.PlayerProgress{Level: level, OwnedFactions: ownedFactions}
}

func seedTestEpisodes(env *testEnv) {
	faction := "SHE"
	env.storyRepo.SeedEpisodes([]*domain.Episode{
		{
			EpisodeID:        "she_ep1",
			Faction:          &faction,
			EpisodeNumber:    1,
			TitleJa:          "SHE 第1章",
			TitleEn:          "SHE Chapter 1",
			RequiredLevel:    2,
			RequiredFactions: []string{"SHE"},
			RequiredEpisodes: []string{},
			ScriptPath:       "stories/{lang}/she_ep1.ks",
			SortOrder:        1,
			IsActive:         true,
		},
		{
			EpisodeID:        "she_ep2",
			Faction:          &faction,
			EpisodeNumber:    2,
			TitleJa:          "SHE 第2章",
			TitleEn:          "SHE Chapter 2",
			RequiredLevel:    6,
			RequiredFactions: []string{"SHE"},
			RequiredEpisodes: []string{"she_ep1"},
			ScriptPath:       "stories/{lang}/she_ep2.ks",
			SortOrder:        5,
			IsActive:         true,
		},
		{
			EpisodeID:        "inactive_ep",
			Faction:          &faction,
			EpisodeNumber:    99,
			TitleJa:          "非公開エピソード",
			TitleEn:          "Inactive Episode",
			RequiredLevel:    1,
			RequiredFactions: []string{},
			RequiredEpisodes: []string{},
			ScriptPath:       "stories/{lang}/inactive.ks",
			SortOrder:        99,
			IsActive:         false,
		},
	})
}

func findReasonByType(reasons []apiscenario.LockReason, typ apiscenario.LockReasonType) *apiscenario.LockReason {
	for i := range reasons {
		if reasons[i].Type == typ {
			return &reasons[i]
		}
	}
	return nil
}

func TestListEpisodes(t *testing.T) {
	t.Run("エピソード一覧の取得", func(t *testing.T) {
		t.Run("レベル未達のとき、後続エピソードは level 理由でロックされる", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(5, "SHE")

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
			require.NoError(t, err)
			require.Len(t, eps, 2)
			assert.True(t, eps[0].IsUnlocked)
			assert.False(t, eps[1].IsUnlocked)
			assert.NotNil(t, findReasonByType(eps[1].LockReasons, apiscenario.LockReasonTypeLevel))
		})

		t.Run("faction 未所有のとき、faction 理由を返す", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(10)

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
			require.NoError(t, err)
			require.Len(t, eps, 2)
			r := findReasonByType(eps[0].LockReasons, apiscenario.LockReasonTypeFaction)
			require.NotNil(t, r)
			require.NotNil(t, r.RequiredFaction)
			assert.Equal(t, "SHE", *r.RequiredFaction)
		})

		t.Run("前提エピソード未完了のとき、episode 理由を返す", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(10, "SHE")

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
			require.NoError(t, err)
			require.Len(t, eps, 2)
			assert.False(t, eps[1].IsUnlocked)
			assert.NotNil(t, findReasonByType(eps[1].LockReasons, apiscenario.LockReasonTypeEpisode))
		})

		t.Run("lang=en のとき、英語タイトルを返す", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(10, "SHE")

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", "en")
			require.NoError(t, err)
			assert.Equal(t, "SHE Chapter 1", eps[0].Title)
		})

		t.Run("前提エピソード完了済みのとき、IsCompleted=true で後続をアンロックする", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(10, "SHE")
			_ = env.storyRepo.MarkComplete(context.Background(), "p1", "she_ep1")

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
			require.NoError(t, err)
			assert.True(t, eps[0].IsCompleted)
			assert.True(t, eps[1].IsUnlocked)
		})

		t.Run("account から到達状況を取得できないとき、一覧の取得はその理由付きで失敗する", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.account.err = errAccountUnavailable

			_, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
			assert.ErrorIs(t, err, errAccountUnavailable)
		})
	})
}

func TestCompleteEpisode(t *testing.T) {
	t.Run("エピソードの完了", func(t *testing.T) {
		tests := []struct {
			name      string
			setup     func(env *testEnv)
			episodeID string
			wantErr   error
		}{
			{
				name: "アンロック済みのとき、完了できる (エラーにならない)",
				setup: func(env *testEnv) {
					env.setPlayerProgress(5, "SHE")
				},
				episodeID: "she_ep1",
				wantErr:   nil,
			},
			{
				name: "存在しないエピソードのとき、ErrEpisodeNotFound になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(5)
				},
				episodeID: "nonexistent",
				wantErr:   ErrEpisodeNotFound,
			},
			{
				name: "ロック中のエピソードのとき、ErrEpisodeLocked になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(1)
				},
				episodeID: "she_ep1",
				wantErr:   ErrEpisodeLocked,
			},
			{
				name: "非アクティブエピソードのとき、ErrEpisodeNotFound になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(99)
				},
				episodeID: "inactive_ep",
				wantErr:   ErrEpisodeNotFound,
			},
			{
				name: "account から到達状況を取得できないとき、ロック扱いにせずその理由を返す",
				setup: func(env *testEnv) {
					env.account.err = errAccountUnavailable
				},
				episodeID: "she_ep1",
				wantErr:   errAccountUnavailable,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				env := newTestEnv()
				seedTestEpisodes(env)
				tc.setup(env)

				err := env.svc.CompleteEpisode(context.Background(), "p1", tc.episodeID)
				assert.ErrorIs(t, err, tc.wantErr)
			})
		}

		t.Run("同じエピソードを 2 回完了しても、完了 ID は 1 件のまま (冪等)", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(5, "SHE")

			require.NoError(t, env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1"))
			require.NoError(t, env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1"))

			ids, err := env.storyRepo.GetCompletedEpisodeIDs(context.Background(), "p1")
			require.NoError(t, err)
			assert.Equal(t, []string{"she_ep1"}, ids)
		})
	})
}

func TestGetScript(t *testing.T) {
	t.Run("スクリプトの取得", func(t *testing.T) {
		tests := []struct {
			name      string
			setup     func(env *testEnv)
			episodeID string
			lang      string
			wantErr   error
		}{
			{
				name: "存在しないエピソードのとき、ErrEpisodeNotFound になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(10)
				},
				episodeID: "nonexistent",
				lang:      "ja",
				wantErr:   ErrEpisodeNotFound,
			},
			{
				name: "非アクティブエピソードのとき、ErrEpisodeNotFound になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(99)
				},
				episodeID: "inactive_ep",
				lang:      "ja",
				wantErr:   ErrEpisodeNotFound,
			},
			{
				name: "ロック中のエピソードのとき、ErrEpisodeLocked になる",
				setup: func(env *testEnv) {
					env.setPlayerProgress(1)
				},
				episodeID: "she_ep1",
				lang:      "ja",
				wantErr:   ErrEpisodeLocked,
			},
			{
				name: "account から到達状況を取得できないとき、ロック扱いにせずその理由を返す",
				setup: func(env *testEnv) {
					env.account.err = errAccountUnavailable
				},
				episodeID: "she_ep1",
				lang:      "ja",
				wantErr:   errAccountUnavailable,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				env := newTestEnv()
				seedTestEpisodes(env)
				tc.setup(env)

				_, err := env.svc.GetScript(context.Background(), "p1", tc.episodeID, tc.lang)
				assert.ErrorIs(t, err, tc.wantErr)
			})
		}

		t.Run("要求言語のスクリプトが無いとき、フォールバックせず ErrScriptNotFound になる", func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			env.setPlayerProgress(10, "SHE")

			fake := &fakeScriptStore{missing: true}
			svc := New(env.storyRepo, fake, env.account)

			_, err := svc.GetScript(context.Background(), "p1", "she_ep1", "en")
			require.Error(t, err)
			assert.ErrorIs(t, err, port.ErrScriptNotFound)
			require.Len(t, fake.calls, 1, "フォールバック廃止のため要求言語で一度のみ読みに行く")
			assert.Equal(t, "stories/en/she_ep1.ks", fake.calls[0])
		})
	})
}

var errAccountUnavailable = errors.New("account unavailable")

// fakePlayerProgressReader は account から返る到達状況 (外部境界) を注入値に差し替える。
type fakePlayerProgressReader struct {
	progress port.PlayerProgress
	err      error
}

func (f *fakePlayerProgressReader) GetPlayerProgress(context.Context) (port.PlayerProgress, error) {
	return f.progress, f.err
}

type fakeScriptStore struct {
	missing bool
	calls   []string
}

func (f *fakeScriptStore) ReadScript(_ context.Context, key string) (string, error) {
	f.calls = append(f.calls, key)
	if f.missing {
		return "", port.ErrScriptNotFound
	}
	return "@endofscript\n", nil
}
