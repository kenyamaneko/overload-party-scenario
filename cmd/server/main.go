package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v5/pgxpool"

	adaptergcs "github.com/kenyamaneko/overload-party-scenario/internal/adapter/gcs"
	adapterlocal "github.com/kenyamaneko/overload-party-scenario/internal/adapter/local"
	scenariopubsub "github.com/kenyamaneko/overload-party-scenario/internal/adapter/pubsub"
	"github.com/kenyamaneko/overload-party-scenario/internal/config"
	"github.com/kenyamaneko/overload-party-scenario/internal/adapter/rest"
	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/repository"
	"github.com/kenyamaneko/overload-party-scenario/internal/router"
	"github.com/kenyamaneko/overload-party-scenario/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("scenario: %v", err)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pgxpool new: %w", err)
	}
	defer pool.Close()

	fsClient, err := firestore.NewClient(ctx, cfg.FirestoreProjectID)
	if err != nil {
		return fmt.Errorf("firestore new client: %w", err)
	}
	defer func() { _ = fsClient.Close() }()

	storyRepo := repository.NewPgStoryRepository(pool)
	// game_config は現在 scenario の runtime パスから参照していない。
	// クライアント到達性は起動時に検証するため、repo を生成だけしておく。
	_ = repository.NewFirestoreGameConfigRepository(fsClient)

	factionPublisher, err := scenariopubsub.NewFactionPublisher(ctx, cfg.PubsubProjectID, cfg.FactionSelectedTopic)
	if err != nil {
		return fmt.Errorf("faction publisher: %w", err)
	}
	defer func() {
		if cerr := factionPublisher.Close(); cerr != nil {
			log.Printf("scenario: faction publisher close failed: %v", cerr)
		}
	}()

	var scriptStore port.ScriptStore
	if cfg.IsLocalStory() {
		localRoot := cfg.StoryLocalPath()
		log.Printf("scenario: story source = local filesystem (root=%q)", localRoot)
		scriptStore = adapterlocal.NewScriptStore(localRoot)
	} else {
		gcsClient, err := storage.NewClient(ctx)
		if err != nil {
			return fmt.Errorf("gcs client: %w", err)
		}
		defer func() {
			if cerr := gcsClient.Close(); cerr != nil {
				log.Printf("scenario: gcs client close failed: %v", cerr)
			}
		}()
		log.Printf("scenario: story source = gcs bucket=%q", cfg.StoryBucket)
		scriptStore = adaptergcs.NewScriptStore(gcsClient, cfg.StoryBucket)
	}

	storySvc := service.NewStoryService(storyRepo, scriptStore, factionPublisher)

	storyH := rest.NewStoryHandler(storySvc)

	r := router.New(storyH)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("scenario: listening on %s (pubsub project=%s topic=%s)",
			srv.Addr, cfg.PubsubProjectID, cfg.FactionSelectedTopic)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("scenario: shutdown requested")
	case err := <-errCh:
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	return nil
}
