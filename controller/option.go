package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

func isPaymentComplianceOptionKey(key string) bool {
	return strings.HasPrefix(key, "payment_setting.compliance_")
}

func isPositiveOptionValue(value string) bool {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return intValue > 0
	}
	floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && floatValue > 0
}

func parseBoundedIntOption(raw string, min int, max int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if value < min || value > max {
		return 0, fmt.Errorf("out of range")
	}
	return value, nil
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		isSensitiveKey := strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			strings.HasSuffix(k, "Key") ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key")
		if isSensitiveKey {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type OptionBulkUpdateRequest struct {
	Options []OptionUpdateRequest `json:"options"`
}

type optionValidationResult struct {
	message string
	i18nKey string
}

func normalizeOptionValue(value any) string {
	switch typedValue := value.(type) {
	case bool:
		return common.Interface2String(typedValue)
	case float64:
		return common.Interface2String(typedValue)
	case int:
		return common.Interface2String(typedValue)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func validateOptionUpdate(option OptionUpdateRequest) *optionValidationResult {
	fail := func(message string) *optionValidationResult {
		return &optionValidationResult{message: message}
	}
	failI18n := func(key string) *optionValidationResult {
		return &optionValidationResult{i18nKey: key}
	}
	switch option.Key {
	case "QuotaForInviter", "QuotaForInvitee":
		if isPositiveOptionValue(option.Value.(string)) && !operation_setting.IsPaymentComplianceConfirmed() {
			return failI18n(i18n.MsgPaymentComplianceRequired)
		}
	default:
		if isPaymentComplianceOptionKey(option.Key) {
			return fail("合规确认字段不允许通过通用设置接口修改")
		}
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			return fail("无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！")
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			return fail("无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！")
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			return fail("无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！")
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			return fail("无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！")
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			return fail("无法启用邮箱域名限制，请先填入限制的邮箱域名！")
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			return fail("无法启用微信登录，请先填入微信登录相关配置信息！")
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			return fail("无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！")
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			return fail("无法启用 Telegram OAuth，请先填入 Telegram Bot Token！")
		}
	case "theme.frontend":
		if option.Value != "default" && option.Value != "classic" {
			return fail("无效的主题值，可选值：default（新版前端）、classic（经典前端）")
		}
	case "ArchiveBackend":
		if option.Value != "local" && option.Value != "azure_blob" {
			return fail("ArchiveBackend 仅支持 local 或 azure_blob")
		}
	case "ArchiveQueueSize":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 1000000); err != nil {
			return fail("ArchiveQueueSize 必须位于 1 到 1000000 之间")
		}
	case "ArchiveWorkerCount":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 1024); err != nil {
			return fail("ArchiveWorkerCount 必须位于 1 到 1024 之间")
		}
	case "ArchiveMaxRequestMB", "ArchiveMaxResponseMB", "ArchiveSegmentMaxMB":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 10240); err != nil {
			return fail(fmt.Sprintf("%s 必须位于 1 到 10240 之间", option.Key))
		}
	case "ArchiveSpoolTTLHours":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 24*365); err != nil {
			return fail("ArchiveSpoolTTLHours 必须位于 1 到 8760 之间")
		}
	case "ArchiveSmallPayloadMaxKB":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 1024*1024); err != nil {
			return fail("ArchiveSmallPayloadMaxKB 必须位于 1 到 1048576 之间")
		}
	case "ArchiveSegmentMaxAgeSeconds":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 86400); err != nil {
			return fail("ArchiveSegmentMaxAgeSeconds 必须位于 1 到 86400 之间")
		}
	case "ArchiveSegmentMaxRecords":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 1000000); err != nil {
			return fail("ArchiveSegmentMaxRecords 必须位于 1 到 1000000 之间")
		}
	case "ArchiveSegmentShardCount":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 4096); err != nil {
			return fail("ArchiveSegmentShardCount 必须位于 1 到 4096 之间")
		}
	case "ArchiveHeaderValueMaxLength":
		if _, err := parseBoundedIntOption(option.Value.(string), 0, 65535); err != nil {
			return fail("ArchiveHeaderValueMaxLength 必须位于 0 到 65535 之间")
		}
	case "ArchiveSamplePercent", "ArchiveMaxCPUPercent", "ArchiveMaxMemoryPercent", "ArchiveMinFreeDiskPercent":
		if _, err := parseBoundedIntOption(option.Value.(string), 0, 100); err != nil {
			return fail(fmt.Sprintf("%s 必须位于 0 到 100 之间", option.Key))
		}
	case "ArchiveMinFreeDiskGB":
		if _, err := parseBoundedIntOption(option.Value.(string), 0, 1048576); err != nil {
			return fail("ArchiveMinFreeDiskGB 必须位于 0 到 1048576 之间")
		}
	case "ArchiveLoadCheckIntervalSeconds":
		if _, err := parseBoundedIntOption(option.Value.(string), 1, 3600); err != nil {
			return fail("ArchiveLoadCheckIntervalSeconds 必须位于 1 到 3600 之间")
		}
	case "GroupRatio":
		if err := ratio_setting.CheckGroupRatio(option.Value.(string)); err != nil {
			return fail(err.Error())
		}
	case "group_strategy_settings":
		if err := operation_setting.ValidateGroupStrategySettings(option.Value.(string)); err != nil {
			return fail(err.Error())
		}
	case "ClientTimeoutResponseHttpStatus":
		status, convErr := strconv.Atoi(strings.TrimSpace(option.Value.(string)))
		if convErr != nil || status < 0 || status > 599 || (status > 0 && status < 100) {
			return fail("渠道超时响应给客户的状态码必须为空或位于 100 到 599 之间")
		}
	case "ImageRatio":
		if err := ratio_setting.UpdateImageRatioByJSONString(option.Value.(string)); err != nil {
			return fail("图片倍率设置失败: " + err.Error())
		}
	case "AudioRatio":
		if err := ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string)); err != nil {
			return fail("音频倍率设置失败: " + err.Error())
		}
	case "AudioCompletionRatio":
		if err := ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string)); err != nil {
			return fail("音频补全倍率设置失败: " + err.Error())
		}
	case "CreateCacheRatio":
		if err := ratio_setting.UpdateCreateCacheRatioByJSONString(option.Value.(string)); err != nil {
			return fail("缓存创建倍率设置失败: " + err.Error())
		}
	case "ModelRequestRateLimitGroup":
		if err := setting.CheckModelRequestRateLimitGroup(option.Value.(string)); err != nil {
			return fail(err.Error())
		}
	case "AutomaticDisableStatusCodes":
		if _, err := operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string)); err != nil {
			return fail(err.Error())
		}
	case "AutomaticRetryStatusCodes":
		if _, err := operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string)); err != nil {
			return fail(err.Error())
		}
	case "console_setting.api_info":
		if err := console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo"); err != nil {
			return fail(err.Error())
		}
	case "console_setting.announcements":
		if err := console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements"); err != nil {
			return fail(err.Error())
		}
	case "console_setting.faq":
		if err := console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ"); err != nil {
			return fail(err.Error())
		}
	case "console_setting.uptime_kuma_groups":
		if err := console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups"); err != nil {
			return fail(err.Error())
		}
	}
	return nil
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &option); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	option.Value = normalizeOptionValue(option.Value)
	if validation := validateOptionUpdate(option); validation != nil {
		if validation.i18nKey != "" {
			common.ApiErrorI18n(c, validation.i18nKey)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": false, "message": validation.message})
		return
	}
	err := model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": option.Key,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateOptionsBulk(c *gin.Context) {
	var request OptionBulkUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if len(request.Options) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "未提供任何配置项",
		})
		return
	}

	values := make(map[string]string, len(request.Options))
	keys := make([]string, 0, len(request.Options))
	for _, option := range request.Options {
		option.Value = normalizeOptionValue(option.Value)
		if _, exists := values[option.Key]; exists {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "批量请求中存在重复配置项: " + option.Key,
			})
			return
		}
		if validation := validateOptionUpdate(option); validation != nil {
			if validation.i18nKey != "" {
				common.ApiErrorI18n(c, validation.i18nKey)
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": validation.message})
			return
		}
		values[option.Key] = option.Value.(string)
		keys = append(keys, option.Key)
	}

	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "option.update_bulk", map[string]interface{}{
		"count": len(keys),
		"keys":  keys,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
