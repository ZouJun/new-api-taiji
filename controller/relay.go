package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/archive"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		relayInfo   *relaycommon.RelayInfo
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			responseErr := relaycommon.BuildFinalTimeoutResponseError(c, relayInfo, newAPIError)
			rawClientMessage := responseErr.Error()
			clientMessage := rawClientMessage
			if responseErr == newAPIError && shouldHideInternalRelayError(c, newAPIError) {
				clientMessage = "upstream error: do request failed"
				rawClientMessage = clientMessage
			}
			responseErr.SetMessage(common.MessageWithRequestId(clientMessage, requestId))
			recordUnhandledRelayError(c, relayInfo, newAPIError)
			recordFinalRelayError(c, relayInfo, newAPIError, responseErr, rawClientMessage)
			logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(newAPIError.Error())))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, responseErr.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(responseErr.StatusCode, gin.H{
					"type":  "error",
					"error": responseErr.ToClaudeError(),
				})
			default:
				c.JSON(responseErr.StatusCode, gin.H{
					"error": responseErr.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err = relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	for {
		relayInfo.RetryIndex = retryParam.GetRetry()
		c.Set("retry", relayInfo.RetryIndex)
		attemptStartedAt := time.Now()
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			addArchiveRelayAttempt(c, relayInfo.RetryIndex, nil, "failed", attemptStartedAt, channelErr)
			break
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)
		relayInfo.BeginAttempt()
		attemptStart := time.Now()

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			relayInfo.LastError = nil
			relaycommon.ClearTimeoutTrace(c)
			addArchiveRelayAttempt(c, relayInfo.RetryIndex, channel, "success", attemptStartedAt, nil)
			return
		}

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		timeoutTrace := relaycommon.CaptureTimeoutTrace(c, relayInfo, attemptStart, time.Now(), false, newAPIError)
		addArchiveRelayAttempt(c, relayInfo.RetryIndex, channel, "failed", attemptStartedAt, newAPIError)

		budgetExceeded := consumeStreamFirstByteRetryBudget(c, relayInfo, attemptStart, newAPIError)
		if budgetExceeded {
			timeoutTrace = relaycommon.CaptureTimeoutTrace(c, relayInfo, attemptStart, time.Now(), true, newAPIError)
		}
		effectiveRetryTimes := relaycommon.ResolveEffectiveRetryTimes(c, relayInfo)
		remainingRetrySlots := effectiveRetryTimes - retryParam.GetRetry()
		willRetry := !budgetExceeded && shouldRetry(c, newAPIError, remainingRetrySlots)
		stopReason := resolveRelayRetryStopReason(c, newAPIError, budgetExceeded, willRetry, remainingRetrySlots)
		decision := relaycommon.RelayRetryDecision{
			WillRetry:           willRetry,
			StopReason:          stopReason,
			EffectiveRetryTimes: effectiveRetryTimes,
			RemainingRetrySlots: remainingRetrySlots,
		}
		processChannelError(
			c,
			*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
			relayInfo,
			newAPIError,
			decision,
		)
		logRelayTimeoutTrace(c, relayInfo, timeoutTrace, budgetExceeded)
		logger.LogInfo(c, fmt.Sprintf("relay retry decision: will_retry=%t stop_reason=%s retry_index=%d remaining_retry_slots=%d used_channels=%v", willRetry, stopReason, retryParam.GetRetry(), remainingRetrySlots, c.GetStringSlice("use_channel")))

		if budgetExceeded {
			break
		}

		if !willRetry {
			break
		}
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

func logRelayTimeoutTrace(c *gin.Context, relayInfo *relaycommon.RelayInfo, trace relaycommon.TimeoutTrace, budgetExceeded bool) {
	if trace.Type == "" && !budgetExceeded {
		return
	}
	group := ""
	channelID := 0
	if relayInfo != nil {
		group = relayInfo.GroupStrategyGroup
		channelID = relayInfo.ChannelId
	}
	logger.LogWarn(
		c,
		fmt.Sprintf(
			"timeout trace: type=%s source=%s stage=%s configured_seconds=%d actual_elapsed_ms=%d wrap_reason=%s budget_exceeded=%t group=%s group_budget_seconds=%d group_budget_spent_ms=%d group_budget_remaining_ms=%d channel_id=%d retry_index=%d",
			trace.Type,
			trace.Source,
			trace.Stage,
			trace.ConfiguredTimeoutSeconds,
			trace.ActualElapsed.Milliseconds(),
			trace.WrapReason,
			budgetExceeded,
			group,
			int(trace.GroupBudget/time.Second),
			trace.GroupBudgetSpent.Milliseconds(),
			trace.GroupBudgetRemaining.Milliseconds(),
			channelID,
			c.GetInt("retry"),
		),
	)
}

func shouldHideInternalRelayError(c *gin.Context, err *types.NewAPIError) bool {
	return relaycommon.IsTimeoutFeatureRelayError(c, err)
}

func recordUnhandledRelayError(c *gin.Context, relayInfo *relaycommon.RelayInfo, err *types.NewAPIError) {
	if err == nil || !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) || relaycommon.IsRelayErrorLogged(c) {
		return
	}
	other := relaycommon.BuildRelayErrorTrace(c, relayInfo, err, relaycommon.RelayRetryDecision{
		WillRetry:           false,
		StopReason:          "request_terminated_before_channel_retry",
		EffectiveRetryTimes: relaycommon.ResolveEffectiveRetryTimes(c, relayInfo),
	})
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	other["error_type"] = err.GetErrorType()
	other["error_code"] = err.GetErrorCode()
	other["status_code"] = err.StatusCode
	other["log_phase"] = "pre_channel_or_unhandled_error"
	other["log_event"] = "relay_error"
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(
		c,
		c.GetInt("id"),
		c.GetInt("channel_id"),
		c.GetString("original_model"),
		c.GetString("token_name"),
		err.MaskSensitiveErrorWithStatusCode(),
		c.GetInt("token_id"),
		useTimeSeconds,
		common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		c.GetString("group"),
		other,
	)
	relaycommon.MarkRelayErrorLogged(c)
}

func recordFinalRelayError(c *gin.Context, relayInfo *relaycommon.RelayInfo, upstreamErr *types.NewAPIError, responseErr *types.NewAPIError, rawClientMessage string) {
	if upstreamErr == nil || responseErr == nil || !constant.ErrorLogEnabled || !types.IsRecordErrorLog(upstreamErr) || relaycommon.IsRelayFinalErrorLogged(c) {
		return
	}
	effectiveRetryTimes := relaycommon.ResolveEffectiveRetryTimes(c, relayInfo)
	finalDecision := relaycommon.RelayRetryDecision{
		WillRetry:           false,
		StopReason:          "final_response_returned",
		EffectiveRetryTimes: effectiveRetryTimes,
		RemainingRetrySlots: effectiveRetryTimes - c.GetInt("retry"),
	}
	other := relaycommon.BuildRelayErrorTrace(c, relayInfo, upstreamErr, finalDecision)
	if c.Request != nil && c.Request.URL != nil {
		other["request_path"] = c.Request.URL.Path
	}
	other["error_type"] = upstreamErr.GetErrorType()
	other["error_code"] = upstreamErr.GetErrorCode()
	other["status_code"] = upstreamErr.StatusCode
	other["final_error"] = true
	other["log_phase"] = "final_response_error"
	other["log_event"] = "relay_final_error"
	other["final_response_status_code"] = responseErr.StatusCode
	other["final_response_message"] = rawClientMessage
	other["final_response_message_with_request_id"] = responseErr.ClientMessage()
	other["final_response_wrapped"] = responseErr != upstreamErr
	other["upstream_error_message"] = upstreamErr.MaskSensitiveErrorWithStatusCode()
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(
		c,
		c.GetInt("id"),
		c.GetInt("channel_id"),
		c.GetString("original_model"),
		c.GetString("token_name"),
		responseErr.MaskSensitiveErrorWithStatusCode(),
		c.GetInt("token_id"),
		useTimeSeconds,
		common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		c.GetString("group"),
		other,
	)
	relaycommon.MarkRelayFinalErrorLogged(c)
}

func consumeStreamFirstByteRetryBudget(c *gin.Context, relayInfo *relaycommon.RelayInfo, attemptStart time.Time, relayErr *types.NewAPIError) bool {
	if relayInfo == nil || !relayInfo.IsStream {
		return false
	}
	_, _, budgetExceeded := relaycommon.ConsumeStreamFirstByteRetryBudget(
		c,
		relayInfo,
		relaycommon.ResolveAttemptStreamFirstByteWait(relayInfo, attemptStart, relayErr),
	)
	return budgetExceeded
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	if selectGroup != "" {
		info.UsingGroup = selectGroup
	}
	relaycommon.SnapshotRuntimeGroupStrategy(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return retryTimes > 0
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if errors.Is(openaiErr, context.Canceled) {
		return false
	}
	if errors.Is(openaiErr, context.DeadlineExceeded) || errors.Is(openaiErr, relaycommon.ErrStreamFirstByteTimeout) {
		return true
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, relayInfo *relaycommon.RelayInfo, err *types.NewAPIError, decision relaycommon.RelayRetryDecision) {
	headerSnapshot, _ := common.BuildArchiveRequestHeaderSnapshot(c.Request.Header, common.ArchiveHeaderValueMaxLength)
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s request_header=%s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error()), common.FormatNonEmptyArchiveRequestHeaderSnapshot(headerSnapshot)))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		other["log_phase"] = "channel_attempt_error"
		other["log_event"] = "relay_channel_error"
		for key, value := range relaycommon.BuildRelayErrorTrace(c, relayInfo, err, decision) {
			other[key] = value
		}
		other = relaycommon.AppendTimeoutMeta(other, c)
		other = relaycommon.AppendTimeoutTrace(other, c)
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
		relaycommon.MarkRelayErrorLogged(c)
	}

}

func resolveRelayRetryStopReason(c *gin.Context, err *types.NewAPIError, budgetExceeded bool, willRetry bool, remainingRetrySlots int) string {
	switch {
	case budgetExceeded:
		return "stream_first_byte_budget_exhausted"
	case err == nil:
		return "no_error"
	case err.StatusCode == 499:
		return "client_disconnected"
	case willRetry:
		return "retry_next_channel"
	case errors.Is(err, context.Canceled):
		if relaycommon.IsTimeoutFeatureRelayError(c, err) {
			return "timeout_context_canceled"
		}
		return "context_canceled"
	case remainingRetrySlots <= 0:
		return "retry_limit_reached"
	case types.IsSkipRetryError(err):
		return "skip_retry_error"
	case types.IsChannelError(err):
		return "channel_not_retryable"
	default:
		if _, ok := c.Get("specific_channel_id"); ok {
			return "specific_channel_no_retry"
		}
		return "retry_policy_rejected"
	}
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		c.Set("retry", retryParam.GetRetry())
		attemptStartedAt := time.Now()
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					addArchiveTaskAttempt(c, retryParam.GetRetry(), channel, "failed", attemptStartedAt, taskErr)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				addArchiveTaskAttempt(c, retryParam.GetRetry(), nil, "failed", attemptStartedAt, taskErr)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			addArchiveTaskAttempt(c, retryParam.GetRetry(), channel, "success", attemptStartedAt, nil)
			break
		}
		addArchiveTaskAttempt(c, retryParam.GetRetry(), channel, "failed", attemptStartedAt, taskErr)

		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				relayInfo,
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode),
				relaycommon.RelayRetryDecision{})
		}

		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func addArchiveRelayAttempt(c *gin.Context, index int, channel *model.Channel, outcome string, startedAt time.Time, err *types.NewAPIError) {
	state, ok := archive.FromContext(c)
	if !ok || state == nil {
		return
	}
	attempt := archive.AttemptSummary{
		Index:      index,
		Outcome:    outcome,
		StartedAt:  startedAt.UTC().UnixMilli(),
		DurationMs: time.Since(startedAt).Milliseconds(),
	}
	if channel != nil {
		attempt.ChannelID = channel.Id
		attempt.ChannelName = channel.Name
		attempt.ChannelType = channel.Type
	}
	if err != nil {
		attempt.HTTPStatus = err.StatusCode
		attempt.ErrorCode = string(err.GetErrorCode())
		attempt.ErrorMessage = common.LocalLogPreview(err.Error())
	}
	attempt.TimeoutReason = archiveTimeoutReason(c)
	state.AddAttempt(attempt)
}

func addArchiveTaskAttempt(c *gin.Context, index int, channel *model.Channel, outcome string, startedAt time.Time, taskErr *dto.TaskError) {
	state, ok := archive.FromContext(c)
	if !ok || state == nil {
		return
	}
	attempt := archive.AttemptSummary{
		Index:      index,
		Outcome:    outcome,
		StartedAt:  startedAt.UTC().UnixMilli(),
		DurationMs: time.Since(startedAt).Milliseconds(),
	}
	if channel != nil {
		attempt.ChannelID = channel.Id
		attempt.ChannelName = channel.Name
		attempt.ChannelType = channel.Type
	}
	if taskErr != nil {
		attempt.HTTPStatus = taskErr.StatusCode
		attempt.ErrorCode = taskErr.Code
		if taskErr.Error != nil {
			attempt.ErrorMessage = common.LocalLogPreview(taskErr.Error.Error())
		} else {
			attempt.ErrorMessage = common.LocalLogPreview(taskErr.Message)
		}
	}
	attempt.TimeoutReason = archiveTimeoutReason(c)
	state.AddAttempt(attempt)
}

func archiveTimeoutReason(c *gin.Context) string {
	raw, ok := c.Get(string(constant.ContextKeyTimeoutMeta))
	if !ok {
		return ""
	}
	meta, ok := raw.(relaycommon.TimeoutMeta)
	if !ok {
		return ""
	}
	parts := []string{}
	if meta.Type != "" {
		parts = append(parts, meta.Type)
	}
	if meta.Stage != "" {
		parts = append(parts, meta.Stage)
	}
	if meta.Source != "" {
		parts = append(parts, meta.Source)
	}
	if meta.Seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", meta.Seconds))
	}
	return strings.Join(parts, ":")
}
