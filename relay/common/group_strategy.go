package common

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type RuntimeGroupStrategy struct {
	Group                      string
	Matched                    bool
	Strategy                   operation_setting.GroupStrategy
	EffectiveRetryTimes        int
	StreamRetryFirstByteBudget time.Duration
	TimeoutHTTPStatus          int
	TimeoutErrorMessage        string
}

func ResolveCurrentStrategyGroup(c *gin.Context, info *RelayInfo) string {
	if c != nil {
		if autoGroup := strings.TrimSpace(rootcommon.GetContextKeyString(c, constant.ContextKeyAutoGroup)); autoGroup != "" {
			return autoGroup
		}
	}
	if info != nil {
		if group := strings.TrimSpace(info.UsingGroup); group != "" && group != "auto" {
			return group
		}
		if group := strings.TrimSpace(info.TokenGroup); group != "" && group != "auto" {
			return group
		}
		if group := strings.TrimSpace(info.UserGroup); group != "" {
			return group
		}
	}
	if c == nil {
		return ""
	}
	if group := strings.TrimSpace(rootcommon.GetContextKeyString(c, constant.ContextKeyUsingGroup)); group != "" && group != "auto" {
		return group
	}
	if group := strings.TrimSpace(rootcommon.GetContextKeyString(c, constant.ContextKeyTokenGroup)); group != "" && group != "auto" {
		return group
	}
	return strings.TrimSpace(rootcommon.GetContextKeyString(c, constant.ContextKeyUserGroup))
}

func ResolveRuntimeGroupStrategy(c *gin.Context, info *RelayInfo) RuntimeGroupStrategy {
	group := ResolveCurrentStrategyGroup(c, info)
	runtime := RuntimeGroupStrategy{
		Group:               group,
		EffectiveRetryTimes: rootcommon.RetryTimes,
		TimeoutHTTPStatus:   operation_setting.DefaultGroupStrategyTimeoutHTTPStatus,
		TimeoutErrorMessage: operation_setting.DefaultGroupStrategyTimeoutErrorMessage,
	}
	strategy, ok := operation_setting.ResolveGroupStrategy(group)
	if !ok {
		return runtime
	}
	runtime.Matched = true
	runtime.Strategy = strategy
	if strategy.RetryTimes != nil {
		runtime.EffectiveRetryTimes = *strategy.RetryTimes
	}
	if strategy.StreamRetryFirstByteBudgetSeconds != nil {
		runtime.StreamRetryFirstByteBudget = time.Duration(*strategy.StreamRetryFirstByteBudgetSeconds) * time.Second
	}
	if strategy.TimeoutHTTPStatus != nil {
		runtime.TimeoutHTTPStatus = *strategy.TimeoutHTTPStatus
	}
	if message := strings.TrimSpace(strategy.TimeoutErrorMessage); message != "" {
		runtime.TimeoutErrorMessage = message
	}
	return runtime
}

func SnapshotRuntimeGroupStrategy(c *gin.Context, info *RelayInfo) RuntimeGroupStrategy {
	runtime := ResolveRuntimeGroupStrategy(c, info)
	if c != nil {
		rootcommon.SetContextKey(c, constant.ContextKeyGroupStrategy, runtime)
	}
	if info != nil {
		info.GroupStrategyGroup = runtime.Group
		info.GroupStrategyMatched = runtime.Matched
		info.EffectiveRetryTimes = runtime.EffectiveRetryTimes
		info.StreamRetryFirstByteBudget = runtime.StreamRetryFirstByteBudget
		info.StreamRetryFirstByteWaitSpent = GetConsumedStreamFirstByteRetryWait(c)
		if runtime.Group != "" {
			info.UsingGroup = runtime.Group
		}
	}
	return runtime
}

func ResolveEffectiveRetryTimes(c *gin.Context, info *RelayInfo) int {
	if c != nil {
		if cached, ok := rootcommon.GetContextKeyType[RuntimeGroupStrategy](c, constant.ContextKeyGroupStrategy); ok {
			return cached.EffectiveRetryTimes
		}
	}
	return SnapshotRuntimeGroupStrategy(c, info).EffectiveRetryTimes
}

func ResolveEffectiveRetryTimesForGroup(group string) int {
	group = strings.TrimSpace(group)
	if strategy, ok := operation_setting.ResolveGroupStrategy(group); ok && strategy.RetryTimes != nil {
		return *strategy.RetryTimes
	}
	return rootcommon.RetryTimes
}

func GetConsumedStreamFirstByteRetryWait(c *gin.Context) time.Duration {
	if c == nil {
		return 0
	}
	wait, ok := rootcommon.GetContextKeyType[time.Duration](c, constant.ContextKeyGroupStrategyWait)
	if !ok {
		return 0
	}
	return wait
}

func ConsumeStreamFirstByteRetryBudget(c *gin.Context, info *RelayInfo, wait time.Duration) (RuntimeGroupStrategy, time.Duration, bool) {
	runtime := SnapshotRuntimeGroupStrategy(c, info)
	if wait <= 0 {
		return runtime, GetConsumedStreamFirstByteRetryWait(c), false
	}
	spent := GetConsumedStreamFirstByteRetryWait(c) + wait
	if c != nil {
		rootcommon.SetContextKey(c, constant.ContextKeyGroupStrategyWait, spent)
	}
	if info != nil {
		info.StreamRetryFirstByteWaitSpent = spent
	}
	if runtime.StreamRetryFirstByteBudget <= 0 {
		return runtime, spent, false
	}
	return runtime, spent, spent >= runtime.StreamRetryFirstByteBudget
}

func ResolveAttemptStreamFirstByteWait(info *RelayInfo, attemptStart time.Time, relayErr *types.NewAPIError) time.Duration {
	if info == nil || !info.IsStream || attemptStart.IsZero() {
		return 0
	}
	if !info.AttemptFirstResponseTime.IsZero() && info.AttemptFirstResponseTime.After(attemptStart) {
		return info.AttemptFirstResponseTime.Sub(attemptStart)
	}
	if relayErr != nil && errors.Is(relayErr, ErrStreamFirstByteTimeout) {
		timeoutSeconds, _ := ResolveStreamFirstByteTimeoutSeconds(info)
		if timeoutSeconds > 0 {
			return time.Duration(timeoutSeconds) * time.Second
		}
	}
	return 0
}

func IsTimeoutFeatureRelayError(c *gin.Context, upstreamErr *types.NewAPIError) bool {
	if upstreamErr == nil {
		return false
	}
	if upstreamErr.StatusCode == http.StatusGatewayTimeout && errors.Is(upstreamErr, ErrStreamFirstByteTimeout) {
		return true
	}
	if upstreamErr.StatusCode != http.StatusGatewayTimeout || !errors.Is(upstreamErr, context.DeadlineExceeded) {
		return false
	}
	meta, ok := GetTimeoutMeta(c)
	if !ok {
		return false
	}
	return meta.Type == TimeoutTypeNonStreamTotal || meta.Type == TimeoutTypeStreamFirstByte
}

func BuildConfiguredTimeoutRelayError(c *gin.Context, runtime RuntimeGroupStrategy, upstreamErr *types.NewAPIError) *types.NewAPIError {
	if !IsTimeoutFeatureRelayError(c, upstreamErr) {
		return upstreamErr
	}
	statusCode := runtime.TimeoutHTTPStatus
	if statusCode == 0 {
		statusCode = operation_setting.DefaultGroupStrategyTimeoutHTTPStatus
	}
	message := strings.TrimSpace(runtime.TimeoutErrorMessage)
	if message == "" {
		message = operation_setting.DefaultGroupStrategyTimeoutErrorMessage
	}
	return types.NewErrorWithStatusCode(errors.New(message), upstreamErr.GetErrorCode(), statusCode)
}

func BuildClientTimeoutResponseError(c *gin.Context, upstreamErr *types.NewAPIError) *types.NewAPIError {
	if !IsTimeoutFeatureRelayError(c, upstreamErr) {
		return upstreamErr
	}
	statusCode, message := operation_setting.ResolveClientTimeoutResponse()
	if statusCode == 0 && message == "" {
		return upstreamErr
	}
	if statusCode == 0 {
		statusCode = upstreamErr.StatusCode
	}
	if message == "" {
		message = upstreamErr.Error()
	}
	return types.NewErrorWithStatusCode(errors.New(message), upstreamErr.GetErrorCode(), statusCode)
}

func BuildGroupStrategyTimeoutError(runtime RuntimeGroupStrategy, upstreamErr *types.NewAPIError) *types.NewAPIError {
	return BuildConfiguredTimeoutRelayError(nil, runtime, upstreamErr)
}
