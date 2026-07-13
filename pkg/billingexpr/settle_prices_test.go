package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeTieredQuotaReturnsPublishedPrices(t *testing.T) {
	expression := `len <= 200000 ? tier("standard", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6 + ai * 10) : tier("long", p * 6 + c * 22.5)`
	snapshot := &BillingSnapshot{
		BillingMode:   "tiered_expr",
		ExprString:    expression,
		ExprHash:      ExprHashString(expression),
		GroupRatio:    1,
		QuotaPerUnit:  500_000,
		ExprVersion:   1,
		EstimatedTier: "standard",
	}

	result, err := ComputeTieredQuota(snapshot, TokenParams{
		P: 1000, C: 500, Len: 1000, CR: 100, CC: 50, CC1h: 25, AI: 20,
	})
	require.NoError(t, err)

	assert.Equal(t, "standard", result.MatchedTier)
	assert.InDelta(t, 3, result.PublishedPrices.Input, 1e-9)
	assert.InDelta(t, 15, result.PublishedPrices.Output, 1e-9)
	assert.InDelta(t, 0.3, result.PublishedPrices.CacheRead, 1e-9)
	assert.InDelta(t, 3.75, result.PublishedPrices.CacheCreate, 1e-9)
	assert.InDelta(t, 6, result.PublishedPrices.CacheCreate1h, 1e-9)
	assert.InDelta(t, 10, result.PublishedPrices.InputAudio, 1e-9)
	assert.InDelta(t, 0.0110675, result.ActualCost, 1e-12)
}
