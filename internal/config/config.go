package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const localStoryPrefix = "local:"

// Config は scenario サービスの起動設定を保持する。
type Config struct {
	Port         int
	Env          string
	DatabaseConn string
	// StoryBucket は GCS バケット名 (本番)、または "local:" プレフィクス付きローカルパス (開発)。
	StoryBucket string

	// GoogleCloudProjectID は scenario が利用する Google Cloud project (Pub/Sub publish 先 + Firestore game_config 読み取り先)。
	GoogleCloudProjectID string

	// OnboardingNameSetTopic は scenario が publish する name 入力ステップ完了イベントの topic 名。
	OnboardingNameSetTopic string
	// OnboardingFactionSetTopic は scenario が publish する faction 選択ステップ完了イベントの topic 名。
	OnboardingFactionSetTopic string
	// PlayerOnboardedTopic は scenario が publish する読了イベントの topic 名。
	PlayerOnboardedTopic string

	// AccountBaseURL はオンボーディング内 name 入力ステップと完了 publish 用に account の internal REST を叩く際のベース URL。
	AccountBaseURL string

	// InternalAuthPublicKey は gateway → scenario の RS256 JWT (X-Internal-Auth) を検証する公開鍵。PEM 形式。
	InternalAuthPublicKey string

	// Outbox worker 設定 (scenario.outbox_events を消費する常駐 worker のチューニング値)。
	OutboxPollInterval      time.Duration
	OutboxBatchSize         int
	OutboxFailureThreshold  int
	OutboxVisibilityTimeout time.Duration

	// DatabaseIAMAuthEnabled は Cloud SQL への接続方式を切り替える。
	DatabaseIAMAuthEnabled bool
	// CloudSQLConnectionName は Cloud SQL インスタンスの接続名 (project:region:instance)。
	CloudSQLConnectionName string
}

// FromEnv は環境変数から Config を構築する。
// 未設定の必須環境変数があれば即エラーで返し、デフォルトへの暗黙 fallback は行わない。
func FromEnv() (*Config, error) {
	cfg := &Config{
		Env:                       os.Getenv("ENV"),
		DatabaseConn:              os.Getenv("DATABASE_CONN"),
		StoryBucket:               os.Getenv("STORY_BUCKET"),
		GoogleCloudProjectID:      os.Getenv("GOOGLE_CLOUD_PROJECT_ID"),
		OnboardingNameSetTopic:    os.Getenv("ONBOARDING_NAME_SET_TOPIC"),
		OnboardingFactionSetTopic: os.Getenv("ONBOARDING_FACTION_SET_TOPIC"),
		PlayerOnboardedTopic:      os.Getenv("PLAYER_ONBOARDED_TOPIC"),
		AccountBaseURL:            os.Getenv("ACCOUNT_BASE_URL"),
		InternalAuthPublicKey:     os.Getenv("INTERNAL_AUTH_PUBLIC_KEY"),
	}

	rawPort := os.Getenv("PORT")
	if rawPort == "" {
		return nil, fmt.Errorf("config: PORT is required")
	}
	n, err := strconv.Atoi(rawPort)
	if err != nil {
		return nil, fmt.Errorf("config: PORT %q: %w", rawPort, err)
	}
	cfg.Port = n

	if cfg.Env == "" {
		return nil, fmt.Errorf("config: ENV is required")
	}
	if cfg.DatabaseConn == "" {
		return nil, fmt.Errorf("config: DATABASE_CONN is required")
	}
	if cfg.StoryBucket == "" {
		return nil, fmt.Errorf("config: STORY_BUCKET is required (GCS bucket name, or \"local:<path>\" for dev)")
	}
	if cfg.IsLocalStory() && cfg.StoryLocalPath() == "" {
		return nil, fmt.Errorf("config: STORY_BUCKET=%q: local path must not be empty after %q", cfg.StoryBucket, localStoryPrefix)
	}
	if cfg.GoogleCloudProjectID == "" {
		return nil, fmt.Errorf("config: GOOGLE_CLOUD_PROJECT_ID is required (scenario は Pub/Sub publish + Firestore game_config で必要)")
	}
	if cfg.OnboardingNameSetTopic == "" {
		return nil, fmt.Errorf("config: ONBOARDING_NAME_SET_TOPIC is required")
	}
	if cfg.OnboardingFactionSetTopic == "" {
		return nil, fmt.Errorf("config: ONBOARDING_FACTION_SET_TOPIC is required")
	}
	if cfg.PlayerOnboardedTopic == "" {
		return nil, fmt.Errorf("config: PLAYER_ONBOARDED_TOPIC is required")
	}
	if cfg.AccountBaseURL == "" {
		return nil, fmt.Errorf("config: ACCOUNT_BASE_URL is required (onboarding name relay and resume judgement)")
	}
	if cfg.InternalAuthPublicKey == "" {
		return nil, fmt.Errorf("config: INTERNAL_AUTH_PUBLIC_KEY is required (gateway → scenario JWT 検証鍵)")
	}

	rawIAMAuth := os.Getenv("DATABASE_IAM_AUTH_ENABLED")
	switch rawIAMAuth {
	case "true":
		cfg.DatabaseIAMAuthEnabled = true
	case "false":
		cfg.DatabaseIAMAuthEnabled = false
	default:
		return nil, fmt.Errorf("config: DATABASE_IAM_AUTH_ENABLED must be %q or %q, got %q", "true", "false", rawIAMAuth)
	}
	if cfg.DatabaseIAMAuthEnabled {
		cfg.CloudSQLConnectionName = os.Getenv("CLOUDSQL_CONNECTION_NAME")
		if cfg.CloudSQLConnectionName == "" {
			return nil, fmt.Errorf("config: CLOUDSQL_CONNECTION_NAME is required when DATABASE_IAM_AUTH_ENABLED is true")
		}
	}

	rawPoll := os.Getenv("OUTBOX_POLL_INTERVAL")
	if rawPoll == "" {
		return nil, fmt.Errorf("config: OUTBOX_POLL_INTERVAL is required")
	}
	pollInterval, err := time.ParseDuration(rawPoll)
	if err != nil {
		return nil, fmt.Errorf("config: OUTBOX_POLL_INTERVAL %q: %w", rawPoll, err)
	}
	if pollInterval <= 0 {
		return nil, fmt.Errorf("config: OUTBOX_POLL_INTERVAL must be positive, got %q", rawPoll)
	}
	cfg.OutboxPollInterval = pollInterval

	rawBatch := os.Getenv("OUTBOX_BATCH_SIZE")
	if rawBatch == "" {
		return nil, fmt.Errorf("config: OUTBOX_BATCH_SIZE is required")
	}
	batch, err := strconv.Atoi(rawBatch)
	if err != nil {
		return nil, fmt.Errorf("config: OUTBOX_BATCH_SIZE %q: %w", rawBatch, err)
	}
	if batch <= 0 {
		return nil, fmt.Errorf("config: OUTBOX_BATCH_SIZE must be positive, got %q", rawBatch)
	}
	cfg.OutboxBatchSize = batch

	rawThreshold := os.Getenv("OUTBOX_FAILURE_THRESHOLD")
	if rawThreshold == "" {
		return nil, fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD is required")
	}
	threshold, err := strconv.Atoi(rawThreshold)
	if err != nil {
		return nil, fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD %q: %w", rawThreshold, err)
	}
	if threshold <= 0 {
		return nil, fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD must be positive, got %q", rawThreshold)
	}
	cfg.OutboxFailureThreshold = threshold

	rawVis := os.Getenv("OUTBOX_VISIBILITY_TIMEOUT")
	if rawVis == "" {
		return nil, fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT is required")
	}
	visibility, err := time.ParseDuration(rawVis)
	if err != nil {
		return nil, fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT %q: %w", rawVis, err)
	}
	if visibility < time.Millisecond {
		return nil, fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms, got %q", rawVis)
	}
	cfg.OutboxVisibilityTimeout = visibility

	return cfg, nil
}

// IsLocalStory は StoryBucket が "local:<path>" 形式を指しているかを返す。
func (c *Config) IsLocalStory() bool {
	return strings.HasPrefix(c.StoryBucket, localStoryPrefix)
}

// StoryLocalPath は IsLocalStory() が true のときファイルシステムパスを返す。
func (c *Config) StoryLocalPath() string {
	if !c.IsLocalStory() {
		return ""
	}
	return strings.TrimPrefix(c.StoryBucket, localStoryPrefix)
}
