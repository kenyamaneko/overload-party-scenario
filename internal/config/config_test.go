package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/config"
)

func newValidEnv() map[string]string {
	return map[string]string{
		"PORT":                         "8080",
		"ENV":                          "production",
		"DATABASE_CONN":                "postgres://localhost:5432/scenario",
		"STORY_BUCKET":                 "scenario-stories-bucket",
		"GOOGLE_CLOUD_PROJECT_ID":      "test-project",
		"ONBOARDING_NAME_SET_TOPIC":    "onboarding-name-set",
		"ONBOARDING_FACTION_SET_TOPIC": "onboarding-faction-set",
		"PLAYER_ONBOARDED_TOPIC":       "player-onboarded",
		"ACCOUNT_BASE_URL":             "https://account.internal.example.com",
		"INTERNAL_AUTH_PUBLIC_KEY":     "test-public-key",
		"DATABASE_IAM_AUTH_ENABLED":    "false",
		"OUTBOX_POLL_INTERVAL":         "5s",
		"OUTBOX_BATCH_SIZE":            "10",
		"OUTBOX_FAILURE_THRESHOLD":     "3",
		"OUTBOX_VISIBILITY_TIMEOUT":    "30s",
	}
}

func setValidEnv(t *testing.T) {
	t.Helper()
	for k, v := range newValidEnv() {
		t.Setenv(k, v)
	}
}

func TestFromEnv(t *testing.T) {
	t.Run("[設定]環境変数からの設定構築", func(t *testing.T) {
		t.Run("必須環境変数の欠落", func(t *testing.T) {
			cases := []struct {
				name            string
				envVar          string
				wantErrContains string
			}{
				{"PORT が未設定のとき、エラーになる", "PORT", "PORT is required"},
				{"ENV が未設定のとき、エラーになる", "ENV", "ENV is required"},
				{"DATABASE_CONN が未設定のとき、エラーになる", "DATABASE_CONN", "DATABASE_CONN is required"},
				{"STORY_BUCKET が未設定のとき、エラーになる", "STORY_BUCKET", "STORY_BUCKET is required"},
				{"GOOGLE_CLOUD_PROJECT_ID が未設定のとき、エラーになる", "GOOGLE_CLOUD_PROJECT_ID", "GOOGLE_CLOUD_PROJECT_ID is required"},
				{"ONBOARDING_NAME_SET_TOPIC が未設定のとき、エラーになる", "ONBOARDING_NAME_SET_TOPIC", "ONBOARDING_NAME_SET_TOPIC is required"},
				{"ONBOARDING_FACTION_SET_TOPIC が未設定のとき、エラーになる", "ONBOARDING_FACTION_SET_TOPIC", "ONBOARDING_FACTION_SET_TOPIC is required"},
				{"PLAYER_ONBOARDED_TOPIC が未設定のとき、エラーになる", "PLAYER_ONBOARDED_TOPIC", "PLAYER_ONBOARDED_TOPIC is required"},
				{"ACCOUNT_BASE_URL が未設定のとき、エラーになる", "ACCOUNT_BASE_URL", "ACCOUNT_BASE_URL is required"},
				{"INTERNAL_AUTH_PUBLIC_KEY が未設定のとき、エラーになる", "INTERNAL_AUTH_PUBLIC_KEY", "INTERNAL_AUTH_PUBLIC_KEY is required"},
				{"DATABASE_IAM_AUTH_ENABLED が未設定のとき、エラーになる", "DATABASE_IAM_AUTH_ENABLED", `DATABASE_IAM_AUTH_ENABLED must be "true" or "false"`},
				{"OUTBOX_POLL_INTERVAL が未設定のとき、エラーになる", "OUTBOX_POLL_INTERVAL", "OUTBOX_POLL_INTERVAL is required"},
				{"OUTBOX_BATCH_SIZE が未設定のとき、エラーになる", "OUTBOX_BATCH_SIZE", "OUTBOX_BATCH_SIZE is required"},
				{"OUTBOX_FAILURE_THRESHOLD が未設定のとき、エラーになる", "OUTBOX_FAILURE_THRESHOLD", "OUTBOX_FAILURE_THRESHOLD is required"},
				{"OUTBOX_VISIBILITY_TIMEOUT が未設定のとき、エラーになる", "OUTBOX_VISIBILITY_TIMEOUT", "OUTBOX_VISIBILITY_TIMEOUT is required"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					setValidEnv(t)
					t.Setenv(tc.envVar, "")

					got, err := config.FromEnv()

					require.ErrorContains(t, err, tc.wantErrContains)
					require.Nil(t, got)
				})
			}
		})

		t.Run("型・書式の検証", func(t *testing.T) {
			cases := []struct {
				name            string
				envVar          string
				value           string
				wantErrContains string
			}{
				{"PORT が数値として解釈できない値のとき、エラーになる", "PORT", "abc", `PORT "abc"`},
				{"OUTBOX_POLL_INTERVAL が期間として解釈できない値のとき、エラーになる", "OUTBOX_POLL_INTERVAL", "abc", `OUTBOX_POLL_INTERVAL "abc"`},
				{"OUTBOX_BATCH_SIZE が数値として解釈できない値のとき、エラーになる", "OUTBOX_BATCH_SIZE", "abc", `OUTBOX_BATCH_SIZE "abc"`},
				{"OUTBOX_FAILURE_THRESHOLD が数値として解釈できない値のとき、エラーになる", "OUTBOX_FAILURE_THRESHOLD", "abc", `OUTBOX_FAILURE_THRESHOLD "abc"`},
				{"OUTBOX_VISIBILITY_TIMEOUT が期間として解釈できない値のとき、エラーになる", "OUTBOX_VISIBILITY_TIMEOUT", "abc", `OUTBOX_VISIBILITY_TIMEOUT "abc"`},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					setValidEnv(t)
					t.Setenv(tc.envVar, tc.value)

					got, err := config.FromEnv()

					require.ErrorContains(t, err, tc.wantErrContains)
					require.Nil(t, got)
				})
			}
		})

		t.Run("境界値", func(t *testing.T) {
			t.Run("下限を下回るとき、エラーになる", func(t *testing.T) {
				cases := []struct {
					name            string
					envVar          string
					value           string
					wantErrContains string
				}{
					{"OUTBOX_POLL_INTERVAL が 0 以下のとき、エラーになる", "OUTBOX_POLL_INTERVAL", "0s", `OUTBOX_POLL_INTERVAL must be positive, got "0s"`},
					{"OUTBOX_BATCH_SIZE が 0 以下のとき、エラーになる", "OUTBOX_BATCH_SIZE", "0", `OUTBOX_BATCH_SIZE must be positive, got "0"`},
					{"OUTBOX_FAILURE_THRESHOLD が 0 以下のとき、エラーになる", "OUTBOX_FAILURE_THRESHOLD", "0", `OUTBOX_FAILURE_THRESHOLD must be positive, got "0"`},
					{"OUTBOX_VISIBILITY_TIMEOUT が 1 ミリ秒未満のとき、エラーになる", "OUTBOX_VISIBILITY_TIMEOUT", "999us", `OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms, got "999us"`},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						setValidEnv(t)
						t.Setenv(tc.envVar, tc.value)

						got, err := config.FromEnv()

						require.ErrorContains(t, err, tc.wantErrContains)
						require.Nil(t, got)
					})
				}
			})

			t.Run("下限以上のとき、構築が成功する", func(t *testing.T) {
				cases := []struct {
					name   string
					envVar string
					value  string
				}{
					{"OUTBOX_POLL_INTERVAL が正の値のとき、構築が成功する", "OUTBOX_POLL_INTERVAL", "1ns"},
					{"OUTBOX_BATCH_SIZE が 1 以上のとき、構築が成功する", "OUTBOX_BATCH_SIZE", "1"},
					{"OUTBOX_FAILURE_THRESHOLD が 1 以上のとき、構築が成功する", "OUTBOX_FAILURE_THRESHOLD", "1"},
					{"OUTBOX_VISIBILITY_TIMEOUT が 1 ミリ秒以上のとき、構築が成功する", "OUTBOX_VISIBILITY_TIMEOUT", "1ms"},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						setValidEnv(t)
						t.Setenv(tc.envVar, tc.value)

						got, err := config.FromEnv()

						require.NoError(t, err)
						require.NotNil(t, got)
					})
				}
			})
		})

		t.Run("STORY_BUCKETの形式分岐", func(t *testing.T) {
			t.Run(`STORY_BUCKET が "local:" で始まり、それに続くパス部分が空文字のとき、エラーになる`, func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("STORY_BUCKET", "local:")

				got, err := config.FromEnv()

				require.ErrorContains(t, err, "must not be empty")
				require.Nil(t, got)
			})

			t.Run(`STORY_BUCKET が "local:" で始まり、それに続くパス部分が非空のとき`, func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("STORY_BUCKET", "local:/tmp/scenario-stories")

				got, err := config.FromEnv()

				t.Run("構築が成功する", func(t *testing.T) {
					require.NoError(t, err)
				})
				t.Run("ローカルストーリー判定は true になる", func(t *testing.T) {
					require.NoError(t, err)
					require.True(t, got.IsLocalStory())
				})
				t.Run("ローカルパスとしてそのパス部分が返る", func(t *testing.T) {
					require.NoError(t, err)
					require.Equal(t, "/tmp/scenario-stories", got.StoryLocalPath())
				})
			})

			t.Run(`STORY_BUCKET が "local:" で始まらないとき`, func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("STORY_BUCKET", "scenario-stories-bucket")

				got, err := config.FromEnv()

				t.Run("構築が成功する", func(t *testing.T) {
					require.NoError(t, err)
				})
				t.Run("ローカルストーリー判定は false になる", func(t *testing.T) {
					require.NoError(t, err)
					require.False(t, got.IsLocalStory())
				})
				t.Run("ローカルパスは空文字になる", func(t *testing.T) {
					require.NoError(t, err)
					require.Equal(t, "", got.StoryLocalPath())
				})
			})
		})

		t.Run("DATABASE_IAM_AUTH_ENABLEDの分岐", func(t *testing.T) {
			cases := []struct {
				name                   string
				iamAuthEnabled         string
				cloudSQLConnectionName string
				wantErrContains        string
			}{
				{`DATABASE_IAM_AUTH_ENABLED が "true" でも "false" でもない値のとき、エラーになる`, "maybe", "", `got "maybe"`},
				{`DATABASE_IAM_AUTH_ENABLED が "true" で、CLOUDSQL_CONNECTION_NAME が未設定のとき、エラーになる`, "true", "", "CLOUDSQL_CONNECTION_NAME is required"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					setValidEnv(t)
					t.Setenv("DATABASE_IAM_AUTH_ENABLED", tc.iamAuthEnabled)
					t.Setenv("CLOUDSQL_CONNECTION_NAME", tc.cloudSQLConnectionName)

					got, err := config.FromEnv()

					require.ErrorContains(t, err, tc.wantErrContains)
					require.Nil(t, got)
				})
			}

			t.Run(`DATABASE_IAM_AUTH_ENABLED が "true" で、CLOUDSQL_CONNECTION_NAME が設定されているとき、構築が成功し、その値が設定に反映される`, func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("DATABASE_IAM_AUTH_ENABLED", "true")
				t.Setenv("CLOUDSQL_CONNECTION_NAME", "test-project:us-central1:test-instance")

				got, err := config.FromEnv()

				require.NoError(t, err)
				require.True(t, got.DatabaseIAMAuthEnabled)
				require.Equal(t, "test-project:us-central1:test-instance", got.CloudSQLConnectionName)
			})

			t.Run(`DATABASE_IAM_AUTH_ENABLED が "false" のとき、CLOUDSQL_CONNECTION_NAME が未設定でも構築が成功する`, func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("DATABASE_IAM_AUTH_ENABLED", "false")
				t.Setenv("CLOUDSQL_CONNECTION_NAME", "")

				got, err := config.FromEnv()

				require.NoError(t, err)
				require.False(t, got.DatabaseIAMAuthEnabled)
			})
		})

		t.Run("全ての必須項目が有効な値で設定されているとき、構築が成功し、各設定値が対応する環境変数の値に一致する", func(t *testing.T) {
			setValidEnv(t)

			got, err := config.FromEnv()

			require.NoError(t, err)
			require.Equal(t, 8080, got.Port)
			require.Equal(t, "production", got.Env)
			require.Equal(t, "postgres://localhost:5432/scenario", got.DatabaseConn)
			require.Equal(t, "scenario-stories-bucket", got.StoryBucket)
			require.Equal(t, "test-project", got.GoogleCloudProjectID)
			require.Equal(t, "onboarding-name-set", got.OnboardingNameSetTopic)
			require.Equal(t, "onboarding-faction-set", got.OnboardingFactionSetTopic)
			require.Equal(t, "player-onboarded", got.PlayerOnboardedTopic)
			require.Equal(t, "https://account.internal.example.com", got.AccountBaseURL)
			require.Equal(t, "test-public-key", got.InternalAuthPublicKey)
			require.False(t, got.DatabaseIAMAuthEnabled)
			require.Equal(t, 5*time.Second, got.OutboxPollInterval)
			require.Equal(t, 10, got.OutboxBatchSize)
			require.Equal(t, 3, got.OutboxFailureThreshold)
			require.Equal(t, 30*time.Second, got.OutboxVisibilityTimeout)
		})
	})
}
