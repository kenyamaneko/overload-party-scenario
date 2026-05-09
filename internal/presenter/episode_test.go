package presenter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
	apiscenario "github.com/kenyamaneko/overload-party-scenario/packages/api-scenario"
)

func TestToLockReason(t *testing.T) {
	requiredLv5 := int64(5)
	currentLv2 := int64(2)
	factionSHE := "SHE"
	episodeSheEp1 := "she_ep1"

	tests := []struct {
		name string
		in   domain.LockReason
		want apiscenario.LockReason
	}{
		{
			name: "level は required_level と current_level のみ埋める",
			in:   domain.LockReason{Type: domain.LockReasonLevel, RequiredLevel: 5, CurrentLevel: 2},
			want: apiscenario.LockReason{
				Type:          apiscenario.LockReasonTypeLevel,
				RequiredLevel: &requiredLv5,
				CurrentLevel:  &currentLv2,
			},
		},
		{
			name: "faction は required_faction のみ埋める",
			in:   domain.LockReason{Type: domain.LockReasonFaction, RequiredFaction: "SHE"},
			want: apiscenario.LockReason{
				Type:            apiscenario.LockReasonTypeFaction,
				RequiredFaction: &factionSHE,
			},
		},
		{
			name: "episode は required_episode のみ埋める",
			in:   domain.LockReason{Type: domain.LockReasonEpisode, RequiredEpisode: "she_ep1"},
			want: apiscenario.LockReason{
				Type:            apiscenario.LockReasonTypeEpisode,
				RequiredEpisode: &episodeSheEp1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := presenter.ToLockReason(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildEpisodeWithStatus(t *testing.T) {
	faction := "SHE"
	thumbnail := "thumbnails/she_ep1.png"
	ep := &domain.Episode{
		EpisodeID:        "she_ep1",
		Faction:          &faction,
		EpisodeNumber:    1,
		TitleJa:          "SHE 第1章",
		TitleEn:          "SHE Chapter 1",
		RequiredLevel:    2,
		RequiredFactions: []string{"SHE"},
		ThumbnailPath:    &thumbnail,
	}

	tests := []struct {
		name           string
		uc             *domain.UnlockContext
		lang           string
		wantUnlocked   bool
		wantCompleted  bool
		wantTitle      string
		wantReasonsLen int
	}{
		{
			name: "条件充足 + 未完了は IsUnlocked=true, IsCompleted=false",
			uc: &domain.UnlockContext{
				PlayerLevel:       5,
				OwnedFactions:     map[string]bool{"SHE": true},
				CompletedEpisodes: map[string]bool{},
			},
			lang:           "ja",
			wantUnlocked:   true,
			wantCompleted:  false,
			wantTitle:      "SHE 第1章",
			wantReasonsLen: 0,
		},
		{
			name: "完了済みは IsCompleted=true",
			uc: &domain.UnlockContext{
				PlayerLevel:       5,
				OwnedFactions:     map[string]bool{"SHE": true},
				CompletedEpisodes: map[string]bool{"she_ep1": true},
			},
			lang:           "en",
			wantUnlocked:   true,
			wantCompleted:  true,
			wantTitle:      "SHE Chapter 1",
			wantReasonsLen: 0,
		},
		{
			name: "条件未達は IsUnlocked=false で LockReasons が入る",
			uc: &domain.UnlockContext{
				PlayerLevel:       1,
				OwnedFactions:     map[string]bool{},
				CompletedEpisodes: map[string]bool{},
			},
			lang:           "ja",
			wantUnlocked:   false,
			wantCompleted:  false,
			wantTitle:      "SHE 第1章",
			wantReasonsLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := presenter.BuildEpisodeWithStatus(ep, tc.uc, tc.lang)
			assert.Equal(t, "she_ep1", got.EpisodeID)
			assert.Equal(t, tc.wantTitle, got.Title)
			assert.Equal(t, tc.wantUnlocked, got.IsUnlocked)
			assert.Equal(t, tc.wantCompleted, got.IsCompleted)
			require.Len(t, got.LockReasons, tc.wantReasonsLen)
			require.NotNil(t, got.ThumbnailURL)
			assert.Equal(t, thumbnail, *got.ThumbnailURL)
		})
	}
}
