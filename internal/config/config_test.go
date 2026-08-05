package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPublicKeyPEM は config が値をそのまま保持することの確認にだけ使うダミー。
// 鍵としての妥当性は検証しないため、PEM の体裁だけ揃えている。
const testPublicKeyPEM = "-----BEGIN PUBLIC KEY-----\ndummy-not-a-real-key\n-----END PUBLIC KEY-----\n"

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
	"INTERNAL_AUTH_PUBLIC_KEY",
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
	"INTERNAL_AUTH_PUBLIC_KEY":     testPublicKeyPEM,
	"DATABASE_IAM_AUTH_ENABLED":    "false",
	"OUTBOX_POLL_INTERVAL":         "1s",
	"OUTBOX_BATCH_SIZE":            "100",
	"OUTBOX_FAILURE_THRESHOLD":     "5",
	"OUTBOX_VISIBILITY_TIMEOUT":    "30s",
}

func TestFromEnv(t *testing.T) {
	t.Run("環境変数からのConfig構築", func(t *testing.T) {
		t.Run("必須envが揃うとき、全フィールドがConfigに伝搬する", func(t *testing.T) {
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
			assert.Equal(t, testPublicKeyPEM, cfg.InternalAuthPublicKey)
			assert.Equal(t, time.Second, cfg.OutboxPollInterval)
			assert.Equal(t, 100, cfg.OutboxBatchSize)
			assert.Equal(t, 5, cfg.OutboxFailureThreshold)
			assert.Equal(t, 30*time.Second, cfg.OutboxVisibilityTimeout)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLEDがfalseのとき、CLOUDSQL_CONNECTION_NAMEが未設定でも成功する", func(t *testing.T) {
			setEnv(t, validEnv)

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.False(t, cfg.DatabaseIAMAuthEnabled)
			assert.Empty(t, cfg.CloudSQLConnectionName)
		})

		t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueかつCLOUDSQL_CONNECTION_NAMEが指定されるとき、両方の値がConfigに反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{
				"DATABASE_IAM_AUTH_ENABLED": "true",
				"CLOUDSQL_CONNECTION_NAME":  "overload-party-dev:asia-northeast1:overload-party-db",
			}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.True(t, cfg.DatabaseIAMAuthEnabled)
			assert.Equal(t, "overload-party-dev:asia-northeast1:overload-party-db", cfg.CloudSQLConnectionName)
		})

		t.Run(`STORY_BUCKETが "local:/tmp/story" のとき、ローカルパス /tmp/storyとして読める`, func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "local:/tmp/story"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.True(t, cfg.IsLocalStory())
			assert.Equal(t, "/tmp/story", cfg.StoryLocalPath())
		})

		t.Run(`STORY_BUCKETが "scenario-story" のとき、ローカルパスにならない`, func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "scenario-story"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.False(t, cfg.IsLocalStory())
			assert.Empty(t, cfg.StoryLocalPath())
		})

		t.Run("OUTBOX_POLL_INTERVALが1nsのとき、その値がConfigに反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "1ns"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, time.Nanosecond, cfg.OutboxPollInterval)
		})

		t.Run("OUTBOX_BATCH_SIZEが1のとき、その値がConfigに反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxBatchSize)
		})

		t.Run("OUTBOX_FAILURE_THRESHOLDが1のとき、その値がConfigに反映される", func(t *testing.T) {
			setEnv(t, mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "1"}))

			cfg, err := FromEnv()

			require.NoError(t, err)
			assert.Equal(t, 1, cfg.OutboxFailureThreshold)
		})

		t.Run("OUTBOX_VISIBILITY_TIMEOUTが1msのとき、その値がConfigに反映される", func(t *testing.T) {
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
				name:    "PORTが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PORT": ""}),
				wantErr: "PORT is required",
			},
			{
				name:    "PORTが数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PORT": "not-a-number"}),
				wantErr: "PORT",
			},
			{
				name:    "ENVが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ENV": ""}),
				wantErr: "ENV is required",
			},
			{
				name:    "DATABASE_CONNが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_CONN": ""}),
				wantErr: "DATABASE_CONN is required",
			},
			{
				name:    "STORY_BUCKETが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"STORY_BUCKET": ""}),
				wantErr: "STORY_BUCKET is required",
			},
			{
				name:    `STORY_BUCKETが "local:" のみでパス部分が空文字のとき、エラーになる`,
				envs:    mergeEnv(validEnv, map[string]string{"STORY_BUCKET": "local:"}),
				wantErr: "local path must not be empty",
			},
			{
				name:    "GOOGLE_CLOUD_PROJECT_IDが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"GOOGLE_CLOUD_PROJECT_ID": ""}),
				wantErr: "GOOGLE_CLOUD_PROJECT_ID is required",
			},
			{
				name:    "ONBOARDING_NAME_SET_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ONBOARDING_NAME_SET_TOPIC": ""}),
				wantErr: "ONBOARDING_NAME_SET_TOPIC is required",
			},
			{
				name:    "ONBOARDING_FACTION_SET_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ONBOARDING_FACTION_SET_TOPIC": ""}),
				wantErr: "ONBOARDING_FACTION_SET_TOPIC is required",
			},
			{
				name:    "PLAYER_ONBOARDED_TOPICが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"PLAYER_ONBOARDED_TOPIC": ""}),
				wantErr: "PLAYER_ONBOARDED_TOPIC is required",
			},
			{
				name:    "ACCOUNT_BASE_URLが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"ACCOUNT_BASE_URL": ""}),
				wantErr: "ACCOUNT_BASE_URL is required",
			},
			{
				name:    "INTERNAL_AUTH_PUBLIC_KEYが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"INTERNAL_AUTH_PUBLIC_KEY": ""}),
				wantErr: "INTERNAL_AUTH_PUBLIC_KEY is required",
			},
			{
				name:    "DATABASE_IAM_AUTH_ENABLEDが未設定のとき、変数名を含むエラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": ""}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name:    `DATABASE_IAM_AUTH_ENABLEDが "true"/"false" 以外の "yes" のとき、変数名を含むエラーになる`,
				envs:    mergeEnv(validEnv, map[string]string{"DATABASE_IAM_AUTH_ENABLED": "yes"}),
				wantErr: "DATABASE_IAM_AUTH_ENABLED must be",
			},
			{
				name: "DATABASE_IAM_AUTH_ENABLEDがtrueかつCLOUDSQL_CONNECTION_NAMEが未設定のとき、CLOUDSQL_CONNECTION_NAMEを含むエラーになる",
				envs: mergeEnv(validEnv, map[string]string{
					"DATABASE_IAM_AUTH_ENABLED": "true",
					"CLOUDSQL_CONNECTION_NAME":  "",
				}),
				wantErr: "CLOUDSQL_CONNECTION_NAME is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVALが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": ""}),
				wantErr: "OUTBOX_POLL_INTERVAL is required",
			},
			{
				name:    "OUTBOX_POLL_INTERVALがduration形式でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "not-a-duration"}),
				wantErr: "OUTBOX_POLL_INTERVAL",
			},
			{
				name:    "OUTBOX_POLL_INTERVALが0のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_POLL_INTERVAL": "0s"}),
				wantErr: "OUTBOX_POLL_INTERVAL must be positive",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": ""}),
				wantErr: "OUTBOX_BATCH_SIZE is required",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "not-a-number"}),
				wantErr: "OUTBOX_BATCH_SIZE",
			},
			{
				name:    "OUTBOX_BATCH_SIZEが0のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_BATCH_SIZE": "0"}),
				wantErr: "OUTBOX_BATCH_SIZE must be positive",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": ""}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD is required",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが数値でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "not-a-number"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD",
			},
			{
				name:    "OUTBOX_FAILURE_THRESHOLDが0のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_FAILURE_THRESHOLD": "0"}),
				wantErr: "OUTBOX_FAILURE_THRESHOLD must be positive",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTが未設定のとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": ""}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT is required",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTがduration形式でないとき、エラーになる",
				envs:    mergeEnv(validEnv, map[string]string{"OUTBOX_VISIBILITY_TIMEOUT": "not-a-duration"}),
				wantErr: "OUTBOX_VISIBILITY_TIMEOUT",
			},
			{
				name:    "OUTBOX_VISIBILITY_TIMEOUTが1ms未満のとき、エラーになる",
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
