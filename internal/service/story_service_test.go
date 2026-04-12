package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository"
)

type testStoryEnv struct {
	svc       *StoryService
	storyRepo *repository.MockStoryRepository
}

func newTestStoryEnv() *testStoryEnv {
	storyRepo := repository.NewMockStoryRepository()
	svc := NewStoryService(storyRepo, nil, nil)
	return &testStoryEnv{svc: svc, storyRepo: storyRepo}
}

func seedTestEpisodes(env *testStoryEnv) {
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

func TestListEpisodes_UnlockedAndLockedByLevel(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 5)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)

	episodes, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
	require.NoError(t, err)
	require.Len(t, episodes, 2)

	assert.Equal(t, "she_ep1", episodes[0].EpisodeID)
	assert.True(t, episodes[0].IsUnlocked)
	assert.Empty(t, episodes[0].LockReasons)

	assert.Equal(t, "she_ep2", episodes[1].EpisodeID)
	assert.False(t, episodes[1].IsUnlocked)
	require.NotEmpty(t, episodes[1].LockReasons)
	assert.NotNil(t, findReasonByType(episodes[1].LockReasons, "level"))
}

func TestListEpisodes_LockedByFaction(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 10)
	seedTestEpisodes(env)

	episodes, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
	require.NoError(t, err)

	require.Len(t, episodes, 2)
	assert.False(t, episodes[0].IsUnlocked)
	require.NotEmpty(t, episodes[0].LockReasons)
	r := findReasonByType(episodes[0].LockReasons, "faction")
	require.NotNil(t, r)
	assert.Equal(t, "SHE", r.Required)
}

func TestListEpisodes_LockedByEpisode(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 10)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)

	episodes, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
	require.NoError(t, err)

	ep2 := episodes[1]
	assert.False(t, ep2.IsUnlocked)
	require.NotEmpty(t, ep2.LockReasons)
	assert.NotNil(t, findReasonByType(ep2.LockReasons, "episode"))
}

func TestListEpisodes_EnglishTitle(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 10)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)

	episodes, err := env.svc.ListEpisodes(context.Background(), "p1", "en")
	require.NoError(t, err)

	assert.Equal(t, "SHE Chapter 1", episodes[0].Title)
}

func TestListEpisodes_CompletedStatus(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 10)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)
	_ = env.storyRepo.MarkComplete(context.Background(), "p1", "she_ep1")

	episodes, err := env.svc.ListEpisodes(context.Background(), "p1", "ja")
	require.NoError(t, err)

	assert.True(t, episodes[0].IsCompleted)
	assert.False(t, episodes[1].IsCompleted)
	assert.True(t, episodes[1].IsUnlocked, "completing ep1 unlocks ep2")
}

func TestCompleteEpisode_Success(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 5)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)

	err := env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1")
	require.NoError(t, err)

	ids, _ := env.storyRepo.GetCompletedEpisodeIDs(context.Background(), "p1")
	assert.Contains(t, ids, "she_ep1")
}

func TestCompleteEpisode_Idempotent(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 5)
	env.storyRepo.GrantFaction("p1", "SHE")
	seedTestEpisodes(env)

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

func TestCompleteEpisode_NotFound(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 5)
	seedTestEpisodes(env)

	err := env.svc.CompleteEpisode(context.Background(), "p1", "nonexistent")
	assert.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestCompleteEpisode_Locked(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 1)
	seedTestEpisodes(env)

	err := env.svc.CompleteEpisode(context.Background(), "p1", "she_ep1")
	assert.ErrorIs(t, err, ErrEpisodeLocked)
}

func TestCompleteEpisode_InactiveEpisode(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 99)
	seedTestEpisodes(env)

	err := env.svc.CompleteEpisode(context.Background(), "p1", "inactive_ep")
	assert.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestGetScript_NotFound(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 10)
	seedTestEpisodes(env)

	_, err := env.svc.GetScript(context.Background(), "p1", "nonexistent", "ja")
	assert.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestGetScript_InactiveEpisode(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 99)
	seedTestEpisodes(env)

	_, err := env.svc.GetScript(context.Background(), "p1", "inactive_ep", "ja")
	assert.ErrorIs(t, err, ErrEpisodeNotFound)
}

func TestGetScript_Locked(t *testing.T) {
	env := newTestStoryEnv()
	env.storyRepo.SetPlayerLevel("p1", 1)
	seedTestEpisodes(env)

	_, err := env.svc.GetScript(context.Background(), "p1", "she_ep1", "ja")
	assert.ErrorIs(t, err, ErrEpisodeLocked)
}

func TestCheckUnlock_AllConditionsMet(t *testing.T) {
	ep := &apiscenario.ScenarioEpisode{
		RequiredLevel:    5,
		RequiredFactions: []string{"SHE"},
		RequiredEpisodes: []string{"she_ep1"},
	}
	uc := &apiscenario.StoryUnlockContext{
		PlayerLevel:       10,
		OwnedFactions:     map[string]bool{"SHE": true},
		CompletedEpisodes: map[string]bool{"she_ep1": true},
	}
	reasons := checkUnlock(ep, uc)
	assert.Empty(t, reasons)
}

func TestCheckUnlock_AllConditionsUnmet(t *testing.T) {
	ep := &apiscenario.ScenarioEpisode{
		RequiredLevel:    5,
		RequiredFactions: []string{"SHE"},
		RequiredEpisodes: []string{"she_ep1"},
	}
	uc := &apiscenario.StoryUnlockContext{
		PlayerLevel:       1,
		OwnedFactions:     map[string]bool{},
		CompletedEpisodes: map[string]bool{},
	}
	reasons := checkUnlock(ep, uc)
	require.Len(t, reasons, 3)
	assert.Equal(t, "level", reasons[0].Type)
	assert.Equal(t, "faction", reasons[1].Type)
	assert.Equal(t, "episode", reasons[2].Type)
}

// fakeFactionPublisher は PublishFactionSelected の呼び出しを記録し、エラーをシミュレートできる。
type fakeFactionPublisher struct {
	calls []fakePublishCall
	err   error
}

type fakePublishCall struct {
	PlayerID string
	Faction  string
}

func (f *fakeFactionPublisher) PublishFactionSelected(_ context.Context, playerID, faction string) error {
	f.calls = append(f.calls, fakePublishCall{PlayerID: playerID, Faction: faction})
	return f.err
}

func TestNotifyInitialFactionSelected_Publishes(t *testing.T) {
	storyRepo := repository.NewMockStoryRepository()
	pub := &fakeFactionPublisher{}
	svc := NewStoryService(storyRepo, nil, pub)

	err := svc.NotifyInitialFactionSelected(context.Background(), "p1", "SHE")
	require.NoError(t, err)

	require.Len(t, pub.calls, 1)
	assert.Equal(t, "p1", pub.calls[0].PlayerID)
	assert.Equal(t, "SHE", pub.calls[0].Faction)
}

func TestNotifyInitialFactionSelected_PropagatesPublishError(t *testing.T) {
	storyRepo := repository.NewMockStoryRepository()
	pub := &fakeFactionPublisher{err: errors.New("simulated pubsub unavailable")}
	svc := NewStoryService(storyRepo, nil, pub)

	err := svc.NotifyInitialFactionSelected(context.Background(), "p1", "Tenki")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated pubsub unavailable")
	require.Len(t, pub.calls, 1)
}

func TestNotifyInitialFactionSelected_NilPublisherReturnsError(t *testing.T) {
	svc := NewStoryService(repository.NewMockStoryRepository(), nil, nil)

	err := svc.NotifyInitialFactionSelected(context.Background(), "p1", "SHE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil factionPublisher")
}
