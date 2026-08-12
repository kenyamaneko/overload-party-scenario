package story

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

func TestListEpisodes(t *testing.T) {
	t.Run("エピソード一覧取得", func(t *testing.T) {
		t.Run("要求レベルを満たすエピソードと満たさないエピソードがあるとき、アンロック状態がそれぞれ正しく付与された一覧が返る", func(t *testing.T) {
			unlockedEp := &domain.Episode{EpisodeID: "TST-EP01", RequiredLevel: 3, IsActive: true}
			lockedEp := &domain.Episode{EpisodeID: "TST-EP02", RequiredLevel: 10, IsActive: true}
			repo := &fakeStoryRepo{activeEpisodes: []*domain.Episode{unlockedEp, lockedEp}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 5}}
			svc := New(repo, &fakeScriptStore{}, account)

			result, err := svc.ListEpisodes(context.Background(), "TST-0001", "ja")

			require.NoError(t, err)
			require.Len(t, result, 2)
			assert.Equal(t, "TST-EP01", result[0].EpisodeID)
			assert.True(t, result[0].IsUnlocked)
			assert.Equal(t, "TST-EP02", result[1].EpisodeID)
			assert.False(t, result[1].IsUnlocked)
		})

		t.Run("アクティブなエピソードが1件も無いとき、空の一覧が返る", func(t *testing.T) {
			repo := &fakeStoryRepo{activeEpisodes: []*domain.Episode{}}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			result, err := svc.ListEpisodes(context.Background(), "TST-0001", "ja")

			require.NoError(t, err)
			assert.Empty(t, result)
		})

		t.Run("エピソード一覧の取得が失敗するとき、エラーになる", func(t *testing.T) {
			errBoom := errors.New("list active episodes boom")
			repo := &fakeStoryRepo{activeErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			_, err := svc.ListEpisodes(context.Background(), "TST-0001", "ja")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("プレイヤーの到達状況の取得が失敗するとき、エラーになる", func(t *testing.T) {
			errBoom := errors.New("get player progress boom")
			repo := &fakeStoryRepo{activeEpisodes: []*domain.Episode{{EpisodeID: "TST-EP01", IsActive: true}}}
			account := &fakePlayerProgressReader{err: errBoom}
			svc := New(repo, &fakeScriptStore{}, account)

			_, err := svc.ListEpisodes(context.Background(), "TST-0001", "ja")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("プレイヤーの完了エピソード一覧の取得が失敗するとき、エラーになる", func(t *testing.T) {
			errBoom := errors.New("get completed episode ids boom")
			repo := &fakeStoryRepo{
				activeEpisodes: []*domain.Episode{{EpisodeID: "TST-EP01", IsActive: true}},
				completedErr:   errBoom,
			}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			_, err := svc.ListEpisodes(context.Background(), "TST-0001", "ja")

			require.ErrorIs(t, err, errBoom)
		})
	})
}

func TestGetScript(t *testing.T) {
	t.Run("エピソードスクリプト取得", func(t *testing.T) {
		t.Run("指定エピソードが存在しないとき、エピソード未検出を表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findErr: port.ErrNotFound}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, ErrEpisodeNotFound)
		})

		t.Run("指定エピソードが非アクティブのとき、エピソード未検出を表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: false}}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, ErrEpisodeNotFound)
		})

		t.Run("エピソード取得が未検出以外のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("find episode boom")
			repo := &fakeStoryRepo{findErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("エピソードがロック状態のとき、ロック済みを表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: true, RequiredLevel: 10}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 1}}
			svc := New(repo, &fakeScriptStore{}, account)

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, ErrEpisodeLocked)
		})

		t.Run("エピソードがアンロック済みのとき、{lang}のパス置換が効いた要求言語のスクリプト本文が返る", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{
				EpisodeID:     "TST-EP01",
				IsActive:      true,
				RequiredLevel: 0,
				ScriptPath:    "scripts/story/TST-EP01.{lang}.ks",
			}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 0}}
			scriptStore := &fakeScriptStore{body: "dummy episode script body"}
			svc := New(repo, scriptStore, account)

			body, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "en")

			require.NoError(t, err)
			assert.Equal(t, "dummy episode script body", body)
			require.Len(t, scriptStore.calledKeys, 1)
			assert.Equal(t, "scripts/story/TST-EP01.en.ks", scriptStore.calledKeys[0])
		})

		t.Run("アンロック判定のためのプレイヤー到達状況取得が失敗するとき、エラーになる", func(t *testing.T) {
			errBoom := errors.New("get player progress boom")
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: true}}
			account := &fakePlayerProgressReader{err: errBoom}
			svc := New(repo, &fakeScriptStore{}, account)

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("スクリプト読み込みが失敗するとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("read script boom")
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: true, RequiredLevel: 0}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 0}}
			scriptStore := &fakeScriptStore{err: errBoom}
			svc := New(repo, scriptStore, account)

			_, err := svc.GetScript(context.Background(), "TST-0001", "TST-EP01", "ja")

			require.ErrorIs(t, err, errBoom)
		})
	})
}

func TestCompleteEpisode(t *testing.T) {
	t.Run("エピソード完了記録", func(t *testing.T) {
		t.Run("指定エピソードが存在しないとき、エピソード未検出を表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findErr: port.ErrNotFound}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.ErrorIs(t, err, ErrEpisodeNotFound)
		})

		t.Run("指定エピソードが非アクティブのとき、エピソード未検出を表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: false}}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.ErrorIs(t, err, ErrEpisodeNotFound)
		})

		t.Run("エピソード取得が未検出以外のエラーを返すとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("find episode boom")
			repo := &fakeStoryRepo{findErr: errBoom}
			svc := New(repo, &fakeScriptStore{}, &fakePlayerProgressReader{})

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("エピソードがロック状態のとき、ロック済みを表すエラーになる", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: true, RequiredLevel: 10}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 1}}
			svc := New(repo, &fakeScriptStore{}, account)

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.ErrorIs(t, err, ErrEpisodeLocked)
		})

		t.Run("エピソードがアンロック済みのとき、完了が記録される", func(t *testing.T) {
			repo := &fakeStoryRepo{findEpisode: &domain.Episode{EpisodeID: "TST-EP01", IsActive: true, RequiredLevel: 0}}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 0}}
			svc := New(repo, &fakeScriptStore{}, account)

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.NoError(t, err)
			require.Len(t, repo.markCompleteCalls, 1)
			assert.Equal(t, storyMarkCompleteCall{playerID: "TST-0001", episodeID: "TST-EP01"}, repo.markCompleteCalls[0])
		})

		t.Run("完了記録が失敗するとき、そのエラー内容を含んだエラーになる", func(t *testing.T) {
			errBoom := errors.New("mark complete boom")
			repo := &fakeStoryRepo{
				findEpisode:     &domain.Episode{EpisodeID: "TST-EP01", IsActive: true, RequiredLevel: 0},
				markCompleteErr: errBoom,
			}
			account := &fakePlayerProgressReader{progress: port.PlayerProgress{Level: 0}}
			svc := New(repo, &fakeScriptStore{}, account)

			err := svc.CompleteEpisode(context.Background(), "TST-0001", "TST-EP01")

			require.ErrorIs(t, err, errBoom)
		})
	})
}
