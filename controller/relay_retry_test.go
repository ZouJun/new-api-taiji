package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestShouldRetry_RetriesGatewayTimeoutErrors(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	timeoutErr := types.NewErrorWithStatusCode(
		context.DeadlineExceeded,
		types.ErrorCodeDoRequestFailed,
		http.StatusGatewayTimeout,
	)

	if !shouldRetry(ctx, timeoutErr, 1) {
		t.Fatal("expected gateway timeout error to remain retry-eligible")
	}
}

func TestShouldRetry_DoesNotRetrySkipRetryErrors(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	skipRetryErr := types.NewErrorWithStatusCode(
		errors.New("bad request"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)

	if shouldRetry(ctx, skipRetryErr, 1) {
		t.Fatal("expected skip-retry error to bypass retry")
	}
}

func TestShouldHideInternalRelayError(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	timeoutErr := types.NewErrorWithStatusCode(
		context.DeadlineExceeded,
		types.ErrorCodeDoRequestFailed,
		http.StatusGatewayTimeout,
	)
	ctx.Set(string(constant.ContextKeyTimeoutMeta), relaycommon.TimeoutMeta{
		Type: relaycommon.TimeoutTypeNonStreamTotal,
	})
	if !shouldHideInternalRelayError(ctx, timeoutErr) {
		t.Fatal("expected timeout error to be hidden from client response")
	}

	retriedErr := types.NewErrorWithStatusCode(
		errors.New("internal retry failure"),
		types.ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)
	ctx.Set("use_channel", []string{"2", "3"})
	if shouldHideInternalRelayError(ctx, retriedErr) {
		t.Fatal("expected non-timeout retry error to keep original response message")
	}

	normalErr := types.NewErrorWithStatusCode(
		errors.New("invalid request"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)
	ctx.Set("use_channel", []string{"2"})
	if shouldHideInternalRelayError(ctx, normalErr) {
		t.Fatal("expected normal client error to keep original response message")
	}
}
