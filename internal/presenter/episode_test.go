package presenter_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

func int64Ptr(v int64) *int64    { return &v }
func stringPtr(v string) *string { return &v }

func TestToLockReason(t *testing.T) {
	t.Run("[シナリオ]ロック理由のwire変換", func(t *testing.T) {
		cases := []struct {
			name  string
			input domain.LockReason
			want  apiscenario.LockReason
		}{
			{
				name: "必要レベル種別の理由のとき、変換後の理由にも必要レベルと現在レベルの値が引き継がれ、必要陣営・必要エピソードのフィールドは埋まらない",
				input: domain.LockReason{
					Type:          domain.LockReasonLevel,
					RequiredLevel: 5,
					CurrentLevel:  3,
				},
				want: apiscenario.LockReason{
					Type:          apiscenario.LockReasonTypeLevel,
					RequiredLevel: int64Ptr(5),
					CurrentLevel:  int64Ptr(3),
				},
			},
			{
				name: "陣営種別の理由のとき、変換後の理由に必要陣営の値が引き継がれ、必要レベル・必要エピソードのフィールドは埋まらない",
				input: domain.LockReason{
					Type:            domain.LockReasonFaction,
					RequiredFaction: "faction-a",
				},
				want: apiscenario.LockReason{
					Type:            apiscenario.LockReasonTypeFaction,
					RequiredFaction: stringPtr("faction-a"),
				},
			},
			{
				name: "エピソード種別の理由のとき、変換後の理由に必要エピソードの値が引き継がれ、必要レベル・必要陣営のフィールドは埋まらない",
				input: domain.LockReason{
					Type:            domain.LockReasonEpisode,
					RequiredEpisode: "TST-0001",
				},
				want: apiscenario.LockReason{
					Type:            apiscenario.LockReasonTypeEpisode,
					RequiredEpisode: stringPtr("TST-0001"),
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.want, presenter.ToLockReason(tc.input))
			})
		}
	})
}

func TestToLockReasons(t *testing.T) {
	t.Run("[シナリオ]複数理由のwire変換", func(t *testing.T) {
		t.Run("理由が0件のとき、変換結果は空配列になり、JSONシリアライズしたときnullでなく[]になる", func(t *testing.T) {
			got := presenter.ToLockReasons(nil)
			require.Len(t, got, 0)

			b, err := json.Marshal(got)
			require.NoError(t, err)
			require.Equal(t, "[]", string(b))
		})

		t.Run("理由が複数件のとき、それぞれの理由が変換され、件数と順序が保たれる", func(t *testing.T) {
			input := []domain.LockReason{
				{Type: domain.LockReasonLevel, RequiredLevel: 5, CurrentLevel: 3},
				{Type: domain.LockReasonFaction, RequiredFaction: "faction-a"},
				{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0001"},
			}
			want := []apiscenario.LockReason{
				{Type: apiscenario.LockReasonTypeLevel, RequiredLevel: int64Ptr(5), CurrentLevel: int64Ptr(3)},
				{Type: apiscenario.LockReasonTypeFaction, RequiredFaction: stringPtr("faction-a")},
				{Type: apiscenario.LockReasonTypeEpisode, RequiredEpisode: stringPtr("TST-0001")},
			}

			got := presenter.ToLockReasons(input)
			require.Equal(t, want, got)
		})
	})
}

func TestBuildEpisodeWithStatus(t *testing.T) {
	t.Run("[シナリオ]エピソードの状態組み立て", func(t *testing.T) {
		t.Run("ロック理由が1件以上あるとき、アンロック済みフラグはfalseになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001", RequiredLevel: 5}
			uc := &domain.UnlockContext{PlayerLevel: 1}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.False(t, got.IsUnlocked)
		})

		t.Run("ロック理由が0件のとき、アンロック済みフラグはtrueになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001", RequiredLevel: 1}
			uc := &domain.UnlockContext{PlayerLevel: 5}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.True(t, got.IsUnlocked)
		})

		t.Run("対象エピソードがプレイヤーの完了済みエピソードに含まれるとき、完了済みフラグはtrueになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001"}
			uc := &domain.UnlockContext{CompletedEpisodes: map[string]bool{"TST-0001": true}}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.True(t, got.IsCompleted)
		})

		t.Run("対象エピソードがプレイヤーの完了済みエピソードに含まれないとき、完了済みフラグはfalseになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001"}
			uc := &domain.UnlockContext{CompletedEpisodes: map[string]bool{"TST-0002": true}}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.False(t, got.IsCompleted)
		})

		t.Run("サムネイルパスが設定されているとき、応答のサムネイルURLにその値が入る", func(t *testing.T) {
			path := "https://example.com/thumbnail.png"
			ep := &domain.Episode{EpisodeID: "TST-0001", ThumbnailPath: &path}
			uc := &domain.UnlockContext{}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.NotNil(t, got.ThumbnailURL)
			require.Equal(t, path, *got.ThumbnailURL)
		})

		t.Run("サムネイルパスが未設定(nil)のとき、応答のサムネイルURLも未設定になる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001", ThumbnailPath: nil}
			uc := &domain.UnlockContext{}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.Nil(t, got.ThumbnailURL)
		})

		t.Run("要求言語がenのとき、応答のタイトルは英語タイトルになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001", TitleJa: "日本語タイトル", TitleEn: "English Title"}
			uc := &domain.UnlockContext{}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "en")

			require.Equal(t, "English Title", got.Title)
		})

		t.Run("要求言語がen以外のとき、応答のタイトルは日本語タイトルになる", func(t *testing.T) {
			ep := &domain.Episode{EpisodeID: "TST-0001", TitleJa: "日本語タイトル", TitleEn: "English Title"}
			uc := &domain.UnlockContext{}

			got := presenter.BuildEpisodeWithStatus(ep, uc, "ja")

			require.Equal(t, "日本語タイトル", got.Title)
		})
	})
}

func TestBuildEpisodesWithStatus(t *testing.T) {
	t.Run("[シナリオ]複数エピソードの一括状態組み立て", func(t *testing.T) {
		t.Run("複数のエピソードを渡すと、それぞれが個別に変換され、件数と順序が保たれる", func(t *testing.T) {
			ep1 := &domain.Episode{EpisodeID: "TST-0001", RequiredLevel: 1}
			ep2 := &domain.Episode{EpisodeID: "TST-0002", RequiredLevel: 10}
			uc := &domain.UnlockContext{PlayerLevel: 5}

			got := presenter.BuildEpisodesWithStatus([]*domain.Episode{ep1, ep2}, uc, "ja")

			require.Len(t, got, 2)
			require.Equal(t, "TST-0001", got[0].EpisodeID)
			require.True(t, got[0].IsUnlocked)
			require.Equal(t, "TST-0002", got[1].EpisodeID)
			require.False(t, got[1].IsUnlocked)
		})
	})
}
