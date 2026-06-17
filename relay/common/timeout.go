package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const (
	TimeoutTypeNonStreamTotal  = "non_stream_total"
	TimeoutTypeStreamFirstByte = "stream_first_byte"

	TimeoutSourceChannelSetting = "channel_setting"
	TimeoutSourceProviderGlobal = "provider_global"
	TimeoutSourceLegacyGlobal   = "legacy_global"
	TimeoutSourceGroupBudget    = "group_strategy_budget"
	TimeoutSourceNone           = "none"

	TimeoutWrapReasonNone                = "none"
	TimeoutWrapReasonGroupBudgetExceeded = "group_budget_exhausted"
	TimeoutWrapReasonChannelSetting      = "channel_timeout_response"
)

var ErrStreamFirstByteTimeout = errors.New("stream first byte timeout")

type TimeoutMeta struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Seconds     int    `json:"seconds"`
	Provider    string `json:"provider,omitempty"`
	Stage       string `json:"stage,omitempty"`
	RetryIndex  int    `json:"retry_index,omitempty"`
	IsStream    bool   `json:"is_stream,omitempty"`
	ChannelID   int    `json:"channel_id,omitempty"`
	RequestPath string `json:"request_path,omitempty"`
}

type TimeoutTrace struct {
	Type                     string        `json:"type,omitempty"`
	Source                   string        `json:"source,omitempty"`
	Stage                    string        `json:"stage,omitempty"`
	Provider                 string        `json:"provider,omitempty"`
	ConfiguredTimeoutSeconds int           `json:"configured_timeout_seconds,omitempty"`
	ActualElapsed            time.Duration `json:"actual_elapsed,omitempty"`
	AttemptStartedAt         time.Time     `json:"attempt_started_at,omitempty"`
	AttemptEndedAt           time.Time     `json:"attempt_ended_at,omitempty"`
	GroupBudget              time.Duration `json:"group_budget,omitempty"`
	GroupBudgetSpent         time.Duration `json:"group_budget_spent,omitempty"`
	GroupBudgetRemaining     time.Duration `json:"group_budget_remaining,omitempty"`
	WrapReason               string        `json:"wrap_reason,omitempty"`
}

func normalizePositiveTimeout(value *int) (int, bool) {
	if value == nil || *value <= 0 {
		return 0, false
	}
	return *value, true
}

func ResolveNonStreamTimeoutSeconds(info *RelayInfo) (int, string) {
	if info != nil {
		if timeout, ok := normalizePositiveTimeout(info.ChannelSetting.NonStreamTimeoutSeconds); ok {
			return timeout, TimeoutSourceChannelSetting
		}
	}
	if rootcommon.RelayDefaultNonStreamTimeout > 0 {
		return rootcommon.RelayDefaultNonStreamTimeout, TimeoutSourceProviderGlobal
	}
	if rootcommon.RelayTimeout > 0 {
		return rootcommon.RelayTimeout, TimeoutSourceLegacyGlobal
	}
	return 0, TimeoutSourceNone
}

func ResolveStreamFirstByteTimeoutSeconds(info *RelayInfo) (int, string) {
	if info != nil {
		if timeout, ok := normalizePositiveTimeout(info.ChannelSetting.StreamFirstByteTimeoutSeconds); ok {
			return timeout, TimeoutSourceChannelSetting
		}
	}
	if rootcommon.RelayDefaultStreamFirstByteTimeout > 0 {
		return rootcommon.RelayDefaultStreamFirstByteTimeout, TimeoutSourceProviderGlobal
	}
	if rootcommon.RelayTimeout > 0 {
		return rootcommon.RelayTimeout, TimeoutSourceLegacyGlobal
	}
	return 0, TimeoutSourceNone
}

func ResolveEffectiveStreamFirstByteTimeout(c *gin.Context, info *RelayInfo) (time.Duration, int, string) {
	baseSeconds, baseSource := ResolveStreamFirstByteTimeoutSeconds(info)
	baseDuration := time.Duration(baseSeconds) * time.Second

	if c == nil {
		return baseDuration, baseSeconds, baseSource
	}

	runtime := SnapshotRuntimeGroupStrategy(c, info)
	if runtime.StreamRetryFirstByteBudget <= 0 {
		return baseDuration, baseSeconds, baseSource
	}

	remaining := runtime.StreamRetryFirstByteBudget - GetConsumedStreamFirstByteRetryWait(c)
	if remaining <= 0 {
		return 0, 0, TimeoutSourceGroupBudget
	}
	if baseDuration <= 0 || remaining < baseDuration {
		return remaining, int(remaining / time.Second), TimeoutSourceGroupBudget
	}
	return baseDuration, baseSeconds, baseSource
}

func ResolveAWSSDKMaxAttempts(info *RelayInfo) (int, string) {
	if info != nil {
		if attempts, ok := normalizePositiveTimeout(info.ChannelSetting.AwsSDKMaxAttempts); ok {
			return attempts, TimeoutSourceChannelSetting
		}
	}
	if rootcommon.AWSSDKMaxAttempts > 0 {
		return rootcommon.AWSSDKMaxAttempts, TimeoutSourceProviderGlobal
	}
	return 0, TimeoutSourceNone
}

func ResolveAWSInvokeTimeoutSeconds(info *RelayInfo) (int, string) {
	// 这里的 invoke 超时只给 AWS 非流式调用用。
	// AWS 流式请求在当前方案里只看首包等待超时，不走总 invoke 超时。
	if info != nil {
		if timeout, ok := normalizePositiveTimeout(info.ChannelSetting.AwsInvokeTimeoutSeconds); ok {
			return timeout, TimeoutSourceChannelSetting
		}
	}
	if rootcommon.AWSInvokeTimeoutSeconds > 0 {
		return rootcommon.AWSInvokeTimeoutSeconds, TimeoutSourceProviderGlobal
	}
	return ResolveNonStreamTimeoutSeconds(info)
}

func SetTimeoutMeta(c *gin.Context, meta TimeoutMeta) {
	if c == nil {
		return
	}
	if meta.ChannelID == 0 {
		meta.ChannelID = c.GetInt("channel_id")
	}
	if meta.RequestPath == "" && c.Request != nil && c.Request.URL != nil {
		meta.RequestPath = c.Request.URL.Path
	}
	meta.RetryIndex = c.GetInt("retry")
	c.Set(string(constant.ContextKeyTimeoutMeta), meta)
}

func AppendTimeoutMeta(other map[string]interface{}, c *gin.Context) map[string]interface{} {
	if other == nil {
		other = make(map[string]interface{})
	}
	if c == nil {
		return other
	}
	raw, ok := c.Get(string(constant.ContextKeyTimeoutMeta))
	if !ok {
		return other
	}
	meta, ok := raw.(TimeoutMeta)
	if !ok {
		return other
	}
	other["timeout_type"] = meta.Type
	other["timeout_source"] = meta.Source
	other["timeout_seconds"] = meta.Seconds
	other["timeout_provider"] = meta.Provider
	other["timeout_stage"] = meta.Stage
	other["retry_index"] = meta.RetryIndex
	other["is_stream"] = meta.IsStream
	return other
}

func GetTimeoutMeta(c *gin.Context) (TimeoutMeta, bool) {
	if c == nil {
		return TimeoutMeta{}, false
	}
	raw, ok := c.Get(string(constant.ContextKeyTimeoutMeta))
	if !ok {
		return TimeoutMeta{}, false
	}
	meta, ok := raw.(TimeoutMeta)
	return meta, ok
}

func SetTimeoutTrace(c *gin.Context, trace TimeoutTrace) {
	if c == nil {
		return
	}
	if trace.WrapReason == "" {
		trace.WrapReason = TimeoutWrapReasonNone
	}
	c.Set(string(constant.ContextKeyTimeoutTrace), trace)
}

func GetTimeoutTrace(c *gin.Context) (TimeoutTrace, bool) {
	if c == nil {
		return TimeoutTrace{}, false
	}
	raw, ok := c.Get(string(constant.ContextKeyTimeoutTrace))
	if !ok {
		return TimeoutTrace{}, false
	}
	trace, ok := raw.(TimeoutTrace)
	return trace, ok
}

func ClearTimeoutTrace(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(string(constant.ContextKeyTimeoutTrace), TimeoutTrace{})
	rootcommon.SetContextKey(c, constant.ContextKeyTimeoutWrapReason, TimeoutWrapReasonNone)
}

func CaptureTimeoutTrace(c *gin.Context, info *RelayInfo, attemptStart time.Time, attemptEnd time.Time, budgetExceeded bool, relayErr error) TimeoutTrace {
	trace := TimeoutTrace{}
	meta, ok := GetTimeoutMeta(c)
	if ok {
		trace.Type = meta.Type
		trace.Source = meta.Source
		trace.Stage = meta.Stage
		trace.Provider = meta.Provider
		trace.ConfiguredTimeoutSeconds = meta.Seconds
	}
	if !attemptStart.IsZero() {
		trace.AttemptStartedAt = attemptStart
	}
	if !attemptEnd.IsZero() {
		trace.AttemptEndedAt = attemptEnd
	}
	if !attemptStart.IsZero() && !attemptEnd.IsZero() && attemptEnd.After(attemptStart) {
		trace.ActualElapsed = attemptEnd.Sub(attemptStart)
	}
	if info != nil {
		trace.GroupBudget = info.StreamRetryFirstByteBudget
		trace.GroupBudgetSpent = info.StreamRetryFirstByteWaitSpent
		if trace.GroupBudget > trace.GroupBudgetSpent {
			trace.GroupBudgetRemaining = trace.GroupBudget - trace.GroupBudgetSpent
		}
		if info.IsStream && trace.ActualElapsed <= 0 && info.AttemptStreamFirstByteTimeout > 0 {
			trace.ActualElapsed = info.AttemptStreamFirstByteTimeout
		}
	}
	switch {
	case budgetExceeded:
		trace.WrapReason = TimeoutWrapReasonGroupBudgetExceeded
	case meta.Source == TimeoutSourceChannelSetting && (errors.Is(relayErr, context.DeadlineExceeded) || errors.Is(relayErr, ErrStreamFirstByteTimeout)):
		trace.WrapReason = TimeoutWrapReasonChannelSetting
	default:
		trace.WrapReason = TimeoutWrapReasonNone
	}
	SetTimeoutTrace(c, trace)
	if c != nil {
		rootcommon.SetContextKey(c, constant.ContextKeyTimeoutWrapReason, trace.WrapReason)
	}
	return trace
}

func AppendTimeoutTrace(other map[string]interface{}, c *gin.Context) map[string]interface{} {
	if other == nil {
		other = make(map[string]interface{})
	}
	trace, ok := GetTimeoutTrace(c)
	if !ok {
		return other
	}
	if !trace.AttemptStartedAt.IsZero() {
		other["timeout_attempt_started_at_unix_ms"] = trace.AttemptStartedAt.UnixMilli()
	}
	if !trace.AttemptEndedAt.IsZero() {
		other["timeout_attempt_ended_at_unix_ms"] = trace.AttemptEndedAt.UnixMilli()
	}
	if trace.ConfiguredTimeoutSeconds > 0 {
		other["configured_timeout_seconds"] = trace.ConfiguredTimeoutSeconds
	}
	if trace.ActualElapsed > 0 {
		other["timeout_actual_elapsed_ms"] = trace.ActualElapsed.Milliseconds()
	}
	if trace.GroupBudget > 0 {
		other["group_budget_seconds"] = int(trace.GroupBudget / time.Second)
	}
	if trace.GroupBudgetSpent > 0 {
		other["group_budget_spent_ms"] = trace.GroupBudgetSpent.Milliseconds()
	}
	if trace.GroupBudgetRemaining >= 0 && trace.GroupBudget > 0 {
		other["group_budget_remaining_ms"] = trace.GroupBudgetRemaining.Milliseconds()
	}
	if trace.WrapReason != "" {
		other["timeout_wrap_reason"] = trace.WrapReason
	}
	return other
}

type FirstByteTimeoutController struct {
	timer    *time.Timer
	cancel   context.CancelFunc
	timedOut atomic.Bool
	stopOnce sync.Once
}

func NewFirstByteTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, *FirstByteTimeoutController) {
	ctx, cancel := context.WithCancel(parent)
	controller := &FirstByteTimeoutController{
		cancel: cancel,
	}
	if timeout <= 0 {
		return ctx, controller
	}
	controller.timer = time.AfterFunc(timeout, func() {
		controller.timedOut.Store(true)
		cancel()
	})
	return ctx, controller
}

func (c *FirstByteTimeoutController) StopWaiting() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if c.timer != nil {
			c.timer.Stop()
		}
	})
}

func (c *FirstByteTimeoutController) Cancel() {
	if c == nil {
		return
	}
	c.StopWaiting()
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *FirstByteTimeoutController) TimeoutTriggered() bool {
	if c == nil {
		return false
	}
	return c.timedOut.Load()
}

func (c *FirstByteTimeoutController) WrapBody(body io.ReadCloser, timeoutSeconds int) io.ReadCloser {
	if body == nil || c == nil {
		return body
	}
	// 这里包装响应体的目的，是把“首包等待超时”准确映射到第一次 Read，
	// 这样上层日志和错误码可以稳定识别为流式首包超时，而不是普通 EOF/取消。
	return &firstByteTimeoutBody{
		ReadCloser:     body,
		controller:     c,
		timeoutSeconds: timeoutSeconds,
	}
}

type firstByteTimeoutBody struct {
	io.ReadCloser
	controller     *FirstByteTimeoutController
	timeoutSeconds int
}

func (b *firstByteTimeoutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.controller.StopWaiting()
		return n, err
	}
	if err != nil && b.controller.TimeoutTriggered() {
		return n, fmt.Errorf("%w after %d seconds", ErrStreamFirstByteTimeout, b.timeoutSeconds)
	}
	return n, err
}

func (b *firstByteTimeoutBody) Close() error {
	if b.controller != nil {
		b.controller.StopWaiting()
	}
	return b.ReadCloser.Close()
}
