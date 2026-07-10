package model

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
		CacheTokens:           100_000,
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
	assert.Equal(t, 8.34375, prices.ConsumeCost)
	assert.Equal(t, 7.509375, prices.SettleCost)
	assert.Equal(t, 7.9599375, prices.FinalSettleCost)
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
		PromptTokens:     12,
		CompletionTokens: 126,
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
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "media_duration"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "model_ratio"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "completion_ratio"))
	assert.False(t, db.Migrator().HasColumn(&BillRecord{}, "other_ratios"))
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
		MediaDurationMs:               1234,
		Number:                        1,
		InputPublishedPrice:           2.5,
		OutputPublishedPrice:          10,
		CachedInputPublishedPrice:     0.63,
		CacheCreatePublishedPrice:     3.13,
		CacheCreateHourPublishedPrice: 5,
		InputAudioPublishedPrice:      25,
		ConsumeCost:                   2.5,
		SettleCost:                    2.25,
		FinalSettleCost:               2.39,
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
	assert.Equal(t, 2.5, item.ConsumeCost)
	assert.Equal(t, 2.25, item.SettleCost)
	assert.Equal(t, 2.39, item.FinalSettleCost)
	assert.Equal(t, int64(1234), item.Time)
}
