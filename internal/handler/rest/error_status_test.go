package rest

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kenyamaneko/overload-party-scenario/internal/port"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/onboarding"
	"github.com/kenyamaneko/overload-party-scenario/internal/usecase/story"
)

// TestResolveOnboardingErrorStatus は onboarding エラーのステータス分類契約を固定する。
func TestResolveOnboardingErrorStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "script not found は 404",
			err:  onboarding.ErrScriptNotFound,
			want: http.StatusNotFound,
		},
		{
			name: "player not found は 404",
			err:  onboarding.ErrPlayerNotFound,
			want: http.StatusNotFound,
		},
		{
			name: "already onboarded は 409",
			err:  onboarding.ErrAlreadyOnboarded,
			want: http.StatusConflict,
		},
		{
			name: "faction not selected は 409",
			err:  onboarding.ErrFactionNotSelected,
			want: http.StatusConflict,
		},
		{
			name: "invalid faction は 400",
			err:  onboarding.ErrInvalidFaction,
			want: http.StatusBadRequest,
		},
		{
			name: "invalid name は 400",
			err:  onboarding.ErrInvalidName,
			want: http.StatusBadRequest,
		},
		{
			name: "ラップされた sentinel も同じ分類になる",
			err:  fmt.Errorf("update onboarding name: %w", onboarding.ErrInvalidName),
			want: http.StatusBadRequest,
		},
		{
			name: "未知のエラーは 500",
			err:  errors.New("unexpected failure"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveOnboardingErrorStatus(tc.err))
		})
	}
}

// TestResolveErrorStatus は story エラーのステータス分類契約を固定する。
func TestResolveErrorStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "episode not found は 404",
			err:  story.ErrEpisodeNotFound,
			want: http.StatusNotFound,
		},
		{
			name: "script not found は 404",
			err:  port.ErrScriptNotFound,
			want: http.StatusNotFound,
		},
		{
			name: "episode locked は 403",
			err:  story.ErrEpisodeLocked,
			want: http.StatusForbidden,
		},
		{
			name: "script infra は 500",
			err:  port.ErrScriptInfra,
			want: http.StatusInternalServerError,
		},
		{
			name: "ラップされた infra も 500 に分類する",
			err:  fmt.Errorf("read episode script: %w", port.ErrScriptInfra),
			want: http.StatusInternalServerError,
		},
		{
			name: "未知のエラーは 500",
			err:  errors.New("unexpected failure"),
			want: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveErrorStatus(tc.err))
		})
	}
}
