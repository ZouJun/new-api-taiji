package model

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseBillDayRange(t *testing.T) {
	startAt, endAt, err := ParseBillDayRange("2026-02-01", "2026-02-02")
	require.NoError(t, err)
	assert.Equal(t, int64(1769875200), startAt)
	assert.Equal(t, int64(1770047999), endAt)
}

func TestParseBillDetailRangeRejectsTooLong(t *testing.T) {
	_, _, err := ParseBillDetailRange("2026-02-01 00:00:00", "2026-02-02 00:00:01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "24")
}

func TestComputeBillPricesTokenBased(t *testing.T) {
	record := &BillRecord{
		PromptTokens:          1_000_000,
		ActualInputTokens:     825_000,
		CachedInputTokens:     100_000,
		CacheCreationTokens:   50_000,
		CacheCreation1hTokens: 25_000,
		AudioInputTokens:      20_000,
		CompletionTokens:      500_000,
	}
	pricingContext := billPricingContext{
		ModelRatio:           1.25,
		CompletionRatio:      4,
		CacheRatio:           0.25,
		CacheCreationRatio:   1.25,
		CacheCreation1hRatio: 2,
		AudioRatio:           10,
	}

	prices := computeBillPrices(record, pricingContext, 0.9)
	assert.Equal(t, "百万tokens", prices.PricingUnit)
	assert.Equal(t, 2.5, prices.InputPublishedPrice)
	assert.Equal(t, 10.0, prices.OutputPublishedPrice)
	assert.Equal(t, 0.625, prices.CachedInputPublishedPrice)
	assert.Equal(t, 0.625, prices.CacheReadPublishedPrice)
	assert.Equal(t, 7.90625, prices.ConsumeCost)
	assert.Equal(t, 7.115625, prices.SettleCost)
	assert.Equal(t, 7.5425625, prices.FinalSettleCost)
}

func TestComputeBillPricesTimeBased(t *testing.T) {
	record := &BillRecord{
		MediaDurationMs: 8000,
		Number:          1,
	}
	pricingContext := billPricingContext{
		ModelPrice:  0.5,
		OtherRatios: map[string]float64{"seconds": 8, "resolution": 1.5},
	}

	prices := computeBillPrices(record, pricingContext, 0.8)
	assert.Equal(t, "秒", prices.PricingUnit)
	assert.Equal(t, 0.75, prices.TimePublishedPrice)
	assert.Equal(t, 6.0, prices.ConsumeCost)
	assert.Equal(t, 4.8, prices.SettleCost)
	assert.Equal(t, 5.088, prices.FinalSettleCost)
}

func TestComputeBillPricesPreservesSmallCosts(t *testing.T) {
	record := &BillRecord{
		PromptTokens:      12,
		ActualInputTokens: 12,
		CompletionTokens:  126,
	}
	pricingContext := billPricingContext{
		ModelRatio:      0.005,
		CompletionRatio: 1,
	}

	prices := computeBillPrices(record, pricingContext, 1)

	assert.Equal(t, 0.00000138, prices.ConsumeCost)
	assert.Equal(t, 0.00000138, prices.SettleCost)
	assert.Equal(t, 0.0000014628, prices.FinalSettleCost)
}

func TestBuildBillUsageSnapshotOpenAI(t *testing.T) {
	snapshot := buildBillUsageSnapshot(1_000, map[string]interface{}{
		"usage_semantic":           "openai",
		"cache_tokens":             200,
		"cache_creation_tokens":    150,
		"cache_creation_tokens_5m": 100,
		"cache_creation_tokens_1h": 50,
	})

	assert.Equal(t, 650, snapshot.ActualInputTokens)
	assert.Equal(t, 200, snapshot.CachedInputTokens)
	assert.Zero(t, snapshot.CacheReadTokens)
	assert.Equal(t, 100, snapshot.CacheCreationTokens)
	assert.Equal(t, 50, snapshot.CacheCreation1hTokens)
}

func TestBuildBillUsageSnapshotAnthropic(t *testing.T) {
	snapshot := buildBillUsageSnapshot(1_000, map[string]interface{}{
		"usage_semantic":           "anthropic",
		"cache_tokens":             200,
		"cache_creation_tokens":    150,
		"cache_creation_tokens_5m": 100,
		"cache_creation_tokens_1h": 50,
	})

	assert.Equal(t, 1_000, snapshot.ActualInputTokens)
	assert.Zero(t, snapshot.CachedInputTokens)
	assert.Equal(t, 200, snapshot.CacheReadTokens)
	assert.Equal(t, 100, snapshot.CacheCreationTokens)
	assert.Equal(t, 50, snapshot.CacheCreation1hTokens)
}

func TestComputeBillPricesTiered(t *testing.T) {
	record := &BillRecord{ActualInputTokens: 1000, CompletionTokens: 500}
	pricingContext := billPricingContext{
		BillingMode:              "tiered_expr",
		TieredConsumeCost:        0.01,
		TieredInputPrice:         3,
		TieredOutputPrice:        15,
		TieredCacheReadPrice:     0.3,
		TieredCacheCreatePrice:   3.75,
		TieredCacheCreate1hPrice: 6,
		TieredInputAudioPrice:    10,
	}

	prices := computeBillPrices(record, pricingContext, 0.9)

	assert.Equal(t, 3.0, prices.InputPublishedPrice)
	assert.Equal(t, 15.0, prices.OutputPublishedPrice)
	assert.Equal(t, 0.3, prices.CacheReadPublishedPrice)
	assert.Equal(t, 0.01, prices.ConsumeCost)
	assert.Equal(t, 0.009, prices.SettleCost)
	assert.Equal(t, 0.00954, prices.FinalSettleCost)
}

func TestComputeBillPricesFixedPriceCallIgnoresReportedTokens(t *testing.T) {
	record := &BillRecord{
		PromptTokens:      100,
		ActualInputTokens: 100,
		CompletionTokens:  20,
		Number:            1,
	}
	pricingContext := billPricingContext{ModelPrice: 0.5}

	prices := computeBillPrices(record, pricingContext, 1)

	assert.Equal(t, "次", prices.PricingUnit)
	assert.Equal(t, 0.5, prices.NumberPublishedPrice)
	assert.Equal(t, 0.5, prices.ConsumeCost)
}

func TestIsBillNumberPricedCall(t *testing.T) {
	assert.True(t, isBillNumberPricedCall(
		&BillRecord{ActualInputTokens: 100, CompletionTokens: 20},
		billPricingContext{ModelPrice: 0.5},
	))
	assert.False(t, isBillNumberPricedCall(
		&BillRecord{ActualInputTokens: 100, CompletionTokens: 20},
		billPricingContext{ModelRatio: 1},
	))
	assert.False(t, isBillNumberPricedCall(
		&BillRecord{},
		billPricingContext{BillingMode: "tiered_expr"},
	))
	assert.False(t, isBillNumberPricedCall(
		&BillRecord{MediaDurationMs: 1_000},
		billPricingContext{ModelPrice: 0.5},
	))
}

func TestBuildBillUsageSnapshotExcludesSeparatelyPricedAudio(t *testing.T) {
	snapshot := buildBillUsageSnapshot(1_000, map[string]interface{}{
		"usage_semantic":          "openai",
		"audio_input_token_count": 200,
		"audio_input_price":       3.5,
	})

	assert.Equal(t, 800, snapshot.ActualInputTokens)
}

func TestMapDurationMilliseconds(t *testing.T) {
	other := map[string]interface{}{"seconds": 1.2345}

	assert.Equal(t, int64(1235), mapDurationMilliseconds(other, "seconds", "duration"))
}

func TestBuildPricingSpecForTieredTokens(t *testing.T) {
	expression := `len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long_context", p * 6 + c * 22.5)`
	other := map[string]interface{}{
		"matched_tier": "standard",
		"expr_b64":     base64.StdEncoding.EncodeToString([]byte(expression)),
	}

	assert.Equal(t, "input_tokens<=200k", buildPricingSpec(other, 0))
}

func TestBuildPricingSpecForMedia(t *testing.T) {
	other := map[string]interface{}{
		"seconds":    8,
		"resolution": 1.5,
	}

	assert.Equal(t, "time=8000ms,resolution=1.5", buildPricingSpec(other, 8000))
}

func TestBuildBillPricingAuditCollectsAllRatioFields(t *testing.T) {
	audit := buildBillPricingAudit(map[string]interface{}{
		"model_ratio":          1.25,
		"user_group_ratio":     0.8,
		"future_custom_ratio":  1.1,
		"model_price":          0.5,
		"resolution":           1.5,
		"seconds":              8,
		"cache_tokens":         100,
		"unrelated_debug_info": "ignored",
	})

	assert.Equal(t, 1.25, audit["model_ratio"])
	assert.Equal(t, 0.8, audit["user_group_ratio"])
	assert.Equal(t, 1.1, audit["future_custom_ratio"])
	assert.Equal(t, 0.5, audit["model_price"])
	assert.Equal(t, 1.5, audit["resolution"])
	assert.Equal(t, 8, audit["seconds"])
	assert.NotContains(t, audit, "cache_tokens")
	assert.NotContains(t, audit, "unrelated_debug_info")
}

func TestGetBillConfigUsesDedicatedSiteURL(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		"ServerAddress":              "https://public.example.com",
		BillSupplierNameOptionKey:    "Example Supplier",
		BillSiteURLOptionKey:         "https://billing.example.com",
		BillDiscountOptionKey:        "0.9",
		BillPricingCurrencyOptionKey: "CNY",
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	config := getBillConfig()

	assert.Equal(t, "Example Supplier", config.SupplierName)
	assert.Equal(t, "https://billing.example.com", config.SiteURL)
	assert.Equal(t, 0.9, config.Discount)
	assert.Equal(t, "CNY", config.PricingCurrency)
}

func TestBillRecordMigrationSupportsSQLite(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:bill_record_migration_test?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&BillRecord{}))
	assert.True(t, db.Migrator().HasTable(&BillRecord{}))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "client_request_id"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "channel_name"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "pricing_context"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "input_published_price"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "final_settle_cost"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "media_duration_ms"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "actual_input_tokens"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "cached_input_tokens"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "cache_read_tokens"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "cache_read_published_price"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "billing_mode"))
	assert.True(t, db.Migrator().HasColumn(&BillRecord{}, "matched_tier"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_record_created_at_id"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_record_day_model"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_request_id"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_client_request_id"))
	assert.False(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_channel_id"))
	assert.False(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_created_at"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "media_duration"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "cache_tokens"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "model_ratio"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "completion_ratio"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "other_ratios"))
}

func TestEnsureBillRecordIndexesRemovesRedundantIndexes(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:bill_record_index_cleanup_test?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&BillRecord{}))
	require.NoError(t, db.Exec(
		"CREATE INDEX idx_bill_records_channel_id ON bill_records (channel_id)",
	).Error)

	originalDB := DB
	DB = db
	t.Cleanup(func() {
		DB = originalDB
	})

	require.NoError(t, ensureBillRecordIndexes())
	assert.False(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_channel_id"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_record_created_at_id"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_record_day_model"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_request_id"))
	assert.True(t, db.Migrator().HasIndex(&BillRecord{}, "idx_bill_records_client_request_id"))
}

func TestGetBillAliDayListUsesStoredPricingSnapshot(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:bill_record_snapshot_test?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&BillRecord{}))

	originalDB := DB
	DB = db
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{
		BillSupplierNameOptionKey:    "Current Supplier",
		BillSiteURLOptionKey:         "https://current.example.com",
		BillDiscountOptionKey:        "0.5",
		BillPricingCurrencyOptionKey: "CNY",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	record := &BillRecord{
		CreatedAt:                     1769875200,
		Day:                           "2026-02-01",
		Username:                      "billing-user",
		ChannelId:                     3,
		ChannelName:                   "frozen-channel",
		ModelName:                     "example-model",
		PricingSpec:                   "default",
		PricingUnit:                   "百万tokens",
		PricingCurrency:               "USD",
		Discount:                      0.9,
		PromptTokens:                  1_000_000,
		ActualInputTokens:             1_000_000,
		MediaDurationMs:               1234,
		Number:                        1,
		InputPublishedPrice:           2.5,
		OutputPublishedPrice:          10,
		CachedInputPublishedPrice:     0.63,
		CacheCreatePublishedPrice:     3.13,
		CacheCreateHourPublishedPrice: 5,
		InputAudioPublishedPrice:      25,
		ConsumeCost:                   2.505,
		SettleCost:                    2.254,
		FinalSettleCost:               2.394,
		PricingContext:                `{"model_ratio":1.25}`,
	}
	require.NoError(t, db.Create(record).Error)

	items, err := GetBillAliDayList(1769875200, 1769961599, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "USD", item.PricingCurrency)
	assert.Equal(t, 0.9, item.Discount)
	assert.Equal(t, 2.5, item.InputPublishedPrice)
	assert.Equal(t, 2.51, item.ConsumeCost)
	assert.Equal(t, 2.25, item.SettleCost)
	assert.Equal(t, 2.39, item.FinalSettleCost)
	assert.Equal(t, int64(1), item.Time)
	assert.Zero(t, item.Number)
}

func TestRecordConsumeLogWritesBillWhenConsumeLogsDisabled(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:bill_record_log_disabled_test?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&BillRecord{}))

	originalDB := DB
	originalLogConsumeEnabled := common.LogConsumeEnabled
	DB = db
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		DB = originalDB
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	context.Request.Header.Set(ClientRequestIDHeader, "client-request")
	context.Set("username", "billing-user")
	context.Set(common.RequestIdKey, "request-id")

	RecordConsumeLog(context, 1, RecordConsumeLogParams{
		PromptTokens:     10,
		CompletionTokens: 20,
		ModelName:        "example-model",
		Quota:            1,
		Other: map[string]interface{}{
			"model_ratio":      1,
			"completion_ratio": 1,
		},
	})

	var record BillRecord
	require.NoError(t, db.First(&record).Error)
	assert.Equal(t, "request-id", record.RequestId)
	assert.Equal(t, "client-request", record.ClientRequestId)
	assert.Equal(t, "ratio", record.BillingMode)
	assert.Empty(t, record.MatchedTier)
	assert.Zero(t, record.Number)

	context.Set(common.RequestIdKey, "tiered-request-id")
	RecordConsumeLog(context, 1, RecordConsumeLogParams{
		PromptTokens:     10,
		CompletionTokens: 20,
		ModelName:        "tiered-model",
		Quota:            1,
		Other: map[string]interface{}{
			"billing_mode": "tiered_expr",
			"matched_tier": "standard",
		},
	})

	var tieredRecord BillRecord
	require.NoError(t, db.Where("request_id = ?", "tiered-request-id").First(&tieredRecord).Error)
	assert.Equal(t, "tiered_expr", tieredRecord.BillingMode)
	assert.Equal(t, "standard", tieredRecord.MatchedTier)
	assert.Zero(t, tieredRecord.Number)
}
