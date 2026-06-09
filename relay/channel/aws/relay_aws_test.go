package aws

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620",
		IsStream:                  false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
		},
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}

func TestNewAwsClient_AppliesRetryMaxAttemptsFromChannelSetting(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	maxAttempts := 6
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "access-key|secret-key|us-east-1",
			ChannelSetting: dto.ChannelSettings{
				AwsSDKMaxAttempts: &maxAttempts,
			},
		},
	}

	client, err := newAwsClient(ctx, info)
	require.NoError(t, err)
	require.Equal(t, maxAttempts, client.Options().RetryMaxAttempts)
}

func TestNewAwsClient_AppliesRetryMaxAttemptsFromGlobalConfig(t *testing.T) {
	t.Parallel()

	oldMaxAttempts := common.AWSSDKMaxAttempts
	common.AWSSDKMaxAttempts = 5
	defer func() {
		common.AWSSDKMaxAttempts = oldMaxAttempts
	}()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "access-key|secret-key|us-east-1",
		},
	}

	client, err := newAwsClient(ctx, info)
	require.NoError(t, err)
	require.Equal(t, 5, client.Options().RetryMaxAttempts)
}
