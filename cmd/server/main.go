package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	adaptergcs "github.com/kenyamaneko/overload-party-scenario/internal/adapter/gcs"
	adapterhttp "github.com/kenyamaneko/overload-party-scenario/internal/adapter/http"
	adapterlocal "github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	scenariopubsub "github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-scenario/internal/config"
	"github.com/kenyamaneko/overload-party-scenario/internal/handler/rest"
	"github.com/kenyamaneko/overload-party-scenario/internal/handler/worker"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	scenariofirestore "github.com/kenyamaneko/overload-party-scenario/internal/repository/firestore"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository/postgres"
	"github.com/kenyamaneko/overload-party-scenario/internal/router"
	"github.com/kenyamaneko/overload-party-scenario/internal/service/onboarding"
	outboxsvc "github.com/kenyamaneko/overload-party-scenario/internal/service/outbox"
	"github.com/kenyamaneko/overload-party-scenario/internal/service/story"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scenario fatal", "error", err)
		os.Exit(1)
	}
}

// setupLogger は ENV に応じてグローバル slog ロガーを初期化する。
// 本番は Cloud Logging 互換 JSON、それ以外は人間可読な text handler。
func setupLogger(env string) {
	var h slog.Handler
	if env == "prod" {
		h = newCloudLoggingHandler()
	} else {
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	slog.SetDefault(slog.New(h).With("service", "scenario"))
}

// newCloudLoggingHandler は Cloud Logging 互換の JSON ハンドラを返す。
// slog のデフォルトフィールド名・値では Cloud Logging が認識しないため変換する。
func newCloudLoggingHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				a.Key = "severity"
				if level, ok := a.Value.Any().(slog.Level); ok {
					switch {
					case level >= slog.LevelError:
						a.Value = slog.StringValue("ERROR")
					case level >= slog.LevelWarn:
						a.Value = slog.StringValue("WARNING")
					case level >= slog.LevelInfo:
						a.Value = slog.StringValue("INFO")
					default:
						a.Value = slog.StringValue("DEBUG")
					}
				}
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	setupLogger(cfg.Env)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseConn)
	if err != nil {
		return fmt.Errorf("pgxpool new: %w", err)
	}
	defer pool.Close()

	fsClient, err := firestore.NewClient(ctx, cfg.FirestoreProjectID)
	if err != nil {
		return fmt.Errorf("firestore new client: %w", err)
	}
	defer func() { _ = fsClient.Close() }()
	// game_config は現在 scenario の runtime パスから参照していない。
	// クライアント到達性は起動時に検証するため、repo を生成だけしておく。
	_ = scenariofirestore.NewGameConfigRepository(fsClient)

	pubPublisher, err := scenariopubsub.New(ctx, cfg.PubsubProjectID, cfg.PlayerOnboardedTopic)
	if err != nil {
		return fmt.Errorf("pubsub publisher: %w", err)
	}
	defer func() {
		if cerr := pubPublisher.Close(); cerr != nil {
			slog.Error("pubsub publisher close failed", "error", cerr)
		}
	}()

	scriptStore, gcsCloser, err := buildScriptStore(ctx, cfg)
	if err != nil {
		return err
	}
	if gcsCloser != nil {
		defer func() {
			if cerr := gcsCloser.Close(); cerr != nil {
				slog.Error("gcs client close failed", "error", cerr)
			}
		}()
	}

	storyRepo := postgres.NewStoryRepository(pool)
	storySvc := story.New(storyRepo, scriptStore)
	storyH := rest.NewStoryHandler(storySvc)

	accountClient := adapterhttp.NewAccountClient(cfg.AccountBaseURL)

	onboardingRepo := postgres.NewOnboardingRepository(pool)
	onboardingSvc := onboarding.New(onboardingRepo, scriptStore, accountClient, accountClient)
	onboardingH := rest.NewOnboardingHandler(onboardingSvc)

	outboxRepo := postgres.NewOutboxRepository(pool)
	outboxRelay, err := outboxsvc.New(outboxRepo, pubPublisher, outboxsvc.Config{
		BatchSize:         cfg.OutboxBatchSize,
		FailureThreshold:  cfg.OutboxFailureThreshold,
		VisibilityTimeout: cfg.OutboxVisibilityTimeout,
	})
	if err != nil {
		return fmt.Errorf("outbox relay: %w", err)
	}
	outboxTicker, err := worker.NewOutboxTicker(outboxRelay, cfg.OutboxPollInterval)
	if err != nil {
		return fmt.Errorf("outbox ticker: %w", err)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router.New(storyH, onboardingH),
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("listening",
		"addr", srv.Addr,
		"pubsub_project", cfg.PubsubProjectID,
		"player_onboarded_topic", cfg.PlayerOnboardedTopic,
		"outbox_poll_interval", cfg.OutboxPollInterval,
	)

	return runHTTP(ctx, srv, outboxTicker)
}

// buildScriptStore は StoryBucket の形式に応じて GCS / local ScriptStore を構築する。
// GCS を選んだ場合は close すべき client も返し、呼び出し側でリソースリリースさせる。
func buildScriptStore(ctx context.Context, cfg *config.Config) (port.ScriptStore, *storage.Client, error) {
	if cfg.IsLocalStory() {
		root := cfg.StoryLocalPath()
		slog.Info("script store source = local filesystem", "root", root)
		return adapterlocal.NewScriptStore(root), nil, nil
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("gcs client: %w", err)
	}
	slog.Info("script store source = gcs", "bucket", cfg.StoryBucket)
	return adaptergcs.NewScriptStore(client, cfg.StoryBucket), client, nil
}

// runHTTP は HTTP server と outbox ticker を起動し、ctx キャンセル時に graceful shutdown する。
// ADR-021 で scenario に常駐 worker (outbox poller) が入ったため errgroup で束ねる。
func runHTTP(ctx context.Context, srv *http.Server, outboxTicker *worker.OutboxTicker) error {
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		return outboxTicker.Run(gCtx)
	})

	g.Go(func() error {
		<-gCtx.Done()
		slog.Info("shutdown requested")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}
