package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
