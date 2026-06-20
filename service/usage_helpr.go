package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

//func GetPromptTokens(textRequest dto.GeneralOpenAIRequest, relayMode int) (int, error) {
//	switch relayMode {
//	case constant.RelayModeChatCompletions:
//		return CountTokenMessages(textRequest.Messages, textRequest.Model)
//	case constant.RelayModeCompletions:
//		return CountTokenInput(textRequest.Prompt, textRequest.Model), nil
//	case constant.RelayModeModerations:
//		return CountTokenInput(textRequest.Input, textRequest.Model), nil
//	}
//	return 0, errors.New("unknown relay mode")
//}

func ResponseText2Usage(c *gin.Context, responseText string, modeName string, promptTokens int) *dto.Usage {
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{}
	usage.PromptTokens = promptTokens
	usage.CompletionTokens = EstimateTokenByModel(modeName, responseText)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.InputTokens = usage.PromptTokens
	usage.OutputTokens = usage.CompletionTokens
	usage.UsageSource = common.UpstreamUsageSourceEstimated
	return usage
}

func ValidUsage(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}

func BuildUpstreamUsageLog(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) map[string]any {
	result := map[string]any{}
	if relayInfo != nil {
		result["provider"] = relayInfo.GetFinalRequestRelayFormat()
		result["model"] = relayInfo.UpstreamModelName
	}
	if usage == nil {
		result["source"] = common.UpstreamUsageSourceMissing
		result["complete"] = false
		return result
	}
	normalizedUsage := *usage
	result["source"] = normalizedUpstreamUsageSource(&normalizedUsage)
	if normalizedUsage.UsageSource != "" {
		result["usage_source"] = normalizedUsage.UsageSource
	}
	if usage.UsageSource != "" && usage.UsageSource != normalizedUsage.UsageSource {
		result["provider_usage_source"] = usage.UsageSource
	}
	result["complete"] = true
	if relayInfo != nil && relayInfo.IsStream && relayInfo.StreamStatus != nil && (!relayInfo.StreamStatus.IsNormalEnd() || relayInfo.StreamStatus.HasErrors()) {
		result["complete"] = false
		result["stream_interrupted"] = true
		result["stream_end_reason"] = string(relayInfo.StreamStatus.EndReason)
		if relayInfo.StreamStatus.EndError != nil {
			result["stream_end_error"] = relayInfo.StreamStatus.EndError.Error()
		}
	}
	raw, err := common.Marshal(normalizedUsage)
	if err == nil {
		var payload map[string]any
		if err = common.Unmarshal(raw, &payload); err == nil {
			result["raw"] = payload
		}
	}
	return result
}

func AttachConsumeLogUpstreamUsage(c *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) map[string]any {
	result := BuildUpstreamUsageLog(relayInfo, usage)
	common.SetConsumeLogUpstreamUsage(c, result)
	return result
}

func normalizedUpstreamUsageSource(usage *dto.Usage) string {
	if usage == nil {
		return common.UpstreamUsageSourceMissing
	}
	switch usage.UsageSource {
	case "":
		usage.UsageSource = common.UpstreamUsageSourceProviderNative
		return common.UpstreamUsageSourceProviderNative
	case common.UpstreamUsageSourceEstimated:
		return common.UpstreamUsageSourceEstimated
	case common.UpstreamUsageSourceProviderNative, common.UpstreamUsageSourceProviderTransformed:
		return usage.UsageSource
	default:
		usage.UsageSource = common.UpstreamUsageSourceProviderTransformed
		return common.UpstreamUsageSourceProviderTransformed
	}
}
