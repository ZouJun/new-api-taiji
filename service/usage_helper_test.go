package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamUsageLogNormalizesSource(t *testing.T) {
	t.Parallel()

	relayInfo := &relaycommon.RelayInfo{UpstreamModelName: "gpt-test"}
	usage := &dto.Usage{
		PromptTokens:     1,
		CompletionTokens: 2,
		TotalTokens:      3,
		UsageSource:      "anthropic",
	}

	result := BuildUpstreamUsageLog(relayInfo, usage)

	assert.Equal(t, common.UpstreamUsageSourceProviderTransformed, result["source"])
	assert.Equal(t, common.UpstreamUsageSourceProviderTransformed, result["usage_source"])
	assert.Equal(t, "anthropic", result["provider_usage_source"])

	raw, ok := result["raw"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, common.UpstreamUsageSourceProviderTransformed, raw["usage_source"])
}

func TestAttachConsumeLogUpstreamUsagePrefersClientFacingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetConsumeLogClientUsage(ctx, map[string]any{
		"input_tokens":  11,
		"output_tokens": 7,
	})

	result := AttachConsumeLogUpstreamUsage(ctx, &relaycommon.RelayInfo{UpstreamModelName: "claude-3"}, &dto.Usage{
		PromptTokens:     3,
		CompletionTokens: 5,
		TotalTokens:      8,
		UsageSource:      common.UpstreamUsageSourceEstimated,
	})

	raw, ok := result["raw"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(11), raw["input_tokens"])
	assert.Equal(t, float64(7), raw["output_tokens"])
	assert.NotContains(t, raw, "prompt_tokens")
}
