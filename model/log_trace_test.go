package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAppendTraceID_UsesContextValue(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(common.TraceIDKey, "Trace123")

	other := appendTraceID(map[string]interface{}{"existing": "ok"}, ctx)
	require.Equal(t, "Trace123", other["trace_id"])
	require.Equal(t, "ok", other["existing"])
}

func TestAppendTraceID_WritesEmptyStringWhenMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	other := appendTraceID(nil, ctx)
	require.Equal(t, "", other["trace_id"])
}

func TestAppendTraceID_WritesEmptyStringForNilContext(t *testing.T) {
	t.Parallel()

	other := appendTraceID(nil, nil)
	require.Equal(t, "", other["trace_id"])
}

func TestRecordConsumeLog_PersistsTraceIDInOther(t *testing.T) {
	truncateTables(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(common.RequestIdKey, "req-123")
	ctx.Set(common.TraceIDKey, "TraceABC123")
	ctx.Set("username", "tester")

	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ChannelId:        10,
		PromptTokens:     1,
		CompletionTokens: 2,
		ModelName:        "gpt-test",
		TokenName:        "token-a",
		Quota:            3,
		Content:          "ok",
		TokenId:          4,
		UseTimeSeconds:   5,
		IsStream:         false,
		Group:            "default",
		Other:            map[string]interface{}{"existing": "yes"},
	})

	var logRow Log
	require.NoError(t, LOG_DB.Last(&logRow).Error)

	otherMap, err := common.StrToMap(logRow.Other)
	require.NoError(t, err)
	require.Equal(t, "TraceABC123", otherMap["trace_id"])
	require.Equal(t, "yes", otherMap["existing"])
	require.Equal(t, "req-123", logRow.RequestId)
}

func TestRecordErrorLog_PersistsTraceIDAlongsideTimeoutMetadata(t *testing.T) {
	truncateTables(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set(common.RequestIdKey, "req-err-1")
	ctx.Set(common.TraceIDKey, "TraceERR999")
	ctx.Set("username", "tester")
	ctx.Set("retry", 2)

	relaycommon.SetTimeoutMeta(ctx, relaycommon.TimeoutMeta{
		Type:     relaycommon.TimeoutTypeStreamFirstByte,
		Source:   relaycommon.TimeoutSourceChannelSetting,
		Seconds:  7,
		Provider: "aws",
		Stage:    "aws_stream_first_byte_wait",
		IsStream: true,
	})

	RecordErrorLog(ctx, 1, 10, "gpt-test", "token-a", "boom", 4, 5, true, "default", map[string]interface{}{
		"existing": "yes",
	})

	var logRow Log
	require.NoError(t, LOG_DB.Last(&logRow).Error)

	otherMap, err := common.StrToMap(logRow.Other)
	require.NoError(t, err)
	require.Equal(t, "TraceERR999", otherMap["trace_id"])
	require.Equal(t, "yes", otherMap["existing"])
	require.Equal(t, relaycommon.TimeoutTypeStreamFirstByte, otherMap["timeout_type"])
	require.Equal(t, "aws_stream_first_byte_wait", otherMap["timeout_stage"])
	require.Equal(t, "req-err-1", logRow.RequestId)
}

func TestRecordConsumeLog_PersistsEmptyTraceIDWhenMissing(t *testing.T) {
	truncateTables(t)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(common.RequestIdKey, "req-empty-trace")
	ctx.Set("username", "tester")

	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ChannelId: 1,
		ModelName: "gpt-test",
		TokenName: "token-a",
		Content:   "ok",
		Group:     "default",
	})

	var logRow Log
	require.NoError(t, LOG_DB.Last(&logRow).Error)

	otherMap, err := common.StrToMap(logRow.Other)
	require.NoError(t, err)
	require.Equal(t, "", otherMap["trace_id"])
}
