package presenter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
	"github.com/kenyamaneko/overload-party-scenario/internal/presenter"
)

func TestToLockReason(t *testing.T) {
	tests := []struct {
		name         string
		in           domain.LockReason
		wantType     string
		wantRequired interface{}
		wantCurrent  interface{}
	}{
		{
			name:         "level は required と current を持つ",
			in:           domain.LockReason{Type: domain.LockReasonLevel, RequiredLevel: 5, CurrentLevel: 2},
			wantType:     "level",
			wantRequired: int64(5),
			wantCurrent:  int64(2),
		},
		{
			name:         "faction は required に faction id を入れる",
			in:           domain.LockReason{Type: domain.LockReasonFaction, RequiredFaction: "SHE"},
			wantType:     "faction",
			wantRequired: "SHE",
			wantCurrent:  nil,
		},
		{
			name:         "episode は required に episode id を入れる",
			in:           domain.LockReason{Type: domain.LockReasonEpisode, RequiredEpisode: "she_ep1"},
			wantType:     "episode",
			wantRequired: "she_ep1",
			wantCurrent:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := presenter.ToLockReason(tc.in)
			assert.Equal(t, tc.wantType, got.Type)
			assert.Equal(t, tc.wantRequired, got.Required)
			assert.Equal(t, tc.wantCurrent, got.Current)
		})
	}
}

func TestBuildEpisodeWithStatus(t *testing.T) {
	faction := "SHE"
	ep := &domain.Episode{
		EpisodeID:        "she_ep1",
		Faction:          &faction,
		EpisodeNumber:    1,
		TitleJa:          "SHE 第1章",
		TitleEn:          "SHE Chapter 1",
		RequiredLevel:    2,
		RequiredFactions: []string{"SHE"},
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
			lang:          "en",
			wantUnlocked:  true,
			wantCompleted: true,
			wantTitle:     "SHE Chapter 1",
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
		})
	}
}
