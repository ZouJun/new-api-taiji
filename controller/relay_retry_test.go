package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
