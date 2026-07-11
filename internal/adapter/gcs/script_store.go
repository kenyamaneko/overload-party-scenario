package gcs

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"cloud.google.com/go/storage"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

// ScriptStore は GCS バケットからストーリースクリプトを読み込む。
type ScriptStore struct {
	client     *storage.Client
	bucketName string
}

// NewScriptStore は指定 GCS バケットを使用する ScriptStore を構築する。
func NewScriptStore(client *storage.Client, bucketName string) *ScriptStore {
	return &ScriptStore{client: client, bucketName: bucketName}
}

// ReadScript は GCS から指定キーのスクリプトを読み込む。
func (s *ScriptStore) ReadScript(ctx context.Context, key string) (string, error) {
	rc, err := s.client.Bucket(s.bucketName).Object(key).NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return "", fmt.Errorf("%w: bucket=%q object=%q",
				port.ErrScriptNotFound, s.bucketName, key)
		}
		return "", fmt.Errorf("%w: gcs read (bucket=%q object=%q): %v",
			port.ErrScriptInfra, s.bucketName, key, err)
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			slog.Warn("gcs reader close failed",
				"bucket", s.bucketName, "object", key, "error", cerr)
		}
	}()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("%w: gcs read body (bucket=%q object=%q): %v",
			port.ErrScriptInfra, s.bucketName, key, err)
	}
	return string(data), nil
}
