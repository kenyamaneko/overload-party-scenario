package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
)

type recordFailureCall struct {
	eventID uuid.UUID
	errMsg  string
}

type fakeOutboxStore struct {
	claimResult []port.ClaimedOutboxEvent
	claimErr    error

	markPublishedIDs   []uuid.UUID
	recordFailureCalls []recordFailureCall
}

func (f *fakeOutboxStore) ClaimUnpublished(ctx context.Context, limit int, visibilityTimeout time.Duration, failureThreshold int) ([]port.ClaimedOutboxEvent, error) {
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return f.claimResult, nil
}

func (f *fakeOutboxStore) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	f.markPublishedIDs = append(f.markPublishedIDs, eventID)
	return nil
}

func (f *fakeOutboxStore) RecordFailure(ctx context.Context, eventID uuid.UUID, errMsg string) error {
	f.recordFailureCalls = append(f.recordFailureCalls, recordFailureCall{eventID: eventID, errMsg: errMsg})
	return nil
}

type publishCall struct {
	eventType string
	payload   []byte
}

type fakeRawEventPublisher struct {
	err          error
	publishCalls []publishCall
}

func (f *fakeRawEventPublisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	f.publishCalls = append(f.publishCalls, publishCall{eventType: eventType, payload: payload})
	return f.err
}

func TestNew(t *testing.T) {
	t.Run("Relayの構築", func(t *testing.T) {
		validStore := &fakeOutboxStore{}
		validPub := &fakeRawEventPublisher{}
		validCfg := Config{BatchSize: 10, FailureThreshold: 3, VisibilityTimeout: time.Minute}

		errCases := []struct {
			name            string
			store           port.OutboxStore
			pub             port.RawEventPublisher
			cfg             Config
			wantErrContains string
		}{
			{
				name:            "storeがnilのとき、エラーになる",
				store:           nil,
				pub:             validPub,
				cfg:             validCfg,
				wantErrContains: "store",
			},
			{
				name:            "publisherがnilのとき、エラーになる",
				store:           validStore,
				pub:             nil,
				cfg:             validCfg,
				wantErrContains: "publisher",
			},
			{
				name:            "BatchSizeが0のとき、エラーになる",
				store:           validStore,
				pub:             validPub,
				cfg:             Config{BatchSize: 0, FailureThreshold: 3, VisibilityTimeout: time.Minute},
				wantErrContains: "BatchSize",
			},
			{
				name:            "FailureThresholdが0のとき、エラーになる",
				store:           validStore,
				pub:             validPub,
				cfg:             Config{BatchSize: 10, FailureThreshold: 0, VisibilityTimeout: time.Minute},
				wantErrContains: "FailureThreshold",
			},
			{
				name:            "VisibilityTimeoutが0のとき、エラーになる",
				store:           validStore,
				pub:             validPub,
				cfg:             Config{BatchSize: 10, FailureThreshold: 3, VisibilityTimeout: 0},
				wantErrContains: "VisibilityTimeout",
			},
		}
		for _, tc := range errCases {
			t.Run(tc.name, func(t *testing.T) {
				relay, err := New(tc.store, tc.pub, tc.cfg)

				require.Error(t, err)
				assert.Nil(t, relay)
				assert.ErrorContains(t, err, tc.wantErrContains)
			})
		}

		t.Run("storeとpublisherが設定され、BatchSize・FailureThreshold・VisibilityTimeoutが全て正の値のとき、構築が成功する", func(t *testing.T) {
			relay, err := New(validStore, validPub, validCfg)

			require.NoError(t, err)
			assert.NotNil(t, relay)
		})
	})
}

func TestRunOnce(t *testing.T) {
	t.Run("1バッチ分のclaim結果に対するpublish振り分け", func(t *testing.T) {
		validCfg := Config{BatchSize: 10, FailureThreshold: 3, VisibilityTimeout: time.Minute}

		t.Run("claim結果が0件のとき、エラーなく完了する", func(t *testing.T) {
			store := &fakeOutboxStore{claimResult: []port.ClaimedOutboxEvent{}}
			pub := &fakeRawEventPublisher{}
			relay, err := New(store, pub, validCfg)
			require.NoError(t, err)

			err = relay.RunOnce(context.Background())

			require.NoError(t, err)
			assert.Empty(t, store.markPublishedIDs)
			assert.Empty(t, store.recordFailureCalls)
		})

		t.Run("未配信イベントの取得自体が失敗するとき、エラーになる", func(t *testing.T) {
			errBoom := errors.New("claim unpublished boom")
			store := &fakeOutboxStore{claimErr: errBoom}
			pub := &fakeRawEventPublisher{}
			relay, err := New(store, pub, validCfg)
			require.NoError(t, err)

			err = relay.RunOnce(context.Background())

			require.ErrorIs(t, err, errBoom)
		})

		t.Run("claimされた行のpublishが成功するとき、その行の publish 済み記録が呼ばれる", func(t *testing.T) {
			eventID := uuid.New()
			store := &fakeOutboxStore{claimResult: []port.ClaimedOutboxEvent{
				{EventID: eventID, EventType: "onboarding_name_set", Payload: []byte(`{}`)},
			}}
			pub := &fakeRawEventPublisher{}
			relay, err := New(store, pub, validCfg)
			require.NoError(t, err)

			err = relay.RunOnce(context.Background())

			require.NoError(t, err)
			require.Len(t, store.markPublishedIDs, 1)
			assert.Equal(t, eventID, store.markPublishedIDs[0])
			assert.Empty(t, store.recordFailureCalls)
			require.Len(t, pub.publishCalls, 1)
			assert.Equal(t, "onboarding_name_set", pub.publishCalls[0].eventType)
			assert.Equal(t, []byte(`{}`), pub.publishCalls[0].payload)
		})

		t.Run("claimされた行のpublishが失敗するとき、その行の失敗記録が呼ばれ、エラーにならない", func(t *testing.T) {
			eventID := uuid.New()
			publishErr := errors.New("publish boom")
			store := &fakeOutboxStore{claimResult: []port.ClaimedOutboxEvent{
				{EventID: eventID, EventType: "onboarding_name_set", Payload: []byte(`{}`)},
			}}
			pub := &fakeRawEventPublisher{err: publishErr}
			relay, err := New(store, pub, validCfg)
			require.NoError(t, err)

			err = relay.RunOnce(context.Background())

			require.NoError(t, err)
			require.Len(t, store.recordFailureCalls, 1)
			assert.Equal(t, eventID, store.recordFailureCalls[0].eventID)
			assert.Contains(t, store.recordFailureCalls[0].errMsg, "publish boom")
			assert.Empty(t, store.markPublishedIDs)
		})
	})
}
