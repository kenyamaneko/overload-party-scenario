package story

import (
	"context"
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
}

func newTestEnv() *testEnv {
	storyRepo := newMockStoryRepository()
	return &testEnv{
		svc:       New(storyRepo, nil),
		storyRepo: storyRepo,
	}
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
	svc := New(env.storyRepo, fake)

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

