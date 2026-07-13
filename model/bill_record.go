package model

import (
	"encoding/base64"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

const (
	BillSupplierNameOptionKey    = "BillSupplierName"
	BillSiteURLOptionKey         = "BillSiteURL"
	BillAccessTokenOptionKey     = "BillAccessToken"
	BillDiscountOptionKey        = "BillDiscount"
	BillPricingCurrencyOptionKey = "BillPricingCurrency"
	ClientRequestIDHeader        = "X-Client-Request-Id"
	billRecordTableComment       = "对外账单调用明细表：成功调用并产生计费日志时平铺固化计量与定价快照，用于按天账单汇总和逐笔调用明细查询"
)

var shanghaiLocation = time.FixedZone("CST", 8*3600)

type BillRecord struct {
	Id                            int     `json:"id" gorm:"index:idx_bill_record_created_at_id,priority:2;comment:账单记录主键"`
	CreatedAt                     int64   `json:"created_at" gorm:"bigint;index:idx_bill_record_created_at_id,priority:1;comment:调用完成并生成账单记录的时间戳（秒），用于时间范围过滤和排序"`
	Day                           string  `json:"day" gorm:"type:varchar(16);index:idx_bill_record_day_model,priority:1;comment:账单归属日期（Asia/Shanghai，格式 YYYY-MM-DD），用于按天汇总"`
	SiteURL                       string  `json:"site_url" gorm:"type:varchar(255);default:'';comment:记账时固化的对外账单站点地址，用于标识账单归属站点"`
	UserId                        int     `json:"user_id" gorm:"comment:发起调用的用户 ID，用于用户维度查询和数据隔离"`
	Username                      string  `json:"username" gorm:"type:varchar(64);default:'';comment:记账时固化的用户名，用于账单展示和汇总"`
	ChannelId                     int     `json:"channel_id" gorm:"comment:实际承载调用的渠道 ID，用于渠道维度追踪"`
	ChannelName                   string  `json:"channel_name" gorm:"type:varchar(128);default:'';comment:记账时固化的渠道名称，避免渠道改名影响历史账单"`
	ModelName                     string  `json:"model_name" gorm:"type:varchar(128);index:idx_bill_record_day_model,priority:2;default:'';comment:实际计费模型名称，用于模型维度汇总和筛选"`
	TokenName                     string  `json:"token_name" gorm:"type:varchar(128);default:'';comment:调用所用令牌名称，用于逐笔明细展示"`
	TokenId                       int     `json:"token_id" gorm:"comment:调用所用令牌 ID，用于令牌维度追踪"`
	UseGroup                      string  `json:"use_group" gorm:"column:use_group;type:varchar(64);default:'';comment:本次调用采用的计费分组，用于还原分组定价上下文"`
	RequestId                     string  `json:"request_id" gorm:"type:varchar(64);index;default:'';comment:系统生成的请求 ID，用于日志与账单记录关联"`
	ClientRequestId               string  `json:"client_request_id" gorm:"type:varchar(128);index;default:'';comment:客户端通过 X-Client-Request-Id 传入的请求标识，用于客户侧对账"`
	PromptTokens                  int     `json:"prompt_tokens" gorm:"default:0;comment:输入 Token 数量，用于输入费用计算"`
	ActualInputTokens             int     `json:"actual_input_tokens" gorm:"default:0;comment:扣除单独计价缓存类别后的实际输入 Token 数量"`
	CompletionTokens              int     `json:"completion_tokens" gorm:"default:0;comment:输出 Token 数量，用于输出费用计算"`
	CachedInputTokens             int     `json:"cached_input_tokens" gorm:"default:0;comment:OpenAI 语义下包含在输入总量中的缓存命中 Token 数量"`
	CacheReadTokens               int     `json:"cache_read_tokens" gorm:"default:0;comment:Anthropic 语义下独立于输入总量的缓存读取 Token 数量"`
	CacheCreationTokens           int     `json:"cache_creation_tokens" gorm:"default:0;comment:缓存创建 Token 数量，用于常规缓存写入费用计算"`
	CacheCreation1hTokens         int     `json:"cache_creation_1h_tokens" gorm:"default:0;comment:一小时缓存创建 Token 数量，用于长时缓存写入费用计算"`
	AudioInputTokens              int     `json:"audio_input_tokens" gorm:"default:0;comment:音频输入 Token 数量，用于音频输入费用计算"`
	ImageInputTokens              int     `json:"image_input_tokens" gorm:"default:0;comment:图片输入计量数量，用于多模态账单明细展示"`
	MediaDurationMs               int64   `json:"media_duration_ms" gorm:"default:0;comment:成功生成的视频或音频时长（毫秒），用于按时长计费和对外账单 time 字段"`
	UseTime                       int     `json:"use_time" gorm:"default:0;comment:本次调用耗时（秒），用于逐笔调用明细展示"`
	IsStream                      bool    `json:"is_stream" gorm:"comment:是否为流式调用，用于逐笔调用类型展示"`
	IsTask                        bool    `json:"is_task" gorm:"comment:是否为异步任务调用，用于区分任务类账单"`
	Number                        int     `json:"number" gorm:"comment:成功且产生计费的调用数量，按次计费时用于费用计算"`
	PricingSpec                   string  `json:"pricing_spec" gorm:"type:varchar(255);default:'';comment:记账时固化的价格适用规格，例如 input_tokens<=200k 或 time=8000ms"`
	PricingUnit                   string  `json:"pricing_unit" gorm:"type:varchar(32);default:'';comment:记账时固化的计价单位，例如百万 tokens、次或秒"`
	PricingCurrency               string  `json:"pricing_currency" gorm:"type:varchar(16);default:'';comment:记账时固化的价格币种，避免配置变化影响历史账单"`
	BillingMode                   string  `json:"billing_mode" gorm:"type:varchar(32);default:'ratio';comment:记账时固化的计费模式，ratio 表示倍率计费，tiered_expr 表示阶梯表达式计费"`
	MatchedTier                   string  `json:"matched_tier" gorm:"type:varchar(255);default:'';comment:阶梯表达式计费实际命中的阶梯名称或规格，倍率计费时为空"`
	Discount                      float64 `json:"discount" gorm:"default:0;comment:记账时固化的结算折扣，用于展示本次账单采用的折扣"`
	InputPublishedPrice           float64 `json:"input_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方输入价格"`
	OutputPublishedPrice          float64 `json:"output_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方输出价格"`
	CachedInputPublishedPrice     float64 `json:"cached_input_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方缓存读取价格"`
	CacheReadPublishedPrice       float64 `json:"cache_read_published_price" gorm:"default:0;comment:记账时固化的 Anthropic 缓存读取官方价格"`
	CacheCreatePublishedPrice     float64 `json:"cache_create_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方缓存创建价格"`
	CacheCreateHourPublishedPrice float64 `json:"cache_create_hour_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方一小时缓存创建价格"`
	InputAudioPublishedPrice      float64 `json:"input_audio_published_price" gorm:"default:0;comment:记账时由倍率推导并固化的官方音频输入价格"`
	NumberPublishedPrice          float64 `json:"number_published_price" gorm:"default:0;comment:记账时由基础价格和附加倍率推导并固化的按次官方价格"`
	TimePublishedPrice            float64 `json:"time_published_price" gorm:"default:0;comment:记账时由基础价格和附加倍率推导并固化的每秒官方价格"`
	ConsumeCost                   float64 `json:"consume_cost" gorm:"default:0;comment:按固化官方价格和实际用量计算的消费原价"`
	SettleCost                    float64 `json:"settle_cost" gorm:"default:0;comment:消费原价应用固化折扣后的结算金额"`
	FinalSettleCost               float64 `json:"final_settle_cost" gorm:"default:0;comment:结算金额应用账单税费规则后的最终结算金额"`
	PricingContext                string  `json:"pricing_context" gorm:"type:text;comment:记账时使用的模型倍率、缓存倍率、分组倍率及附加倍率 JSON，仅用于定价审计"`
	RawOther                      string  `json:"raw_other" gorm:"type:text;comment:原始计费扩展信息 JSON，用于问题排查和后续字段补全"`
}

type BillAliDayListItem struct {
	SupplierName                  string  `json:"supplierName"`
	SiteURL                       string  `json:"siteUrl"`
	Day                           string  `json:"day"`
	Username                      string  `json:"username"`
	ChannelName                   string  `json:"channelName"`
	ModelName                     string  `json:"modelName"`
	PricingSpec                   string  `json:"pricingSpec"`
	PricingUnit                   string  `json:"pricingUnit"`
	PricingCurrency               string  `json:"pricingCurrency"`
	Number                        int64   `json:"number"`
	InputToken                    int64   `json:"inputToken"`
	ActualInputToken              int64   `json:"actualInputToken"`
	CachedInputToken              int64   `json:"cachedInputToken"`
	CacheCreateToken              int64   `json:"cacheCreateToken"`
	CacheCreateHourToken          int64   `json:"cacheCreateHourToken"`
	CacheReadToken                int64   `json:"cacheReadToken"`
	InputAudioToken               int64   `json:"inputAudioToken"`
	OutputToken                   int64   `json:"outputToken"`
	Time                          int64   `json:"time"`
	InputPublishedPrice           float64 `json:"inputPublishedPrice"`
	OutputPublishedPrice          float64 `json:"outputPublishedPrice"`
	CachedInputPublishedPrice     float64 `json:"cachedInputPublishedPrice"`
	CacheReadPublishedPrice       float64 `json:"cacheReadPublishedPrice"`
	CacheCreatePublishedPrice     float64 `json:"cacheCreatePublishedPrice"`
	CacheCreateHourPublishedPrice float64 `json:"cacheCreateHourPublishedPrice"`
	InputAudioPublishedPrice      float64 `json:"inputAudioPublishedPrice"`
	NumberPublishedPrice          float64 `json:"numberPublishedPrice"`
	TimePublishedPrice            float64 `json:"timePublishedPrice"`
	Discount                      float64 `json:"discount"`
	ConsumeCost                   float64 `json:"consumeCost"`
	SettleCost                    float64 `json:"settleCost"`
	FinalSettleCost               float64 `json:"finalSettleCost"`
	channelIdInternal             int
	mediaDurationMsInternal       int64
}

type BillAliDetailListItem struct {
	SiteURL             string `json:"siteUrl"`
	Time                string `json:"time"`
	UserId              int    `json:"userId"`
	Username            string `json:"username"`
	TokenName           string `json:"tokenName"`
	ModelName           string `json:"modelName"`
	PromptTokens        int    `json:"promptTokens"`
	CompletionTokens    int    `json:"completionTokens"`
	CacheTokens         int    `json:"cacheTokens"`
	CacheCreationTokens int    `json:"cacheCreationTokens"`
	ImageInput          int    `json:"imageInput"`
	AudioInput          int    `json:"audioInput"`
	IsStream            bool   `json:"isStream"`
	UseTime             int    `json:"useTime"`
	RequestId           string `json:"requestId"`
	ClientRequestId     string `json:"clientRequestId"`
}

type billConfig struct {
	SupplierName    string
	SiteURL         string
	PricingCurrency string
	Discount        float64
}

type billPrices struct {
	PricingUnit                   string
	InputPublishedPrice           float64
	OutputPublishedPrice          float64
	CachedInputPublishedPrice     float64
	CacheReadPublishedPrice       float64
	CacheCreatePublishedPrice     float64
	CacheCreateHourPublishedPrice float64
	InputAudioPublishedPrice      float64
	NumberPublishedPrice          float64
	TimePublishedPrice            float64
	ConsumeCost                   float64
	SettleCost                    float64
	FinalSettleCost               float64
}

type billPricingContext struct {
	ModelRatio               float64
	CompletionRatio          float64
	CacheRatio               float64
	CacheCreationRatio       float64
	CacheCreation1hRatio     float64
	AudioRatio               float64
	AudioInputPrice          float64
	AudioCompletionRatio     float64
	ModelPrice               float64
	GroupRatio               float64
	OtherRatios              map[string]float64
	BillingMode              string
	TieredConsumeCost        float64
	TieredInputPrice         float64
	TieredOutputPrice        float64
	TieredCacheReadPrice     float64
	TieredCacheCreatePrice   float64
	TieredCacheCreate1hPrice float64
	TieredInputAudioPrice    float64
}

type billUsageSnapshot struct {
	ActualInputTokens     int
	CachedInputTokens     int
	CacheReadTokens       int
	CacheCreationTokens   int
	CacheCreation1hTokens int
}

func getBillConfig() billConfig {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	discount, _ := strconv.ParseFloat(strings.TrimSpace(common.OptionMap[BillDiscountOptionKey]), 64)
	if discount <= 0 {
		discount = 1
	}
	currency := strings.TrimSpace(common.OptionMap[BillPricingCurrencyOptionKey])
	if currency == "" {
		currency = "USD"
	}
	return billConfig{
		SupplierName:    strings.TrimSpace(common.OptionMap[BillSupplierNameOptionKey]),
		SiteURL:         strings.TrimSpace(common.OptionMap[BillSiteURLOptionKey]),
		PricingCurrency: currency,
		Discount:        discount,
	}
}

func billDayFromTimestamp(ts int64) string {
	return time.Unix(ts, 0).In(shanghaiLocation).Format("2006-01-02")
}

func billTimeString(ts int64) string {
	return time.Unix(ts, 0).In(shanghaiLocation).Format("2006-01-02 15:04:05")
}

func ParseBillDayRange(startDay, endDay string) (int64, int64, error) {
	if strings.TrimSpace(startDay) == "" && strings.TrimSpace(endDay) == "" {
		yesterday := time.Now().In(shanghaiLocation).AddDate(0, 0, -1)
		startDay = yesterday.Format("2006-01-02")
		endDay = startDay
	}
	if strings.TrimSpace(startDay) == "" {
		startDay = endDay
	}
	if strings.TrimSpace(endDay) == "" {
		endDay = startDay
	}
	startTime, err := time.ParseInLocation("2006-01-02", startDay, shanghaiLocation)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid startDay")
	}
	endTime, err := time.ParseInLocation("2006-01-02", endDay, shanghaiLocation)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid endDay")
	}
	return startTime.Unix(), endTime.Add(24*time.Hour - time.Second).Unix(), nil
}

func ParseBillDetailRange(startTime, endTime string) (int64, int64, error) {
	if strings.TrimSpace(startTime) == "" || strings.TrimSpace(endTime) == "" {
		return 0, 0, fmt.Errorf("startTime and endTime are required")
	}
	start, err := time.ParseInLocation("2006-01-02 15:04:05", startTime, shanghaiLocation)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid startTime")
	}
	end, err := time.ParseInLocation("2006-01-02 15:04:05", endTime, shanghaiLocation)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid endTime")
	}
	if end.Before(start) {
		return 0, 0, fmt.Errorf("endTime must be after startTime")
	}
	if end.Sub(start) > 24*time.Hour {
		return 0, 0, fmt.Errorf("时间范围不能超过 24 小时")
	}
	return start.Unix(), end.Unix(), nil
}

func roundBillValue(v float64) float64 {
	return math.Round(v*10_000_000_000) / 10_000_000_000
}

func roundExternalBillAmount(v float64) float64 {
	return math.Round(v*100) / 100
}

func mapIntValue(other map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		value, ok := other[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float32:
			return int(v)
		case float64:
			return int(v)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func mapFloatValue(other map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		value, ok := other[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case int:
			return float64(v)
		case int32:
			return float64(v)
		case int64:
			return float64(v)
		case float32:
			return float64(v)
		case float64:
			return v
		case string:
			n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func mapDurationMilliseconds(other map[string]interface{}, keys ...string) int64 {
	seconds := mapFloatValue(other, keys...)
	if seconds <= 0 {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}

func mapBoolValue(other map[string]interface{}, key string) bool {
	value, ok := other[key]
	if !ok || value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func buildBillUsageSnapshot(promptTokens int, other map[string]interface{}) billUsageSnapshot {
	cacheTokens := mapIntValue(other, "cache_tokens")
	cacheCreationTotal := mapIntValue(other, "cache_creation_tokens", "cache_write_tokens")
	cacheCreation5m := mapIntValue(other, "cache_creation_tokens_5m")
	cacheCreation1h := mapIntValue(other, "cache_creation_tokens_1h")

	cacheCreationTokens := cacheCreationTotal
	if cacheCreation5m > 0 || cacheCreation1h > 0 {
		remaining := cacheCreationTotal - cacheCreation5m - cacheCreation1h
		if remaining < 0 {
			remaining = 0
		}
		cacheCreationTokens = cacheCreation5m + remaining
	}

	usageSemantic, _ := other["usage_semantic"].(string)
	isAnthropic := strings.EqualFold(strings.TrimSpace(usageSemantic), "anthropic")
	snapshot := billUsageSnapshot{
		ActualInputTokens:     promptTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheCreation1hTokens: cacheCreation1h,
	}
	if isAnthropic {
		snapshot.CacheReadTokens = cacheTokens
		return snapshot
	}

	snapshot.CachedInputTokens = cacheTokens
	snapshot.ActualInputTokens -= cacheTokens + cacheCreationTokens + cacheCreation1h
	audioInputTokens := mapIntValue(other, "audio_input", "audio_input_token_count")
	if audioInputTokens > 0 && (mapFloatValue(other, "audio_input_price") > 0 || mapFloatValue(other, "audio_ratio") > 0) {
		snapshot.ActualInputTokens -= audioInputTokens
	}
	if snapshot.ActualInputTokens < 0 {
		snapshot.ActualInputTokens = 0
	}
	return snapshot
}

func buildPricingSpec(other map[string]interface{}, mediaDurationMs int64) string {
	if explicit, ok := other["pricing_spec"].(string); ok && strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}

	parts := make([]string, 0, 3)
	if mediaDurationMs > 0 {
		parts = append(parts, fmt.Sprintf("time=%dms", mediaDurationMs))
	}
	specKeys := make([]string, 0, 2)
	for key := range other {
		if key == "size" || key == "resolution" || strings.HasPrefix(key, "resolution-") {
			specKeys = append(specKeys, key)
		}
	}
	sort.Strings(specKeys)
	for _, key := range specKeys {
		value := strings.TrimSpace(fmt.Sprint(other[key]))
		if value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ",")
	}

	matchedTier, _ := other["matched_tier"].(string)
	encodedExpression, _ := other["expr_b64"].(string)
	matchedTier = strings.TrimSpace(matchedTier)
	encodedExpression = strings.TrimSpace(encodedExpression)
	if matchedTier != "" && encodedExpression != "" {
		expression, err := base64.StdEncoding.DecodeString(encodedExpression)
		if err == nil {
			if spec := billingexpr.MatchedTierSpec(string(expression), matchedTier); spec != "" {
				return spec
			}
		}
		return "tier=" + matchedTier
	}
	return "default"
}

func parseOtherRatios(other map[string]interface{}) map[string]float64 {
	ratios := make(map[string]float64)
	for key := range other {
		if key == "seconds" || key == "size" || key == "resolution" || strings.HasPrefix(key, "resolution-") {
			ratios[key] = mapFloatValue(other, key)
		}
	}
	return ratios
}

func buildBillPricingContext(other map[string]interface{}) billPricingContext {
	billingMode, _ := other["billing_mode"].(string)
	billingMode = strings.TrimSpace(billingMode)
	if billingMode == "" {
		billingMode = "ratio"
	}
	return billPricingContext{
		ModelRatio:               mapFloatValue(other, "model_ratio"),
		CompletionRatio:          mapFloatValue(other, "completion_ratio"),
		CacheRatio:               mapFloatValue(other, "cache_ratio"),
		CacheCreationRatio:       mapFloatValue(other, "cache_creation_ratio_5m", "cache_creation_ratio"),
		CacheCreation1hRatio:     mapFloatValue(other, "cache_creation_ratio_1h"),
		AudioRatio:               mapFloatValue(other, "audio_ratio"),
		AudioInputPrice:          mapFloatValue(other, "audio_input_price"),
		AudioCompletionRatio:     mapFloatValue(other, "audio_completion_ratio"),
		ModelPrice:               mapFloatValue(other, "model_price"),
		GroupRatio:               mapFloatValue(other, "group_ratio"),
		OtherRatios:              parseOtherRatios(other),
		BillingMode:              billingMode,
		TieredConsumeCost:        mapFloatValue(other, "tiered_consume_cost"),
		TieredInputPrice:         mapFloatValue(other, "tiered_input_published_price"),
		TieredOutputPrice:        mapFloatValue(other, "tiered_output_published_price"),
		TieredCacheReadPrice:     mapFloatValue(other, "tiered_cache_read_published_price"),
		TieredCacheCreatePrice:   mapFloatValue(other, "tiered_cache_create_published_price"),
		TieredCacheCreate1hPrice: mapFloatValue(other, "tiered_cache_create_1h_published_price"),
		TieredInputAudioPrice:    mapFloatValue(other, "tiered_input_audio_published_price"),
	}
}

func buildBillPricingAudit(other map[string]interface{}) map[string]interface{} {
	audit := make(map[string]interface{})
	for key, value := range other {
		lowerKey := strings.ToLower(key)
		if strings.Contains(lowerKey, "ratio") ||
			strings.HasPrefix(key, "tiered_") ||
			key == "billing_mode" ||
			key == "matched_tier" ||
			key == "expr_b64" ||
			key == "model_price" ||
			key == "audio_input_price" ||
			key == "seconds" ||
			key == "duration" ||
			key == "size" ||
			key == "resolution" ||
			strings.HasPrefix(key, "resolution-") {
			audit[key] = value
		}
	}
	if billingMode, _ := other["billing_mode"].(string); strings.TrimSpace(billingMode) == "" {
		audit["billing_mode"] = "ratio"
	}
	return audit
}

func recordBillFromConsumeLog(userId int, username string, createdAt int64, requestId string, params RecordConsumeLogParams, clientRequestId string) {
	if params.Quota <= 0 {
		return
	}
	config := getBillConfig()
	other := params.Other
	if other == nil {
		other = map[string]interface{}{}
	}
	pricingContext := buildBillPricingContext(other)
	pricingContextJSON, err := common.Marshal(buildBillPricingAudit(other))
	if err != nil {
		common.SysLog("failed to serialize bill pricing context: " + err.Error())
		return
	}
	mediaDurationMs := mapDurationMilliseconds(other, "seconds", "duration")
	usageSnapshot := buildBillUsageSnapshot(params.PromptTokens, other)
	matchedTier, _ := other["matched_tier"].(string)
	record := &BillRecord{
		CreatedAt:             createdAt,
		Day:                   billDayFromTimestamp(createdAt),
		SiteURL:               config.SiteURL,
		UserId:                userId,
		Username:              username,
		ChannelId:             params.ChannelId,
		ChannelName:           resolveBillChannelName(params.ChannelId),
		ModelName:             params.ModelName,
		TokenName:             params.TokenName,
		TokenId:               params.TokenId,
		UseGroup:              params.Group,
		RequestId:             requestId,
		ClientRequestId:       clientRequestId,
		PromptTokens:          params.PromptTokens,
		ActualInputTokens:     usageSnapshot.ActualInputTokens,
		CompletionTokens:      params.CompletionTokens,
		CachedInputTokens:     usageSnapshot.CachedInputTokens,
		CacheReadTokens:       usageSnapshot.CacheReadTokens,
		CacheCreationTokens:   usageSnapshot.CacheCreationTokens,
		CacheCreation1hTokens: usageSnapshot.CacheCreation1hTokens,
		AudioInputTokens:      mapIntValue(other, "audio_input", "audio_input_token_count"),
		ImageInputTokens:      mapIntValue(other, "image_input"),
		MediaDurationMs:       mediaDurationMs,
		UseTime:               params.UseTimeSeconds,
		IsStream:              params.IsStream,
		IsTask:                mapBoolValue(other, "is_task"),
		PricingSpec:           buildPricingSpec(other, mediaDurationMs),
		PricingCurrency:       config.PricingCurrency,
		BillingMode:           pricingContext.BillingMode,
		MatchedTier:           strings.TrimSpace(matchedTier),
		Discount:              config.Discount,
		PricingContext:        string(pricingContextJSON),
		RawOther:              common.MapToJsonStr(other),
	}
	if isBillNumberPricedCall(record, pricingContext) {
		record.Number = 1
	}
	prices := computeBillPrices(record, pricingContext, config.Discount)
	record.PricingUnit = prices.PricingUnit
	record.InputPublishedPrice = prices.InputPublishedPrice
	record.OutputPublishedPrice = prices.OutputPublishedPrice
	record.CachedInputPublishedPrice = prices.CachedInputPublishedPrice
	record.CacheReadPublishedPrice = prices.CacheReadPublishedPrice
	record.CacheCreatePublishedPrice = prices.CacheCreatePublishedPrice
	record.CacheCreateHourPublishedPrice = prices.CacheCreateHourPublishedPrice
	record.InputAudioPublishedPrice = prices.InputAudioPublishedPrice
	record.NumberPublishedPrice = prices.NumberPublishedPrice
	record.TimePublishedPrice = prices.TimePublishedPrice
	record.ConsumeCost = prices.ConsumeCost
	record.SettleCost = prices.SettleCost
	record.FinalSettleCost = prices.FinalSettleCost
	if err := DB.Create(record).Error; err != nil {
		common.SysLog("failed to record bill record: " + err.Error())
	}
}

func resolveBillChannelName(channelId int) string {
	if channelId <= 0 {
		return ""
	}
	if channel, err := CacheGetChannel(channelId); err == nil && channel != nil {
		return channel.Name
	}
	channel, err := GetChannelById(channelId, false)
	if err != nil || channel == nil {
		return ""
	}
	return channel.Name
}

func hasTokenUsage(record *BillRecord) bool {
	return record.PromptTokens > 0 ||
		record.CompletionTokens > 0 ||
		record.CachedInputTokens > 0 ||
		record.CacheReadTokens > 0 ||
		record.CacheCreationTokens > 0 ||
		record.CacheCreation1hTokens > 0 ||
		record.AudioInputTokens > 0 ||
		record.ImageInputTokens > 0
}

func isBillNumberPricedCall(record *BillRecord, pricingContext billPricingContext) bool {
	if pricingContext.BillingMode == "tiered_expr" || record.MediaDurationMs > 0 {
		return false
	}
	return (pricingContext.ModelPrice > 0 && pricingContext.ModelRatio <= 0) || !hasTokenUsage(record)
}

func computeBillPrices(record *BillRecord, pricingContext billPricingContext, discount float64) billPrices {
	if pricingContext.BillingMode == "tiered_expr" {
		prices := billPrices{
			PricingUnit:                   "百万tokens",
			InputPublishedPrice:           roundBillValue(pricingContext.TieredInputPrice),
			OutputPublishedPrice:          roundBillValue(pricingContext.TieredOutputPrice),
			CachedInputPublishedPrice:     roundBillValue(pricingContext.TieredCacheReadPrice),
			CacheReadPublishedPrice:       roundBillValue(pricingContext.TieredCacheReadPrice),
			CacheCreatePublishedPrice:     roundBillValue(pricingContext.TieredCacheCreatePrice),
			CacheCreateHourPublishedPrice: roundBillValue(pricingContext.TieredCacheCreate1hPrice),
			InputAudioPublishedPrice:      roundBillValue(pricingContext.TieredInputAudioPrice),
			ConsumeCost:                   roundBillValue(pricingContext.TieredConsumeCost),
		}
		prices.SettleCost = roundBillValue(prices.ConsumeCost * discount)
		prices.FinalSettleCost = roundBillValue(prices.SettleCost * 1.06)
		return prices
	}

	baseTokenPrice := pricingContext.ModelRatio * 2
	outputPrice := baseTokenPrice * pricingContext.CompletionRatio
	cachePrice := baseTokenPrice * pricingContext.CacheRatio
	cacheCreatePrice := baseTokenPrice * pricingContext.CacheCreationRatio
	cacheCreate1hPrice := baseTokenPrice * pricingContext.CacheCreation1hRatio
	audioInputPrice := baseTokenPrice * pricingContext.AudioRatio
	if pricingContext.AudioInputPrice > 0 {
		audioInputPrice = pricingContext.AudioInputPrice
	}

	nonSecondsMultiplier := 1.0
	for key, ratio := range pricingContext.OtherRatios {
		if key == "seconds" || ratio <= 0 || ratio == 1 {
			continue
		}
		nonSecondsMultiplier *= ratio
	}
	baseUnitPrice := pricingContext.ModelPrice
	if baseUnitPrice <= 0 {
		baseUnitPrice = baseTokenPrice
	}

	prices := billPrices{
		PricingUnit:                   "百万tokens",
		InputPublishedPrice:           roundBillValue(baseTokenPrice),
		OutputPublishedPrice:          roundBillValue(outputPrice),
		CachedInputPublishedPrice:     roundBillValue(cachePrice),
		CacheReadPublishedPrice:       roundBillValue(cachePrice),
		CacheCreatePublishedPrice:     roundBillValue(cacheCreatePrice),
		CacheCreateHourPublishedPrice: roundBillValue(cacheCreate1hPrice),
		InputAudioPublishedPrice:      roundBillValue(audioInputPrice),
	}

	consumeCost := 0.0
	if hasTokenUsage(record) {
		consumeCost += float64(record.ActualInputTokens) / 1_000_000 * baseTokenPrice
		consumeCost += float64(record.CachedInputTokens) / 1_000_000 * cachePrice
		consumeCost += float64(record.CacheReadTokens) / 1_000_000 * cachePrice
		consumeCost += float64(record.CacheCreationTokens) / 1_000_000 * cacheCreatePrice
		consumeCost += float64(record.CacheCreation1hTokens) / 1_000_000 * cacheCreate1hPrice
		consumeCost += float64(record.AudioInputTokens) / 1_000_000 * audioInputPrice
		consumeCost += float64(record.CompletionTokens) / 1_000_000 * outputPrice
	}
	if record.MediaDurationMs > 0 {
		prices.PricingUnit = "秒"
		prices.TimePublishedPrice = roundBillValue(baseUnitPrice * nonSecondsMultiplier)
		consumeCost += float64(record.MediaDurationMs) / 1000 * prices.TimePublishedPrice
	}
	isFixedPriceCall := pricingContext.ModelPrice > 0 && pricingContext.ModelRatio <= 0
	if isBillNumberPricedCall(record, pricingContext) && record.Number > 0 {
		prices.PricingUnit = "次"
		prices.NumberPublishedPrice = roundBillValue(baseUnitPrice * nonSecondsMultiplier)
		if isFixedPriceCall {
			consumeCost = float64(record.Number) * prices.NumberPublishedPrice
		} else {
			consumeCost += float64(record.Number) * prices.NumberPublishedPrice
		}
	}

	prices.ConsumeCost = roundBillValue(consumeCost)
	prices.SettleCost = roundBillValue(prices.ConsumeCost * discount)
	prices.FinalSettleCost = roundBillValue(prices.SettleCost * 1.06)
	return prices
}

func GetBillAliDetailList(startAt, endAt int64, modelPrefix string, siteURL string, pageNum, pageSize int) ([]*BillAliDetailListItem, int64, error) {
	config := getBillConfig()
	if siteURL != "" && siteURL != config.SiteURL {
		return []*BillAliDetailListItem{}, 0, nil
	}
	query := DB.Model(&BillRecord{}).Where("created_at >= ? AND created_at <= ?", startAt, endAt)
	if strings.TrimSpace(modelPrefix) != "" {
		query = query.Where("model_name LIKE ?", strings.TrimSpace(modelPrefix)+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*BillRecord
	offset := 0
	if pageNum > 1 {
		offset = (pageNum - 1) * pageSize
	}
	if err := query.Order("created_at desc, id desc").Limit(pageSize).Offset(offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*BillAliDetailListItem, 0, len(records))
	for _, record := range records {
		items = append(items, &BillAliDetailListItem{
			SiteURL:             config.SiteURL,
			Time:                billTimeString(record.CreatedAt),
			UserId:              record.UserId,
			Username:            record.Username,
			TokenName:           record.TokenName,
			ModelName:           record.ModelName,
			PromptTokens:        record.PromptTokens,
			CompletionTokens:    record.CompletionTokens,
			CacheTokens:         record.CachedInputTokens + record.CacheReadTokens,
			CacheCreationTokens: record.CacheCreationTokens + record.CacheCreation1hTokens,
			ImageInput:          record.ImageInputTokens,
			AudioInput:          record.AudioInputTokens,
			IsStream:            record.IsStream,
			UseTime:             record.UseTime,
			RequestId:           record.RequestId,
			ClientRequestId:     record.ClientRequestId,
		})
	}
	return items, total, nil
}

func GetBillAliDayList(startAt, endAt int64, modelPrefix string) ([]*BillAliDayListItem, error) {
	config := getBillConfig()
	query := DB.Model(&BillRecord{}).Where(
		"day >= ? AND day <= ?",
		billDayFromTimestamp(startAt),
		billDayFromTimestamp(endAt),
	)
	if strings.TrimSpace(modelPrefix) != "" {
		query = query.Where("model_name LIKE ?", strings.TrimSpace(modelPrefix)+"%")
	}
	var records []*BillRecord
	if err := query.Order("created_at asc, id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	aggregated := make(map[string]*BillAliDayListItem)
	for _, record := range records {
		key := strings.Join([]string{
			record.Day,
			record.Username,
			strconv.Itoa(record.ChannelId),
			record.ChannelName,
			record.ModelName,
			record.PricingSpec,
			record.PricingUnit,
			record.PricingCurrency,
			strconv.FormatFloat(record.Discount, 'g', -1, 64),
			strconv.FormatFloat(record.InputPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.OutputPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.CachedInputPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.CacheReadPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.CacheCreatePublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.CacheCreateHourPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.InputAudioPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.NumberPublishedPrice, 'g', -1, 64),
			strconv.FormatFloat(record.TimePublishedPrice, 'g', -1, 64),
		}, "\x00")
		item, ok := aggregated[key]
		if !ok {
			item = &BillAliDayListItem{
				SupplierName:                  config.SupplierName,
				SiteURL:                       config.SiteURL,
				Day:                           record.Day,
				Username:                      record.Username,
				ChannelName:                   record.ChannelName,
				ModelName:                     record.ModelName,
				PricingSpec:                   record.PricingSpec,
				PricingUnit:                   record.PricingUnit,
				PricingCurrency:               record.PricingCurrency,
				InputPublishedPrice:           record.InputPublishedPrice,
				OutputPublishedPrice:          record.OutputPublishedPrice,
				CachedInputPublishedPrice:     record.CachedInputPublishedPrice,
				CacheReadPublishedPrice:       record.CacheReadPublishedPrice,
				CacheCreatePublishedPrice:     record.CacheCreatePublishedPrice,
				CacheCreateHourPublishedPrice: record.CacheCreateHourPublishedPrice,
				InputAudioPublishedPrice:      record.InputAudioPublishedPrice,
				NumberPublishedPrice:          record.NumberPublishedPrice,
				TimePublishedPrice:            record.TimePublishedPrice,
				Discount:                      record.Discount,
				channelIdInternal:             record.ChannelId,
			}
			aggregated[key] = item
		}
		if record.PricingUnit == "次" {
			item.Number += int64(record.Number)
		}
		item.InputToken += int64(record.PromptTokens)
		item.ActualInputToken += int64(record.ActualInputTokens)
		item.CachedInputToken += int64(record.CachedInputTokens)
		item.CacheCreateToken += int64(record.CacheCreationTokens)
		item.CacheCreateHourToken += int64(record.CacheCreation1hTokens)
		item.CacheReadToken += int64(record.CacheReadTokens)
		item.InputAudioToken += int64(record.AudioInputTokens)
		item.OutputToken += int64(record.CompletionTokens)
		item.mediaDurationMsInternal += record.MediaDurationMs
		item.ConsumeCost = roundBillValue(item.ConsumeCost + record.ConsumeCost)
		item.SettleCost = roundBillValue(item.SettleCost + record.SettleCost)
		item.FinalSettleCost = roundBillValue(item.FinalSettleCost + record.FinalSettleCost)
	}

	items := make([]*BillAliDayListItem, 0, len(aggregated))
	for _, item := range aggregated {
		item.Time = int64(math.Round(float64(item.mediaDurationMsInternal) / 1000))
		item.ConsumeCost = roundExternalBillAmount(item.ConsumeCost)
		item.SettleCost = roundExternalBillAmount(item.SettleCost)
		item.FinalSettleCost = roundExternalBillAmount(item.FinalSettleCost)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Day != items[j].Day {
			return items[i].Day < items[j].Day
		}
		if items[i].Username != items[j].Username {
			return items[i].Username < items[j].Username
		}
		if items[i].ModelName != items[j].ModelName {
			return items[i].ModelName < items[j].ModelName
		}
		return items[i].channelIdInternal < items[j].channelIdInternal
	})
	return items, nil
}
