package common

import (
	"net/http/httptest"
	"testing"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func TestResolveEffectiveRetryTimesFallsBackToGlobal(t *testing.T) {
	prepareGroupStrategyRuntimeTestOptions()
	originalRetryTimes := rootcommon.RetryTimes
	rootcommon.RetryTimes = 3
	t.Cleanup(func() {
		rootcommon.RetryTimes = originalRetryTimes
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"retry_times":1}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	rootcommon.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")

	if got := ResolveEffectiveRetryTimes(ctx, nil); got != 3 {
		t.Fatalf("expected global retry fallback 3, got %d", got)
	}
}

func TestResolveEffectiveRetryTimesUsesMatchedGroup(t *testing.T) {
	prepareGroupStrategyRuntimeTestOptions()
	originalRetryTimes := rootcommon.RetryTimes
	rootcommon.RetryTimes = 2
	t.Cleanup(func() {
		rootcommon.RetryTimes = originalRetryTimes
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"retry_times":4}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	rootcommon.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	if got := ResolveEffectiveRetryTimes(ctx, nil); got != 4 {
		t.Fatalf("expected retry override 4, got %d", got)
	}
}

func TestResolveEffectiveRetryTimesPrefersAutoGroup(t *testing.T) {
	prepareGroupStrategyRuntimeTestOptions()
	originalRetryTimes := rootcommon.RetryTimes
	rootcommon.RetryTimes = 2
	t.Cleanup(func() {
		rootcommon.RetryTimes = originalRetryTimes
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"retry_times":1},"vip":{"enabled":true,"retry_times":5}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	rootcommon.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	rootcommon.SetContextKey(ctx, constant.ContextKeyAutoGroup, "vip")

	if got := ResolveEffectiveRetryTimes(ctx, nil); got != 5 {
		t.Fatalf("expected auto group retry override 5, got %d", got)
	}
}

func TestConsumeStreamFirstByteRetryBudget(t *testing.T) {
	prepareGroupStrategyRuntimeTestOptions()
	t.Cleanup(func() {
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
	if err := operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"stream_retry_first_byte_budget_seconds":3}}`); err != nil {
		t.Fatalf("setup group strategy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	rootcommon.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	runtime, spent, exceeded := ConsumeStreamFirstByteRetryBudget(ctx, nil, 2*time.Second)
	if exceeded {
		t.Fatal("expected first consume not to exhaust budget")
	}
	if runtime.StreamRetryFirstByteBudget != 3*time.Second {
		t.Fatalf("expected 3s budget, got %s", runtime.StreamRetryFirstByteBudget)
	}
	if spent != 2*time.Second {
		t.Fatalf("expected spent 2s, got %s", spent)
	}

	_, spent, exceeded = ConsumeStreamFirstByteRetryBudget(ctx, nil, 1*time.Second)
	if !exceeded {
		t.Fatal("expected second consume to exhaust budget")
	}
	if spent != 3*time.Second {
		t.Fatalf("expected total spent 3s, got %s", spent)
	}
}

func TestResolveAttemptStreamFirstByteWaitFromFirstResponse(t *testing.T) {
	t.Parallel()

	start := time.Now()
	info := &RelayInfo{
		IsStream:                 true,
		AttemptFirstResponseTime: start.Add(1500 * time.Millisecond),
	}

	wait := ResolveAttemptStreamFirstByteWait(info, start, nil)
	if wait != 1500*time.Millisecond {
		t.Fatalf("expected 1.5s wait, got %s", wait)
	}
}

func TestBuildGroupStrategyTimeoutError(t *testing.T) {
	t.Parallel()

	upstreamErr := types.NewErrorWithStatusCode(ErrStreamFirstByteTimeout, types.ErrorCodeDoRequestFailed, 504)
	runtime := RuntimeGroupStrategy{
		TimeoutHTTPStatus:   429,
		TimeoutErrorMessage: "busy",
	}
	err := BuildGroupStrategyTimeoutError(runtime, upstreamErr)
	if err.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", err.StatusCode)
	}
	if err.Error() != "busy" {
		t.Fatalf("expected custom message, got %q", err.Error())
	}
}

func TestBuildGroupStrategyTimeoutErrorPreservesNonTimeoutError(t *testing.T) {
	t.Parallel()

	upstreamErr := types.NewErrorWithStatusCode(ErrStreamFirstByteTimeout, types.ErrorCodeDoRequestFailed, 504)
	upstreamErr.SetMessage("upstream busy")
	nonTimeoutErr := types.NewErrorWithStatusCode(upstreamErr, types.ErrorCodeDoRequestFailed, 502, types.ErrOptionWithStatusCode(502))
	nonTimeoutErr.SetMessage("upstream specific error")

	err := BuildGroupStrategyTimeoutError(RuntimeGroupStrategy{
		TimeoutHTTPStatus:   429,
		TimeoutErrorMessage: "busy",
	}, nonTimeoutErr)
	if err.StatusCode != 502 {
		t.Fatalf("expected upstream status 502 to be preserved, got %d", err.StatusCode)
	}
	if err.Error() != "upstream specific error" {
		t.Fatalf("expected upstream message to be preserved, got %q", err.Error())
	}
}

func prepareGroupStrategyRuntimeTestOptions() {
	rootcommon.OptionMapRWMutex.Lock()
	defer rootcommon.OptionMapRWMutex.Unlock()
	if rootcommon.OptionMap == nil {
		rootcommon.OptionMap = make(map[string]string)
	}
	rootcommon.OptionMap["GroupRatio"] = `{"default":1,"vip":1}`
}
