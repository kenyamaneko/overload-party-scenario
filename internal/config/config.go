package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const localStoryPrefix = "local:"

// Config は scenario サービスの起動設定を保持する。
type Config struct {
	Port        int
	Env         string
	DatabaseURL string
	// GCS バケット名（本番）または "local:" プレフィクス付きローカルパス（開発）。
	// 必須 — scenario がサイレントに no-op 状態で起動することを防ぐ。
	StoryBucket string

	// faction-selected topic をホストする Google Cloud project。必須 — 初期 faction 選択の
	// hand-off を静かにドロップせず fail-fast する。
	PubsubProjectID string

	// サービス横断 faction 選択イベントの topic 名。
	// デフォルト "faction-selected"。クロスプロジェクトテスト用にのみ変更する。
	FactionSelectedTopic string

	// FirestoreProjectID は game_config の読み取り先プロジェクト ID。
	// ローカル/CI では FIRESTORE_EMULATOR_HOST を別途設定することでエミュレーターに接続。
	FirestoreProjectID string
}

// FromEnv は環境変数から Config を構築する。
func FromEnv() (*Config, error) {
	cfg := &Config{
		Port:                 9007,
		Env:                  getEnv("ENV", "dev"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		StoryBucket:          os.Getenv("STORY_BUCKET"),
		PubsubProjectID:      os.Getenv("PUBSUB_PROJECT_ID"),
		FactionSelectedTopic: getEnv("FACTION_SELECTED_TOPIC", "faction-selected"),
		FirestoreProjectID:   os.Getenv("FIRESTORE_PROJECT_ID"),
	}

	if raw := os.Getenv("PORT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config: PORT %q: %w", raw, err)
		}
		cfg.Port = n
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.StoryBucket == "" {
		return nil, fmt.Errorf("config: STORY_BUCKET is required (GCS bucket name, or \"local:<path>\" for dev)")
	}
	if cfg.IsLocalStory() && cfg.StoryLocalPath() == "" {
		return nil, fmt.Errorf("config: STORY_BUCKET=%q: local path must not be empty after %q", cfg.StoryBucket, localStoryPrefix)
	}
	if cfg.PubsubProjectID == "" {
		return nil, fmt.Errorf("config: PUBSUB_PROJECT_ID is required (scenario publishes faction-selected events to Pub/Sub)")
	}
	if cfg.FirestoreProjectID == "" {
		return nil, fmt.Errorf("config: FIRESTORE_PROJECT_ID is required (game_config)")
	}
	return cfg, nil
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
