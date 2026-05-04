package postgres_test

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
)

// fixed UUID 文字列（テスト用 player_id）。FK 越しに実 UUID 型へ入る必要があるため
// 生成時に衝突しない固定値を使う。
const (
	testPlayer1 = "11111111-1111-1111-1111-111111111111"
	testPlayer2 = "22222222-2222-2222-2222-222222222222"
)

func TestListActiveEpisodes(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	type episodeSeed struct {
		episodeID        string
		faction          *string
		episodeNumber    int64
		sortOrder        int64
		isActive         bool
		requiredFactions []string
	}

	tests := []struct {
		name    string
		seeds   []episodeSeed
		wantIDs []string // 期待される順序（sort_order 昇順）
	}{
		{
			name: "active のみ sort_order 昇順で返り inactive は除外",
			seeds: []episodeSeed{
				{"ep_b", strPtr("SHE"), 2, 20, true, nil},
				{"ep_a", strPtr("SHE"), 1, 10, true, []string{"SHE"}},
				{"ep_inactive", strPtr("Tenki"), 1, 15, false, nil},
				{"ep_c", nil, 1, 30, true, []string{"Neutral", "Tuners"}},
			},
			wantIDs: []string{"ep_a", "ep_b", "ep_c"},
		},
		{
			name:    "シードなしなら空スライス",
			seeds:   nil,
			wantIDs: nil,
		},
		{
			name: "全て inactive なら空スライス",
			seeds: []episodeSeed{
				{"ep_x", strPtr("SHE"), 1, 10, false, nil},
				{"ep_y", strPtr("Tenki"), 1, 20, false, nil},
			},
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				seedEpisode(t, s.episodeID, s.faction, s.episodeNumber,
					s.episodeID+" JP", s.episodeID+" EN",
					1, nil, "path/"+s.episodeID+"/{lang}.json", s.sortOrder, s.isActive)
				for _, f := range s.requiredFactions {
					seedRequiredFaction(t, s.episodeID, f)
				}
			}

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)

			var gotIDs []string
			for _, ep := range got {
				gotIDs = append(gotIDs, ep.EpisodeID)
			}
			assert.Equal(t, tt.wantIDs, gotIDs)
		})
	}
}

func TestListActiveEpisodes_AggregatesRequiredFactions(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	seedEpisode(t, "ep1", strPtr("SHE"), 1, "JA", "EN", 5, nil, "s/ep1/{lang}.json", 1, true)
	seedRequiredFaction(t, "ep1", "SHE")
	seedRequiredFaction(t, "ep1", "Tenki")

	seedEpisode(t, "ep2", nil, 1, "JA", "EN", 1, nil, "s/ep2/{lang}.json", 2, true)
	// ep2 には required faction を入れない（空配列期待）

	eps, err := repo.ListActiveEpisodes(ctx)
	require.NoError(t, err)
	require.Len(t, eps, 2)

	got := map[string][]string{}
	for _, ep := range eps {
		got[ep.EpisodeID] = ep.RequiredFactions
	}

	// faction_id 昇順で返すのでソート済みのはず
	assert.Equal(t, []string{"SHE", "Tenki"}, got["ep1"])
	assert.Empty(t, got["ep2"])
}

func TestFindEpisodeByID(t *testing.T) {
	sharedPg.Truncate(t)
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	seedEpisode(t, "ep_found", strPtr("SHE"), 3, "見つかる", "Found", 5,
		[]string{"prev1"}, "s/ep_found/{lang}.json", 42, true)
	seedRequiredFaction(t, "ep_found", "SHE")

	tests := []struct {
		name        string
		episodeID   string
		wantErrIs   error
		wantTitleJa string
		wantLevel   int64
		wantFaction []string
	}{
		{
			name:        "存在する ID は取得成功",
			episodeID:   "ep_found",
			wantTitleJa: "見つかる",
			wantLevel:   5,
			wantFaction: []string{"SHE"},
		},
		{
			name:      "存在しない ID は ErrNotFound",
			episodeID: "missing",
			wantErrIs: port.ErrNotFound,
		},
		{
			name:      "空文字 ID も ErrNotFound",
			episodeID: "",
			wantErrIs: port.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.FindEpisodeByID(ctx, tt.episodeID)
			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantTitleJa, got.TitleJa)
			assert.Equal(t, tt.wantLevel, got.RequiredLevel)
			assert.Equal(t, tt.wantFaction, got.RequiredFactions)
		})
	}
}

func TestGetCompletedEpisodeIDs(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name     string
		seeds    map[string][]string // playerID -> episodeIDs
		target   string
		episodes []string // 事前に必要な episode マスター
		wantIDs  []string
	}{
		{
			name:     "指定プレイヤーの完了済み ID のみ返る",
			episodes: []string{"ep1", "ep2", "ep3"},
			seeds: map[string][]string{
				testPlayer1: {"ep1", "ep2"},
				testPlayer2: {"ep3"},
			},
			target:  testPlayer1,
			wantIDs: []string{"ep1", "ep2"},
		},
		{
			name:     "進行履歴がなければ空",
			episodes: []string{"ep1"},
			seeds:    nil,
			target:   testPlayer1,
			wantIDs:  nil,
		},
		{
			name:     "他プレイヤー分は混入しない",
			episodes: []string{"ep1"},
			seeds: map[string][]string{
				testPlayer2: {"ep1"},
			},
			target:  testPlayer1,
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for i, epID := range tt.episodes {
				seedEpisode(t, epID, nil, 1, "JA", "EN", 1, nil,
					"s/"+epID+"/{lang}.json", int64(i+1), true)
			}
			for pid, ids := range tt.seeds {
				for _, id := range ids {
					seedProgress(t, pid, id)
				}
			}

			got, err := repo.GetCompletedEpisodeIDs(ctx, tt.target)
			require.NoError(t, err)
			sort.Strings(got)
			sort.Strings(tt.wantIDs)
			assert.Equal(t, tt.wantIDs, got)
		})
	}
}

func TestGetUnlockContext(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	tests := []struct {
		name               string
		setup              func(t *testing.T)
		playerID           string
		wantErr            bool
		wantLevel          int64
		wantFactions       map[string]bool
		wantCompletedCount int
	}{
		{
			name: "player + factions + progress を集約して返す",
			setup: func(t *testing.T) {
				seedPlayer(t, testPlayer1, 12)
				seedPlayerFaction(t, testPlayer1, "SHE")
				seedPlayerFaction(t, testPlayer1, "Tenki")
				seedEpisode(t, "ep1", nil, 1, "JA", "EN", 1, nil, "s/ep1/{lang}.json", 1, true)
				seedProgress(t, testPlayer1, "ep1")
			},
			playerID:           testPlayer1,
			wantLevel:          12,
			wantFactions:       map[string]bool{"SHE": true, "Tenki": true},
			wantCompletedCount: 1,
		},
		{
			name: "faction / progress が無くても level は返る",
			setup: func(t *testing.T) {
				seedPlayer(t, testPlayer1, 3)
			},
			playerID:           testPlayer1,
			wantLevel:          3,
			wantFactions:       map[string]bool{},
			wantCompletedCount: 0,
		},
		{
			name: "存在しないプレイヤーはエラー",
			setup: func(t *testing.T) {
				seedPlayer(t, testPlayer2, 1)
			},
			playerID: testPlayer1,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			tt.setup(t)

			got, err := repo.GetUnlockContext(ctx, tt.playerID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantLevel, got.PlayerLevel)
			assert.Equal(t, tt.wantFactions, got.OwnedFactions)
			assert.Len(t, got.CompletedEpisodes, tt.wantCompletedCount)
		})
	}
}

func TestStoryRepository_MarkComplete(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("新規登録は 1 行追加", func(t *testing.T) {
		sharedPg.Truncate(t)
		seedEpisode(t, "ep1", nil, 1, "JA", "EN", 1, nil, "s/ep1/{lang}.json", 1, true)

		require.NoError(t, repo.MarkComplete(ctx, testPlayer1, "ep1"))

		var n int
		err := sharedPg.Pool.QueryRow(ctx,
			`SELECT count(*) FROM scenario.player_story_progress WHERE player_id = $1 AND episode_id = $2`,
			testPlayer1, "ep1").Scan(&n)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})

	t.Run("冪等: 2 回呼んでも 1 行のまま", func(t *testing.T) {
		sharedPg.Truncate(t)
		seedEpisode(t, "ep1", nil, 1, "JA", "EN", 1, nil, "s/ep1/{lang}.json", 1, true)

		require.NoError(t, repo.MarkComplete(ctx, testPlayer1, "ep1"))
		require.NoError(t, repo.MarkComplete(ctx, testPlayer1, "ep1"))

		var n int
		err := sharedPg.Pool.QueryRow(ctx,
			`SELECT count(*) FROM scenario.player_story_progress WHERE player_id = $1`,
			testPlayer1).Scan(&n)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})

	t.Run("存在しない episode_id は FK 違反", func(t *testing.T) {
		sharedPg.Truncate(t)

		err := repo.MarkComplete(ctx, testPlayer1, "no_such_episode")
		assert.Error(t, err)
	})
}
