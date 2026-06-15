package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func TestShouldRetry_UsesGroupStrategyRetryOverride(t *testing.T) {
	prepareGroupStrategyRelayTestOptions()
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 3
	t.Cleanup(func() {
		common.RetryTimes = originalRetryTimes
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"retry_times":1}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	if got := relaycommon.ResolveEffectiveRetryTimes(ctx, nil); got != 1 {
		t.Fatalf("expected group retry override 1, got %d", got)
	}

	timeoutErr := types.NewErrorWithStatusCode(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	if shouldRetry(ctx, timeoutErr, 0) {
		t.Fatal("expected no retry when group override is exhausted")
	}
}

func TestGroupStrategyTimeoutErrorWinsAfterRetryBudgetExhausted(t *testing.T) {
	prepareGroupStrategyRelayTestOptions()
	t.Cleanup(func() {
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"stream_retry_first_byte_budget_seconds":2,"timeout_http_status":429,"timeout_error_message":"busy"}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	info := &relaycommon.RelayInfo{IsStream: true}
	upstreamErr := types.NewErrorWithStatusCode(relaycommon.ErrStreamFirstByteTimeout, types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	runtime, _, exceeded := relaycommon.ConsumeStreamFirstByteRetryBudget(ctx, info, 2*time.Second)
	if !exceeded {
		t.Fatal("expected retry budget to be exhausted")
	}

	finalErr := relaycommon.BuildGroupStrategyTimeoutError(runtime, upstreamErr)
	if finalErr.StatusCode != 429 {
		t.Fatalf("expected custom status 429, got %d", finalErr.StatusCode)
	}
	if finalErr.Error() != "busy" {
		t.Fatalf("expected custom message, got %q", finalErr.Error())
	}
}

func TestShouldRetry_DoesNotRetryChannelErrorsWhenExhausted(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	channelErr := types.NewError(errors.New("no key available"), types.ErrorCodeChannelNoAvailableKey)
	if shouldRetry(ctx, channelErr, 0) {
		t.Fatal("expected exhausted channel error not to retry")
	}
}

func TestShouldRetry_RetriesStreamFirstByteTimeoutWhenBudgetRemains(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	streamErr := types.NewErrorWithStatusCode(relaycommon.ErrStreamFirstByteTimeout, types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	if !shouldRetry(ctx, streamErr, 1) {
		t.Fatal("expected stream first-byte timeout to remain retry-eligible")
	}
}

func TestShouldRetry_RetriesMaskedStreamFirstByteTimeoutWhenBudgetRemains(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	streamErr := types.NewError(
		relaycommon.ErrStreamFirstByteTimeout,
		types.ErrorCodeDoRequestFailed,
		types.ErrOptionWithStatusCode(http.StatusGatewayTimeout),
		types.ErrOptionWithHideErrMsg("upstream error: do request failed"),
	)

	if !shouldRetry(ctx, streamErr, 1) {
		t.Fatal("expected masked stream first-byte timeout to remain retry-eligible")
	}
	if got := streamErr.ToOpenAIError().Message; got != "upstream error: do request failed" {
		t.Fatalf("expected masked client message, got %q", got)
	}
}

func TestShouldRetry_DoesNotRetrySpecificChannelRequests(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("specific_channel_id", 123)

	timeoutErr := types.NewErrorWithStatusCode(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	if shouldRetry(ctx, timeoutErr, 1) {
		t.Fatal("expected specific channel requests not to retry")
	}
}

func prepareGroupStrategyRelayTestOptions() {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["GroupRatio"] = `{"default":1,"vip":1}`
}
