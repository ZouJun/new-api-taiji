package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectArchiveRequestHeaderIncludesMissingValues(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	other := injectArchiveRequestHeader(ctx, nil)
	raw, ok := other["request_header"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "trace-123", raw["X-Trace-Id"])
	assert.Equal(t, "", raw["X-Request-ID"])
	assert.Equal(t, "", raw["Routify-Provider-Color-Id"])
}

func TestInjectConsumeLogUpstreamUsageFallsBackWithProviderAndModel(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set("base_url", "https://provider.example")
	ctx.Set("original_model", "gpt-test")

	other := injectConsumeLogUpstreamUsage(ctx, nil)
	raw, ok := other["upstream_usage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "missing", raw["source"])
	assert.Equal(t, false, raw["complete"])
	assert.Equal(t, "https://provider.example", raw["provider"])
	assert.Equal(t, "gpt-test", raw["model"])
}

func TestInjectConsumeLogUpstreamUsageUsesContextValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetConsumeLogUpstreamUsage(ctx, map[string]interface{}{
		"source":   "estimated",
		"complete": false,
	})

	other := injectConsumeLogUpstreamUsage(ctx, nil)
	raw, ok := other["upstream_usage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "estimated", raw["source"])
	assert.Equal(t, false, raw["complete"])
}
