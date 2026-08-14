//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
)

func TestStoryRepository_ListActiveEpisodes(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[シナリオ]アクティブなエピソード一覧の取得", func(t *testing.T) {
		t.Run("アクティブなエピソードが存在するとき、表示順で一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0002", nil, 2, "second", "Second", 1, nil, "path/{lang}", 2, true)
			seedEpisode(t, "TST-EP-0001", nil, 1, "first", "First", 1, nil, "path/{lang}", 1, true)

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)
			require.Len(t, got, 2)
			assert.Equal(t, []string{"TST-EP-0001", "TST-EP-0002"}, []string{got[0].EpisodeID, got[1].EpisodeID})
		})

		t.Run("アクティブなエピソードが1件も無いとき、空の一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "inactive1", "Inactive1", 1, nil, "path/{lang}", 1, false)
			seedEpisode(t, "TST-EP-0002", nil, 2, "inactive2", "Inactive2", 1, nil, "path/{lang}", 2, false)

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("非アクティブなエピソードは一覧に含まれない", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "active", "Active", 1, nil, "path/{lang}", 1, true)
			seedEpisode(t, "TST-EP-0002", nil, 2, "inactive", "Inactive", 1, nil, "path/{lang}", 2, false)

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "TST-EP-0001", got[0].EpisodeID)
		})

		t.Run("エピソードに必須陣営が複数登録されているとき、それらが全て一覧の該当エピソードに反映される", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "multi", "Multi", 1, nil, "path/{lang}", 1, true)
			seedRequiredFaction(t, "TST-EP-0001", "SHE")
			seedRequiredFaction(t, "TST-EP-0001", "Tenki")

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.ElementsMatch(t, []string{"SHE", "Tenki"}, got[0].RequiredFactions)
		})

		t.Run("エピソードに必須陣営が登録されていないとき、必須陣営は空になる", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "none", "None", 1, nil, "path/{lang}", 1, true)

			got, err := repo.ListActiveEpisodes(ctx)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Empty(t, got[0].RequiredFactions)
		})
	})
}

func TestStoryRepository_FindEpisodeByID(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[シナリオ]IDによるエピソード取得", func(t *testing.T) {
		t.Run("指定IDのエピソードが存在するとき、そのエピソードが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", strPtr("SHE"), 1, "タイトル", "Title", 5, nil, "path/{lang}", 1, true)

			got, err := repo.FindEpisodeByID(ctx, "TST-EP-0001")
			require.NoError(t, err)
			assert.Equal(t, "TST-EP-0001", got.EpisodeID)
			assert.Equal(t, "タイトル", got.TitleJa)
			assert.Equal(t, int64(5), got.RequiredLevel)
		})

		t.Run("指定IDのエピソードが存在しないとき、未検出を表すエラーになる", func(t *testing.T) {
			sharedPg.Truncate(t)

			_, err := repo.FindEpisodeByID(ctx, "TST-EP-9999")
			require.ErrorIs(t, err, port.ErrNotFound)
		})

		t.Run("指定IDのエピソードが非アクティブでも、絞り込まずそのまま返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "inactive", "Inactive", 1, nil, "path/{lang}", 1, false)

			got, err := repo.FindEpisodeByID(ctx, "TST-EP-0001")
			require.NoError(t, err)
			assert.False(t, got.IsActive)
		})

		t.Run("指定IDのエピソードに必須陣営が登録されているとき、その一覧が含まれる", func(t *testing.T) {
			sharedPg.Truncate(t)
			seedEpisode(t, "TST-EP-0001", nil, 1, "multi", "Multi", 1, nil, "path/{lang}", 1, true)
			seedRequiredFaction(t, "TST-EP-0001", "SHE")
			seedRequiredFaction(t, "TST-EP-0001", "Tenki")

			got, err := repo.FindEpisodeByID(ctx, "TST-EP-0001")
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"SHE", "Tenki"}, got.RequiredFactions)
		})
	})
}

func TestStoryRepository_GetCompletedEpisodeIDs(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[シナリオ]完了済みエピソードID一覧の取得", func(t *testing.T) {
		t.Run("対象プレイヤーの完了記録が存在するとき、そのエピソードID一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			seedEpisode(t, "TST-EP-0001", nil, 1, "ep1", "Ep1", 1, nil, "path/{lang}", 1, true)
			seedEpisode(t, "TST-EP-0002", nil, 2, "ep2", "Ep2", 1, nil, "path/{lang}", 2, true)
			seedProgress(t, playerID, "TST-EP-0001")
			seedProgress(t, playerID, "TST-EP-0002")

			got, err := repo.GetCompletedEpisodeIDs(ctx, playerID)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"TST-EP-0001", "TST-EP-0002"}, got)
		})

		t.Run("対象プレイヤーの完了記録が存在しないとき、空の一覧が返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()

			got, err := repo.GetCompletedEpisodeIDs(ctx, playerID)
			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("複数プレイヤーの完了記録が存在するとき、指定したプレイヤーのエピソードID一覧のみが返る", func(t *testing.T) {
			sharedPg.Truncate(t)
			targetPlayerID := uuid.New().String()
			otherPlayerID := uuid.New().String()
			seedEpisode(t, "TST-EP-0001", nil, 1, "ep1", "Ep1", 1, nil, "path/{lang}", 1, true)
			seedEpisode(t, "TST-EP-0002", nil, 2, "ep2", "Ep2", 1, nil, "path/{lang}", 2, true)
			seedProgress(t, targetPlayerID, "TST-EP-0001")
			seedProgress(t, otherPlayerID, "TST-EP-0002")

			got, err := repo.GetCompletedEpisodeIDs(ctx, targetPlayerID)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"TST-EP-0001"}, got)
		})
	})
}

func TestStoryRepository_MarkComplete(t *testing.T) {
	repo := postgres.NewStoryRepository(sharedPg.Pool)
	ctx := context.Background()

	t.Run("[シナリオ]エピソード完了の記録", func(t *testing.T) {
		t.Run("未完了のエピソードを完了にすると、完了記録が作成される", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			seedEpisode(t, "TST-EP-0001", nil, 1, "ep1", "Ep1", 1, nil, "path/{lang}", 1, true)

			require.NoError(t, repo.MarkComplete(ctx, playerID, "TST-EP-0001"))

			assert.Equal(t, 1, countProgress(t, playerID, "TST-EP-0001"))
		})

		t.Run("既に完了記録があるエピソードを再度完了にしても、エラーにならず記録は重複しない", func(t *testing.T) {
			sharedPg.Truncate(t)
			playerID := uuid.New().String()
			seedEpisode(t, "TST-EP-0001", nil, 1, "ep1", "Ep1", 1, nil, "path/{lang}", 1, true)
			require.NoError(t, repo.MarkComplete(ctx, playerID, "TST-EP-0001"))

			require.NoError(t, repo.MarkComplete(ctx, playerID, "TST-EP-0001"))

			assert.Equal(t, 1, countProgress(t, playerID, "TST-EP-0001"))
		})
	})
}
