package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
)

func TestEpisodeLockReasons(t *testing.T) {
	t.Run("[シナリオ]エピソードのロック理由判定", func(t *testing.T) {
		cases := []struct {
			name    string
			episode domain.Episode
			context domain.UnlockContext
			want    []domain.LockReason
		}{
			{
				name:    "プレイヤーレベルが必要レベル未満のとき、ロック理由に必要レベル種別の理由が含まれ、その理由には必要レベルと現在レベルの値が入る",
				episode: domain.Episode{RequiredLevel: 5},
				context: domain.UnlockContext{PlayerLevel: 3},
				want: []domain.LockReason{
					{Type: domain.LockReasonLevel, RequiredLevel: 5, CurrentLevel: 3},
				},
			},
			{
				name:    "プレイヤーレベルが必要レベルと等しいとき、必要レベル種別の理由は含まれない",
				episode: domain.Episode{RequiredLevel: 5},
				context: domain.UnlockContext{PlayerLevel: 5},
				want:    nil,
			},
			{
				name:    "プレイヤーレベルが必要レベルを上回るとき、必要レベル種別の理由は含まれない",
				episode: domain.Episode{RequiredLevel: 5},
				context: domain.UnlockContext{PlayerLevel: 10},
				want:    nil,
			},
			{
				name:    "必要陣営のうち未所持の陣営が1つあるとき、その陣営を指す陣営種別の理由が1件含まれる",
				episode: domain.Episode{RequiredFactions: []string{"faction-a"}},
				context: domain.UnlockContext{OwnedFactions: map[string]bool{}},
				want: []domain.LockReason{
					{Type: domain.LockReasonFaction, RequiredFaction: "faction-a"},
				},
			},
			{
				name:    "必要陣営が複数あり、いずれも未所持のとき、陣営種別の理由が未所持の数だけ含まれる",
				episode: domain.Episode{RequiredFactions: []string{"faction-a", "faction-b"}},
				context: domain.UnlockContext{OwnedFactions: map[string]bool{}},
				want: []domain.LockReason{
					{Type: domain.LockReasonFaction, RequiredFaction: "faction-a"},
					{Type: domain.LockReasonFaction, RequiredFaction: "faction-b"},
				},
			},
			{
				name:    "必要陣営のうち一部だけ未所持のとき、未所持分だけ陣営種別の理由が含まれる",
				episode: domain.Episode{RequiredFactions: []string{"faction-a", "faction-b"}},
				context: domain.UnlockContext{OwnedFactions: map[string]bool{"faction-a": true}},
				want: []domain.LockReason{
					{Type: domain.LockReasonFaction, RequiredFaction: "faction-b"},
				},
			},
			{
				name:    "必要陣営を全て所持しているとき、陣営種別の理由は含まれない",
				episode: domain.Episode{RequiredFactions: []string{"faction-a", "faction-b"}},
				context: domain.UnlockContext{OwnedFactions: map[string]bool{"faction-a": true, "faction-b": true}},
				want:    nil,
			},
			{
				name:    "必要な前提エピソードのうち未クリアのものが1つあるとき、そのエピソードを指すエピソード種別の理由が1件含まれる",
				episode: domain.Episode{RequiredEpisodes: []string{"TST-0001"}},
				context: domain.UnlockContext{CompletedEpisodes: map[string]bool{}},
				want: []domain.LockReason{
					{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0001"},
				},
			},
			{
				name:    "必要な前提エピソードが複数あり、いずれも未クリアのとき、エピソード種別の理由が未クリアの数だけ含まれる",
				episode: domain.Episode{RequiredEpisodes: []string{"TST-0001", "TST-0002"}},
				context: domain.UnlockContext{CompletedEpisodes: map[string]bool{}},
				want: []domain.LockReason{
					{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0001"},
					{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0002"},
				},
			},
			{
				name:    "必要な前提エピソードのうち一部だけ未クリアのとき、未クリア分だけエピソード種別の理由が含まれる",
				episode: domain.Episode{RequiredEpisodes: []string{"TST-0001", "TST-0002"}},
				context: domain.UnlockContext{CompletedEpisodes: map[string]bool{"TST-0001": true}},
				want: []domain.LockReason{
					{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0002"},
				},
			},
			{
				name:    "必要な前提エピソードを全てクリア済みのとき、エピソード種別の理由は含まれない",
				episode: domain.Episode{RequiredEpisodes: []string{"TST-0001", "TST-0002"}},
				context: domain.UnlockContext{CompletedEpisodes: map[string]bool{"TST-0001": true, "TST-0002": true}},
				want:    nil,
			},
			{
				name: "レベル・陣営・前提エピソードの全てを満たしているとき、ロック理由は空になる",
				episode: domain.Episode{
					RequiredLevel:    5,
					RequiredFactions: []string{"faction-a"},
					RequiredEpisodes: []string{"TST-0001"},
				},
				context: domain.UnlockContext{
					PlayerLevel:       5,
					OwnedFactions:     map[string]bool{"faction-a": true},
					CompletedEpisodes: map[string]bool{"TST-0001": true},
				},
				want: nil,
			},
			{
				name: "レベル・陣営・前提エピソードのうち複数が同時に未充足のとき、それぞれの種別の理由が全て含まれる",
				episode: domain.Episode{
					RequiredLevel:    5,
					RequiredFactions: []string{"faction-a"},
					RequiredEpisodes: []string{"TST-0001"},
				},
				context: domain.UnlockContext{
					PlayerLevel:       3,
					OwnedFactions:     map[string]bool{},
					CompletedEpisodes: map[string]bool{},
				},
				want: []domain.LockReason{
					{Type: domain.LockReasonLevel, RequiredLevel: 5, CurrentLevel: 3},
					{Type: domain.LockReasonFaction, RequiredFaction: "faction-a"},
					{Type: domain.LockReasonEpisode, RequiredEpisode: "TST-0001"},
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ep := tc.episode
				uc := tc.context
				got := ep.LockReasons(&uc)
				require.ElementsMatch(t, tc.want, got)
			})
		}
	})
}

func TestEpisodeTitle(t *testing.T) {
	t.Run("[シナリオ]表示タイトルの言語切り替え", func(t *testing.T) {
		ep := domain.Episode{TitleJa: "日本語タイトル", TitleEn: "English Title"}

		cases := []struct {
			name string
			lang string
			want string
		}{
			{name: `要求言語が "en" のとき、英語タイトルが返る`, lang: "en", want: "English Title"},
			{name: "要求言語が日本語コードのとき、日本語タイトルが返る", lang: "ja", want: "日本語タイトル"},
			{name: "要求言語が未知の値のとき、日本語タイトルが返る", lang: "xx", want: "日本語タイトル"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.want, ep.Title(tc.lang))
			})
		}
	})
}
