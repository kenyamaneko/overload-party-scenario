package pubsub

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("[配信]Publisher の構築", func(t *testing.T) {
		cases := []struct {
			name                      string
			projectID                 string
			onboardingNameSetTopic    string
			onboardingFactionSetTopic string
			playerOnboardedTopic      string
			wantErrSubstring          string
		}{
			{
				name:                      "プロジェクトIDが空文字のとき、エラーになる",
				projectID:                 "",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "projectID",
			},
			{
				name:                      "名前入力ステップ完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "topic",
			},
			{
				name:                      "陣営選択ステップ完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "",
				playerOnboardedTopic:      "TST-PLAYER-ONBOARDED",
				wantErrSubstring:          "topic",
			},
			{
				name:                      "プレイヤーオンボード完了イベントのtopicが空文字のとき、エラーになる",
				projectID:                 "tst-project",
				onboardingNameSetTopic:    "TST-ONBOARDING-NAME-SET",
				onboardingFactionSetTopic: "TST-ONBOARDING-FACTION-SET",
				playerOnboardedTopic:      "",
				wantErrSubstring:          "topic",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				p, err := New(context.Background(), tc.projectID, tc.onboardingNameSetTopic, tc.onboardingFactionSetTopic, tc.playerOnboardedTopic)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrSubstring)
				assert.Nil(t, p)
			})
		}
	})
}
