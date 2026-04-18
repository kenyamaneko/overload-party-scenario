package story

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

type testEnv struct {
	svc       *Service
	storyRepo *port.MockStoryRepository
}

func newTestEnv() *testEnv {
	storyRepo := port.NewMockStoryRepository()
	return &testEnv{
		svc:       New(storyRepo, nil, nil),
		storyRepo: storyRepo,
	}
}

func seedTestEpisodes(env *testEnv) {
	faction := "SHE"
	env.storyRepo.SeedEpisodes([]*apiscenario.ScenarioEpisode{
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

func findReasonByType(reasons []apiscenario.LockReason, typ string) *apiscenario.LockReason {
	for i := range reasons {
		if reasons[i].Type == typ {
			return &reasons[i]
		}
	}
	return nil
}

func TestListEpisodes(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(env *testEnv)
		lang   string
		verify func(t *testing.T, eps []apiscenario.EpisodeWithStatus)
	}{
		{
			name: "レベル未達のエピソードはロックされる",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 5)
				env.storyRepo.GrantFaction("p1", "SHE")
			},
			lang: "ja",
			verify: func(t *testing.T, eps []apiscenario.EpisodeWithStatus) {
				require.Len(t, eps, 2)
				assert.True(t, eps[0].IsUnlocked)
				assert.False(t, eps[1].IsUnlocked)
				assert.NotNil(t, findReasonByType(eps[1].LockReasons, "level"))
			},
		},
		{
			name: "faction 未所有はロック理由 faction を返す",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 10)
			},
			lang: "ja",
			verify: func(t *testing.T, eps []apiscenario.EpisodeWithStatus) {
				require.Len(t, eps, 2)
				r := findReasonByType(eps[0].LockReasons, "faction")
				require.NotNil(t, r)
				assert.Equal(t, "SHE", r.Required)
			},
		},
		{
			name: "前提エピソード未完了はロック理由 episode を返す",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 10)
				env.storyRepo.GrantFaction("p1", "SHE")
			},
			lang: "ja",
			verify: func(t *testing.T, eps []apiscenario.EpisodeWithStatus) {
				require.Len(t, eps, 2)
				assert.False(t, eps[1].IsUnlocked)
				assert.NotNil(t, findReasonByType(eps[1].LockReasons, "episode"))
			},
		},
		{
			name: "lang=en は英語タイトルを返す",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 10)
				env.storyRepo.GrantFaction("p1", "SHE")
			},
			lang: "en",
			verify: func(t *testing.T, eps []apiscenario.EpisodeWithStatus) {
				assert.Equal(t, "SHE Chapter 1", eps[0].Title)
			},
		},
		{
			name: "完了済みエピソードは IsCompleted が true で後続をアンロックする",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 10)
				env.storyRepo.GrantFaction("p1", "SHE")
				_ = env.storyRepo.MarkComplete(context.Background(), "p1", "she_ep1")
			},
			lang: "ja",
			verify: func(t *testing.T, eps []apiscenario.EpisodeWithStatus) {
				assert.True(t, eps[0].IsCompleted)
				assert.True(t, eps[1].IsUnlocked)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			tc.setup(env)

			eps, err := env.svc.ListEpisodes(context.Background(), "p1", tc.lang)
			require.NoError(t, err)
			tc.verify(t, eps)
		})
	}
}

func TestCompleteEpisode(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(env *testEnv)
		episodeID string
		wantErr   error
	}{
		{
			name: "アンロック済みエピソードを完了できる",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 5)
				env.storyRepo.GrantFaction("p1", "SHE")
			},
			episodeID: "she_ep1",
		},
		{
			name: "存在しないエピソードは ErrEpisodeNotFound",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 5)
			},
			episodeID: "nonexistent",
			wantErr:   ErrEpisodeNotFound,
		},
		{
			name: "ロック中のエピソードは ErrEpisodeLocked",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 1)
			},
			episodeID: "she_ep1",
			wantErr:   ErrEpisodeLocked,
		},
		{
			name: "非アクティブエピソードは ErrEpisodeNotFound",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 99)
			},
			episodeID: "inactive_ep",
			wantErr:   ErrEpisodeNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			tc.setup(env)

			err := env.svc.CompleteEpisode(context.Background(), "p1", tc.episodeID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCompleteEpisode_Idempotent(t *testing.T) {
	env := newTestEnv()
	seedTestEpisodes(env)
	env.storyRepo.SetPlayerLevel("p1", 5)
	env.storyRepo.GrantFaction("p1", "SHE")

	require.NoError(t, env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1"))
	require.NoError(t, env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1"))

	ids, _ := env.storyRepo.GetCompletedEpisodeIDs(context.Background(), "p1")
	count := 0
	for _, id := range ids {
		if id == "she_ep1" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestGetScript(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(env *testEnv)
		episodeID string
		lang      string
		wantErr   error
	}{
		{
			name: "存在しないエピソードは ErrEpisodeNotFound",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 10)
			},
			episodeID: "nonexistent",
			lang:      "ja",
			wantErr:   ErrEpisodeNotFound,
		},
		{
			name: "非アクティブエピソードは ErrEpisodeNotFound",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 99)
			},
			episodeID: "inactive_ep",
			lang:      "ja",
			wantErr:   ErrEpisodeNotFound,
		},
		{
			name: "ロック中のエピソードは ErrEpisodeLocked",
			setup: func(env *testEnv) {
				env.storyRepo.SetPlayerLevel("p1", 1)
			},
			episodeID: "she_ep1",
			lang:      "ja",
			wantErr:   ErrEpisodeLocked,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv()
			seedTestEpisodes(env)
			tc.setup(env)

			_, err := env.svc.GetScript(context.Background(), "p1", tc.episodeID, tc.lang)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGetScript_NoLanguageFallback(t *testing.T) {
	env := newTestEnv()
	seedTestEpisodes(env)
	env.storyRepo.SetPlayerLevel("p1", 10)
	env.storyRepo.GrantFaction("p1", "SHE")

	fake := &fakeScriptStore{missing: true}
	svc := New(env.storyRepo, fake, nil)

	_, err := svc.GetScript(context.Background(), "p1", "she_ep1", "en")
	require.Error(t, err)
	assert.ErrorIs(t, err, port.ErrScriptNotFound)
	require.Len(t, fake.calls, 1, "フォールバック廃止のため要求言語で一度のみ読みに行く")
	assert.Equal(t, "stories/en/she_ep1.ks", fake.calls[0])
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

func TestCheckUnlock(t *testing.T) {
	tests := []struct {
		name        string
		ep          *apiscenario.ScenarioEpisode
		uc          *apiscenario.StoryUnlockContext
		wantReasons []string
	}{
		{
			name: "全条件を満たす場合は理由なし",
			ep: &apiscenario.ScenarioEpisode{
				RequiredLevel:    5,
				RequiredFactions: []string{"SHE"},
				RequiredEpisodes: []string{"she_ep1"},
			},
			uc: &apiscenario.StoryUnlockContext{
				PlayerLevel:       10,
				OwnedFactions:     map[string]bool{"SHE": true},
				CompletedEpisodes: map[string]bool{"she_ep1": true},
			},
			wantReasons: nil,
		},
		{
			name: "全条件未達は level, faction, episode の3理由を返す",
			ep: &apiscenario.ScenarioEpisode{
				RequiredLevel:    5,
				RequiredFactions: []string{"SHE"},
				RequiredEpisodes: []string{"she_ep1"},
			},
			uc: &apiscenario.StoryUnlockContext{
				PlayerLevel:       1,
				OwnedFactions:     map[string]bool{},
				CompletedEpisodes: map[string]bool{},
			},
			wantReasons: []string{"level", "faction", "episode"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reasons := checkUnlock(tc.ep, tc.uc)
			require.Len(t, reasons, len(tc.wantReasons))
			for i, want := range tc.wantReasons {
				assert.Equal(t, want, reasons[i].Type)
			}
		})
	}
}

type fakeFactionPublisher struct {
	calls []struct{ PlayerID, Faction string }
	err   error
}

func (f *fakeFactionPublisher) PublishFactionSelected(_ context.Context, playerID, faction string) error {
	f.calls = append(f.calls, struct{ PlayerID, Faction string }{playerID, faction})
	return f.err
}

func TestNotifyInitialFactionSelected(t *testing.T) {
	t.Run("publisher が nil なら明示的エラー", func(t *testing.T) {
		svc := New(port.NewMockStoryRepository(), nil, nil)
		err := svc.NotifyInitialFactionSelected(context.Background(), "p1", "SHE")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil factionPublisher")
	})

	t.Run("publisher の成功は呼び出しを記録する", func(t *testing.T) {
		pub := &fakeFactionPublisher{}
		svc := New(port.NewMockStoryRepository(), nil, pub)

		require.NoError(t, svc.NotifyInitialFactionSelected(context.Background(), "p1", "SHE"))
		require.Len(t, pub.calls, 1)
		assert.Equal(t, "p1", pub.calls[0].PlayerID)
		assert.Equal(t, "SHE", pub.calls[0].Faction)
	})

	t.Run("publisher のエラーは伝播する", func(t *testing.T) {
		pub := &fakeFactionPublisher{err: errors.New("simulated pubsub unavailable")}
		svc := New(port.NewMockStoryRepository(), nil, pub)

		err := svc.NotifyInitialFactionSelected(context.Background(), "p1", "Tenki")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulated pubsub unavailable")
	})
}
