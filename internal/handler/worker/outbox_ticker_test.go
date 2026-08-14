package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/handler/worker"
)

const (
	tickWaitFor  = 3 * time.Second
	tickWaitTick = time.Millisecond
	tickInterval = 5 * time.Millisecond
)

type fakeOutboxRunner struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (r *fakeOutboxRunner) RunOnce(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.err
}

func (r *fakeOutboxRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestNewOutboxTicker(t *testing.T) {
	t.Run("[配信]定期実行の構築", func(t *testing.T) {
		t.Run("実行対象がnilのとき、エラーになる", func(t *testing.T) {
			ticker, err := worker.NewOutboxTicker(nil, tickInterval)

			assert.ErrorContains(t, err, "runner is nil")
			assert.Nil(t, ticker)
		})

		t.Run("実行間隔が0以下のとき、エラーになる", func(t *testing.T) {
			cases := []struct {
				name     string
				interval time.Duration
			}{
				{name: "実行間隔が0のとき", interval: 0},
				{name: "実行間隔が負のとき", interval: -1 * time.Second},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					ticker, err := worker.NewOutboxTicker(&fakeOutboxRunner{}, tc.interval)

					assert.ErrorContains(t, err, "interval must be positive")
					assert.Nil(t, ticker)
				})
			}
		})

		t.Run("実行対象と実行間隔が両方有効なとき、構築が成功する", func(t *testing.T) {
			ticker, err := worker.NewOutboxTicker(&fakeOutboxRunner{}, tickInterval)

			require.NoError(t, err)
			assert.NotNil(t, ticker)
		})
	})
}

func TestOutboxTickerRun(t *testing.T) {
	t.Run("[配信]定期実行", func(t *testing.T) {
		t.Run("下流の定期実行処理が正常に完了するとき、intervalごとに繰り返し呼ばれる", func(t *testing.T) {
			runner := &fakeOutboxRunner{}
			ticker, err := worker.NewOutboxTicker(runner, tickInterval)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- ticker.Run(ctx) }()

			assert.Eventually(t, func() bool { return runner.callCount() >= 3 }, tickWaitFor, tickWaitTick)

			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(tickWaitFor):
				t.Fatal("Run did not return after context cancellation")
			}
		})

		t.Run("下流の定期実行処理が毎回エラーを返すとき、呼び出しを継続する", func(t *testing.T) {
			runner := &fakeOutboxRunner{err: errors.New("tick failed")}
			ticker, err := worker.NewOutboxTicker(runner, tickInterval)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- ticker.Run(ctx) }()

			assert.Eventually(t, func() bool { return runner.callCount() >= 3 }, tickWaitFor, tickWaitTick)

			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(tickWaitFor):
				t.Fatal("Run did not return after context cancellation")
			}
		})

		t.Run("コンテキストがキャンセルされると、エラーなく終了する", func(t *testing.T) {
			runner := &fakeOutboxRunner{}
			ticker, err := worker.NewOutboxTicker(runner, time.Hour)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- ticker.Run(ctx) }()

			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(tickWaitFor):
				t.Fatal("Run did not return after context cancellation")
			}
		})
	})
}
