package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allEnvKeys = []string{
	"PORT",
	"ENV",
	"DATABASE_CONN",
	"STORY_BUCKET",
	"GOOGLE_CLOUD_PROJECT_ID",
	"ONBOARDING_NAME_SET_TOPIC",
	"ONBOARDING_FACTION_SET_TOPIC",
	"PLAYER_ONBOARDED_TOPIC",
	"ACCOUNT_BASE_URL",
	"INTERNAL_AUTH_SECRET",
	"DATABASE_IAM_AUTH_ENABLED",
	"CLOUDSQL_CONNECTION_NAME",
	"OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE",
	"OUTBOX_FAILURE_THRESHOLD",
	"OUTBOX_VISIBILITY_TIMEOUT",
}

// setEnv は os.Getenv が "" と unset を区別しない性質を使い、未指定キーに "" を渡して未設定を再現する。
func setEnv(t *testing.T, envs map[string]string) {
	t.Helper()
	for _, k := range allEnvKeys {
		t.Setenv(k, envs[k])
	}
}

func mergeEnv(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var validEnv = map[string]string{
	"PORT":                         "9007",
	"ENV":                          "dev",
	"DATABASE_CONN":                "host=localhost port=5432 dbname=scenario user=scenario password=scenario sslmode=disable",
	"STORY_BUCKET":                 "local:./testdata/stories",
	"GOOGLE_CLOUD_PROJECT_ID":      "scenario-local",
	"ONBOARDING_NAME_SET_TOPIC":    "onboarding-name-set",
	"ONBOARDING_FACTION_SET_TOPIC": "onboarding-faction-set",
	"PLAYER_ONBOARDED_TOPIC":       "player-onboarded",
	"ACCOUNT_BASE_URL":             "http://localhost:9001",
	"INTERNAL_AUTH_SECRET":         "test-internal-auth-secret-do-not-use-in-prod-xxxxx",
	"DATABASE_IAM_AUTH_ENABLED":    "false",
	"OUTBOX_POLL_INTERVAL":         "1s",
	"OUTBOX_BATCH_SIZE":            "100",
	"OUTBOX_FAILURE_THRESHOLD":     "5",
	"OUTBOX_VISIBILITY_TIMEOUT":    "30s",
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からの Config 構築", func(t *testing.T) {
		t.Run("必須 env が揃うとき、全フィールドが Config に伝搬する", func(t *testing.T) {
			setEnv(t, validEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 9007, cfg.Port)
			assert.Equal(t, "dev", cfg.Env)
			assert.Equal(t, "host=localhost port=5432 dbname=scenario user=scenario password=scenario sslmode=disable", cfg.DatabaseConn)
			assert.Equal(t, "local:./testdata/stories", cfg.StoryBucket)
			assert.Equal(t, "scenario-local", cfg.GoogleCloudProjectID)
			assert.Equal(t, "onboarding-name-set", cfg.OnboardingNameSetTopic)
			assert.Equal(t, "onboarding-faction-set", cfg.OnboardingFactionSetTopic)
			assert.Equal(t, "player-onboarded", cfg.PlayerOnboardedTopic)
			assert.Equal(t, "http://localhost:9001", cfg.AccountBaseURL)
			assert.Equal(t, "test-internal-auth-secret-do-not-use-in-prod-xxxxx", cfg.InternalAuthSecret)
			assert.Equal(t, time.Second, cfg.OutboxPollInterval)
			assert.Equal(t, 100, cfg.OutboxBatchSize)
			assert.Equal(t, 5, cfg.OutboxFailureThreshold)
			assert.Equal(t, 30*time.Second, cfg.OutboxVisibilityTimeout)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLED が false のとき、CLOUDSQL_CONNECTION_NAME が未設定でも成功する", func(t *testing.T) {
			setEnv(t, validEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.False(t, cfg.DatabaseIAMAuthEnabled)
			assert.Empty(t, cfg.CloudSQLConnectionName)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLED が true かつ CLOUDSQL_CONNECTION_NAME が指定されるとき、両方の値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{
				"DATABASE_IAM_AUTH_ENABLED": "true",
				"CLOUDSQL_CONNECTION_NAME":  "overload-party-dev:asia-northeast1:overload-party-db",
			}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.True(t, cfg.DatabaseIAMAuthEnabled)
			assert.Equal(t, "overload-party-dev:asia-northeast1:overload-party-db", cfg.CloudSQLConnectionName)
		})

		t.Run(`STORY_BUCKET が "local:/tmp/story" のとき、ローカルパス /tmp/story として読める`, func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "local:/tmp/story"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.True(t, cfg.IsLocalStory())
			assert.Equal(t, "/tmp/story", cfg.StoryLocalPath())
		})

		t.Run(`STORY_BUCKET が "scenario-story" のとき、ローカルパスにならない`, func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "scenario-story"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.False(t, cfg.IsLocalStory())
			assert.Empty(t, cfg.StoryLocalPath())
		})

		t.Run("OUTBOX_POLL_INTERVAL が 1ns のとき、その値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "1ns"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Nanosecond, cfg.OutboxPollInterval)
		})

		t.Run("OUTBOX_BATCH_SIZE が 1 のとき、その値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxBatchSize)
		})

		t.Run("OUTBOX_FAILURE_THRESHOLD が 1 のとき、その値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxFailureThreshold)
		})

		t.Run("OUTBOX_VISIBILITY_TIMEOUT が 1ms のとき、その値が Config に反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "1ms"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Millisecond, cfg.OutboxVisibilityTimeout)
		})

		invalidCases := []struct {
			name    string
			envs    map[string]string
			wantErr string
		}{
			{
				name:    "PORT が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PORT": ""}),
				wantErr: "PORT is required",
			},
			{
				name:    "PORT が数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PORT": "not-a-number"}),
				wantErr: "PORT",
			},
			{
				name:    "ENV が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ENV": ""}),
				wantErr: "ENV is required",
			},
			{
				name:    "DATABASE_CONN が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_CONN": ""}),
				wantErr: "DATABASE_CONN is required",
			},
			{
				name:    "STORY_BUCKET が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"STORY_BUCKET": ""}),
				wantErr: "STORY_BUCKET is required",
			},
			{
				name:    `STORY_BUCKET が "local:" のみでパス部分が空文字のとき、エラーになる`,
				envs:    mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "local:"}),
				wantErr: "local path must not be empty",
			},
			{
				name:    "GOOGLE_CLOUD_PROJECT_ID が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"GOOGLE_CLOUD_PROJECT_ID": ""}),
				wantErr: "GOOGLE_CLOUD_PROJECT_ID is required",
			},
			{
				name:    "ONBOARDING_NAME_SET_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ONBOARDING_NAME_SET_TOPIC": ""}),
				wantErr: "ONBOARDING_NAME_SET_TOPIC is required",
			},
			{
				name:    "ONBOARDING_FACTION_SET_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ONBOARDING_FACTION_SET_TOPIC": ""}),
				wantErr: "ONBOARDING_FACTION_SET_TOPIC is required",
			},
			{
				name:    "PLAYER_ONBOARDED_TOPIC が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PLAYER_ONBOARDED_TOPIC": ""}),
				wantErr: "PLAYER_ONBOARDED_TOPIC is required",
			},
			{
				name:    "ACCOUNT_BASE_URL が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ACCOUNT_BASE_URL": ""}),
				wantErr: "ACCOUNT_BASE_URL is required",
			},
			{
				name:    "INTERNAL_AUTH_SECRET が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"INTERNAL_AUTH_SECRET": ""}),
				wantErr: "INTERNAL_AUTH_SECRET is required",
			},
			{
				name:    "DATABASE_IAM_AUTH_ENABLED が未設定のとき、変数名を含むエラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": ""}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name:    `DATABASE_IAM_AUTH_ENABLED が "true"/"false" 以外の "yes" のとき、変数名を含むエラーになる`,
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": "yes"}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name: "DATABASE_IAM_AUTH_ENABLED が true かつ CLOUDSQL_CONNECTION_NAME が未設定のとき、CLOUDSQL_CONNECTION_NAME を含むエラーになる",
				envs: mergeEnv(validEnv, map[string]string{
					"DATABASE_IAM_AUTH_ENABLED": "true",
					"CLOUDSQL_CONNECTION_NAME":  "",
				}),
				wantErr: "CLOUDSQL_CONNECTION_NAME is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": ""}),
				wantErr: "OUTBOX_POLL_INTERVAL is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が duration 形式でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "not-a-duration"}),
				wantErr: "OUTBOX_POLL_INTERVAL",
			},
			{
				name:    "OUTBOX_POLL_INTERVAL が 0 のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "0s"}),
				wantErr: "OUTBOX_POLL_INTERVAL must be positive",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": ""}),
				wantErr: "OUTBOX_BATCH_SIZE is required",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "not-a-number"}),
				wantErr: "OUTBOX_BATCH_SIZE",
			},
			{
				name:    "OUTBOX_BATCH_SIZE が 0 のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "0"}),
				wantErr: "OUTBOX_BATCH_SIZE must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD is required",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "not-a-number"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLD が 0 のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD must be positive",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT is required",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が duration 形式でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "not-a-duration"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUT が 1ms 未満のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "500us"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms",
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				setEnv(t, tc.envs)

				_, err := FromEnv()

				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			})
		}
	})
}
