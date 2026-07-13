package billingexpr

import (
	"math"

	"github.com/QuantumNous/new-api/common"
)

// quotaConversion converts raw expression output to quota based on the
// expression version. This is the central dispatch point for future versions
// that may use a different conversion formula.
func quotaConversion(exprOutput float64, snap *BillingSnapshot) float64 {
	switch snap.ExprVersion {
	default: // v1: coefficients are $/1M tokens prices
		return exprOutput / 1_000_000 * snap.QuotaPerUnit
	}
}

// ComputeTieredQuota runs the Expr from a frozen BillingSnapshot against
// actual token counts and returns the settlement result.
func ComputeTieredQuota(snap *BillingSnapshot, params TokenParams) (TieredResult, error) {
	return ComputeTieredQuotaWithRequest(snap, params, RequestInput{})
}

func ComputeTieredQuotaWithRequest(snap *BillingSnapshot, params TokenParams, request RequestInput) (TieredResult, error) {
	cost, trace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return TieredResult{}, err
	}

	publishedPrices := TieredPublishedPrices{
		Input: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.P += delta
		}),
		Output: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.C += delta
		}),
		CacheRead: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.CR += delta
		}),
		CacheCreate: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.CC += delta
		}),
		CacheCreate1h: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.CC1h += delta
		}),
		InputAudio: tieredMarginalPrice(snap, params, request, trace.MatchedTier, func(value *TokenParams, delta float64) {
			value.AI += delta
		}),
	}
	quotaBeforeGroup := quotaConversion(cost, snap)
	afterGroup, clamp := common.QuotaRoundChecked(quotaBeforeGroup * snap.GroupRatio)
	crossed := trace.MatchedTier != snap.EstimatedTier

	return TieredResult{
		ActualQuotaBeforeGroup: quotaBeforeGroup,
		ActualQuotaAfterGroup:  afterGroup,
		ActualCost:             cost / 1_000_000,
		MatchedTier:            trace.MatchedTier,
		CrossedTier:            crossed,
		PublishedPrices:        publishedPrices,
		Clamp:                  clamp,
	}, nil
}

func tieredMarginalPrice(
	snap *BillingSnapshot,
	params TokenParams,
	request RequestInput,
	matchedTier string,
	adjust func(*TokenParams, float64),
) float64 {
	baseCost, _, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return 0
	}

	increased := params
	adjust(&increased, 1)
	increasedCost, increasedTrace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, increased, request)
	if err == nil && increasedTrace.MatchedTier == matchedTier {
		return validPublishedPrice(increasedCost - baseCost)
	}

	decreased := params
	adjust(&decreased, -1)
	decreasedCost, decreasedTrace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, decreased, request)
	if err == nil && decreasedTrace.MatchedTier == matchedTier {
		return validPublishedPrice(baseCost - decreasedCost)
	}
	return 0
}

func validPublishedPrice(price float64) float64 {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0
	}
	return price
}
