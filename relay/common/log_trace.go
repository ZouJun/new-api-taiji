package common

import (
	"context"
	"errors"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func BuildRelayErrorTrace(c *gin.Context, info *RelayInfo, err *types.NewAPIError, decision RelayRetryDecision) map[string]interface{} {
	trace := map[string]interface{}{}
	appendFailureTrace(trace, c, err)
	appendRetryTrace(trace, c, info, decision)
	return trace
}

func AppendRelayTrace(other map[string]interface{}, c *gin.Context, info *RelayInfo, decision RelayRetryDecision) map[string]interface{} {
	if other == nil {
		other = make(map[string]interface{})
	}
	appendRetryTrace(other, c, info, decision)
	return other
}

type RelayRetryDecision struct {
	WillRetry           bool
	StopReason          string
	EffectiveRetryTimes int
	RemainingRetrySlots int
}

func appendFailureTrace(other map[string]interface{}, c *gin.Context, err *types.NewAPIError) {
	if other == nil || err == nil {
		return
	}
	source, category := resolveRelayFailureSource(c, err)
	other["failure_source"] = source
	other["failure_category"] = category
	if err.StatusCode == 499 {
		other["client_disconnect"] = true
	}
	if errors.Is(err, context.Canceled) {
		other["context_canceled"] = true
		other["cancel_origin"] = inferCancelOrigin(c, err)
		if c != nil && c.Request != nil {
			if reqErr := c.Request.Context().Err(); reqErr != nil {
				other["request_context_err"] = reqErr.Error()
			}
		}
		if meta, ok := GetTimeoutMeta(c); ok {
			other["cancel_with_timeout_meta"] = true
			other["cancel_timeout_type"] = meta.Type
			other["cancel_timeout_stage"] = meta.Stage
		}
	}
	if err.StatusCode >= 400 && err.StatusCode <= 599 {
		other["http_status_family"] = err.StatusCode / 100
	}
}

func appendRetryTrace(other map[string]interface{}, c *gin.Context, info *RelayInfo, decision RelayRetryDecision) {
	if other == nil {
		return
	}
	usedChannels := []string(nil)
	if c != nil {
		usedChannels = c.GetStringSlice("use_channel")
	}
	other["retry_index"] = 0
	if c != nil {
		other["retry_index"] = c.GetInt("retry")
	}
	other["retry_channel_count"] = len(usedChannels)
	if len(usedChannels) > 0 {
		other["retry_channels"] = usedChannels
	}
	other["will_retry"] = decision.WillRetry
	if decision.StopReason != "" {
		other["retry_stop_reason"] = decision.StopReason
	}
	other["effective_retry_times"] = decision.EffectiveRetryTimes
	other["remaining_retry_slots"] = decision.RemainingRetrySlots

	if info == nil {
		return
	}
	other["group_strategy_group"] = info.GroupStrategyGroup
	other["group_strategy_matched"] = info.GroupStrategyMatched
	if info.StreamRetryFirstByteBudget > 0 {
		other["stream_retry_first_byte_budget_seconds"] = int(info.StreamRetryFirstByteBudget / time.Second)
	}
	if info.StreamRetryFirstByteWaitSpent > 0 {
		other["stream_retry_first_byte_wait_spent_ms"] = info.StreamRetryFirstByteWaitSpent.Milliseconds()
	}
	if info.AttemptStreamFirstByteTimeout > 0 {
		other["attempt_stream_first_byte_timeout_ms"] = info.AttemptStreamFirstByteTimeout.Milliseconds()
	}
}

func inferCancelOrigin(c *gin.Context, err *types.NewAPIError) string {
	if err == nil {
		return "unknown"
	}
	if err.StatusCode == 499 {
		return "client_request_context"
	}
	if IsTimeoutFeatureRelayError(c, err) {
		return "timeout_context"
	}
	if c != nil && c.Request != nil && c.Request.Context().Err() == context.Canceled {
		return "client_request_context"
	}
	return "unknown_context"
}

func resolveRelayFailureSource(c *gin.Context, err *types.NewAPIError) (string, string) {
	if err == nil {
		return "unknown", "unknown"
	}
	switch {
	case err.StatusCode == 499:
		return "client", "client_disconnect"
	case errors.Is(err, context.Canceled):
		if IsTimeoutFeatureRelayError(c, err) {
			return "timeout_feature", "timeout_context_canceled"
		}
		return "internal", "context_canceled"
	case IsTimeoutFeatureRelayError(c, err):
		return "timeout_feature", "timeout"
	case err.GetErrorCode() == types.ErrorCodeBadResponseStatusCode ||
		err.GetErrorType() == types.ErrorTypeOpenAIError ||
		err.GetErrorType() == types.ErrorTypeClaudeError ||
		err.GetErrorType() == types.ErrorTypeUpstreamError:
		return "upstream", "upstream_non_2xx"
	case err.GetErrorCode() == types.ErrorCodeInvalidRequest ||
		err.GetErrorCode() == types.ErrorCodeReadRequestBodyFailed ||
		err.GetErrorCode() == types.ErrorCodeBadRequestBody:
		return "client", "invalid_request"
	default:
		return "internal", "internal_error"
	}
}

func MarkRelayErrorLogged(c *gin.Context) {
	if c == nil {
		return
	}
	rootcommon.SetContextKey(c, constant.ContextKeyRelayErrorLogged, true)
}

func IsRelayErrorLogged(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return rootcommon.GetContextKeyBool(c, constant.ContextKeyRelayErrorLogged)
}

func MarkRelayFinalErrorLogged(c *gin.Context) {
	if c == nil {
		return
	}
	rootcommon.SetContextKey(c, constant.ContextKeyRelayFinalErrorLogged, true)
}

func IsRelayFinalErrorLogged(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return rootcommon.GetContextKeyBool(c, constant.ContextKeyRelayFinalErrorLogged)
}
