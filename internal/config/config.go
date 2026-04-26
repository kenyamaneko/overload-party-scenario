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
	// GCS バケット名（本番）または "local:" プレフィクス付きローカルパス（開発）。
	// 必須 — scenario がサイレントに no-op 状態で起動することを防ぐ。
	StoryBucket string

	// player-onboarded topic をホストする Google Cloud project。必須 — オンボーディング
	// 完了 hand-off を静かにドロップせず fail-fast する。
	PubsubProjectID string

	// player-onboarded topic 名 (scenario → account / card / gateway …)。
	// デフォルト "player-onboarded"。クロスプロジェクトテスト用にのみ変更する。
	// ADR-022 により faction-selected は廃止し、faction 情報も本イベントに同居する。
	PlayerOnboardedTopic string

	// FirestoreProjectID は game_config の読み取り先プロジェクト ID。
	// ローカル/CI では FIRESTORE_EMULATOR_HOST を別途設定することでエミュレーターに接続。
	FirestoreProjectID string

	// Outbox worker 設定。scenario.outbox_events を消費する常駐 worker のチューニング値。
	// ハードコードではなく env で持つのは、負荷試験やインシデント時にデプロイなしで
	// 試行錯誤できるようにするため (CLAUDE.md「デフォルト値へのフォールバック禁止」に
	// 従い、全値必須で起動時 fail-fast する)。
	OutboxPollInterval      time.Duration // 例: 1s
	OutboxBatchSize         int           // 1 tick で claim する最大行数
	OutboxFailureThreshold  int           // この回数以上の連続失敗で ERROR ログ (死蔵検知)
	OutboxVisibilityTimeout time.Duration // claim 後この期間は他 worker が同じ行を拾わない
}

// FromEnv は環境変数から Config を構築する。
func FromEnv() (*Config, error) {
	cfg := &Config{
		Port:                 9007,
		Env:                  getEnv("ENV", "dev"),
		DatabaseConn:         os.Getenv("DATABASE_CONN"),
		StoryBucket:          os.Getenv("STORY_BUCKET"),
		PubsubProjectID:      os.Getenv("PUBSUB_PROJECT_ID"),
		PlayerOnboardedTopic: getEnv("PLAYER_ONBOARDED_TOPIC", "player-onboarded"),
		FirestoreProjectID:   os.Getenv("FIRESTORE_PROJECT_ID"),
	}

	if raw := os.Getenv("PORT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config: PORT %q: %w", raw, err)
		}
		cfg.Port = n
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
	if cfg.PubsubProjectID == "" {
		return nil, fmt.Errorf("config: PUBSUB_PROJECT_ID is required (scenario publishes player-onboarded events to Pub/Sub)")
	}
	if cfg.FirestoreProjectID == "" {
		return nil, fmt.Errorf("config: FIRESTORE_PROJECT_ID is required (game_config)")
	}

	if err := loadOutboxConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadOutboxConfig は outbox worker のチューニング値を env から読む。
// 全値必須で、パース不能・非正値は fail-fast する (CLAUDE.md「デフォルト値への
// フォールバック禁止」方針。shop と同方針)。
func loadOutboxConfig(cfg *Config) error {
	raw := os.Getenv("OUTBOX_POLL_INTERVAL")
	if raw == "" {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL is required")
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL %q: %w", raw, err)
	}
	if d <= 0 {
		return fmt.Errorf("config: OUTBOX_POLL_INTERVAL must be positive, got %q", raw)
	}
	cfg.OutboxPollInterval = d

	rawBatch := os.Getenv("OUTBOX_BATCH_SIZE")
	if rawBatch == "" {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE is required")
	}
	n, err := strconv.Atoi(rawBatch)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE %q: %w", rawBatch, err)
	}
	if n <= 0 {
		return fmt.Errorf("config: OUTBOX_BATCH_SIZE must be positive, got %q", rawBatch)
	}
	cfg.OutboxBatchSize = n

	rawThreshold := os.Getenv("OUTBOX_FAILURE_THRESHOLD")
	if rawThreshold == "" {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD is required")
	}
	t, err := strconv.Atoi(rawThreshold)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD %q: %w", rawThreshold, err)
	}
	if t <= 0 {
		return fmt.Errorf("config: OUTBOX_FAILURE_THRESHOLD must be positive, got %q", rawThreshold)
	}
	cfg.OutboxFailureThreshold = t

	rawVis := os.Getenv("OUTBOX_VISIBILITY_TIMEOUT")
	if rawVis == "" {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT is required")
	}
	v, err := time.ParseDuration(rawVis)
	if err != nil {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT %q: %w", rawVis, err)
	}
	if v < time.Millisecond {
		return fmt.Errorf("config: OUTBOX_VISIBILITY_TIMEOUT must be >= 1ms, got %q", rawVis)
	}
	cfg.OutboxVisibilityTimeout = v
	return nil
}

// IsLocalStory は StoryBucket が GCS バケットではなくローカルファイルシステム
// （"local:<path>" 形式）を指しているかを返す。
func (c *Config) IsLocalStory() bool {
	return strings.HasPrefix(c.StoryBucket, localStoryPrefix)
}

// StoryLocalPath は IsLocalStory() が true のときファイルシステムパスを返す。
// それ以外は空文字列を返す。
func (c *Config) StoryLocalPath() string {
	if !c.IsLocalStory() {
		return ""
	}
	return strings.TrimPrefix(c.StoryBucket, localStoryPrefix)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
