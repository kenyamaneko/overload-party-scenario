package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
)

func TestEpisodeLockReasons(t *testing.T) {
	t.Run("エピソードのロック理由判定", func(t *testing.T) {
		tests := []struct {
			name        string
			ep          *domain.Episode
			uc          *domain.UnlockContext
			wantReasons []domain.LockReasonType
		}{
			{
				name: "全条件を満たすとき、ロック理由は無い",
				ep: &domain.Episode{
					RequiredLevel:    5,
					RequiredFactions: []string{"SHE"},
					RequiredEpisodes: []string{"she_ep1"},
				},
				uc: &domain.UnlockContext{
					PlayerLevel:       10,
					OwnedFactions:     map[string]bool{"SHE": true},
					CompletedEpisodes: map[string]bool{"she_ep1": true},
				},
				wantReasons: nil,
			},
			{
				name: "全条件未達のとき、level・faction・episodeの3理由を返す",
				ep: &domain.Episode{
					RequiredLevel:    5,
					RequiredFactions: []string{"SHE"},
					RequiredEpisodes: []string{"she_ep1"},
				},
				uc: &domain.UnlockContext{
					PlayerLevel:       1,
					OwnedFactions:     map[string]bool{},
					CompletedEpisodes: map[string]bool{},
				},
				wantReasons: []domain.LockReasonType{
					domain.LockReasonLevel,
					domain.LockReasonFaction,
					domain.LockReasonEpisode,
				},
			},
			{
				name: "levelのみ未達のとき、level理由1件を返す",
				ep: &domain.Episode{
					RequiredLevel:    10,
					RequiredFactions: []string{"SHE"},
					RequiredEpisodes: []string{},
				},
				uc: &domain.UnlockContext{
					PlayerLevel:       3,
					OwnedFactions:     map[string]bool{"SHE": true},
					CompletedEpisodes: map[string]bool{},
				},
				wantReasons: []domain.LockReasonType{domain.LockReasonLevel},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				reasons := tc.ep.LockReasons(tc.uc)
				require.Len(t, reasons, len(tc.wantReasons))
				for i, want := range tc.wantReasons {
					assert.Equal(t, want, reasons[i].Type)
				}
			})
		}
	})
}

func TestEpisodeTitle(t *testing.T) {
	t.Run("エピソードのタイトル言語選択", func(t *testing.T) {
		ep := &domain.Episode{TitleJa: "日本語", TitleEn: "English"}

		tests := []struct {
			name string
			lang string
			want string
		}{
			{name: "lang=jaのとき、日本語タイトルを返す", lang: "ja", want: "日本語"},
			{name: "lang=enのとき、英語タイトルを返す", lang: "en", want: "English"},
			{name: "未知の言語のとき、日本語タイトルにフォールバックする", lang: "fr", want: "日本語"},
			{name: "空文字の言語のとき、日本語タイトルにフォールバックする", lang: "", want: "日本語"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, ep.Title(tc.lang))
			})
		}
	})
}
