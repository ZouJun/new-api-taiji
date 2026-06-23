package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenRouterSonarClientResponseExtractsCitationsAndSearchResults(t *testing.T) {
	base := dto.OpenAITextResponse{
		Id:      "resp_1",
		Model:   "perplexity/sonar",
		Object:  "chat.completion",
		Created: 123,
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0,
			Message: dto.Message{
				Content: "hello",
			},
			FinishReason: "stop",
		}},
		Usage: dto.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			Cost:             0.00552,
		},
	}

	rawBody := []byte(`{
		"choices": [
			{
				"message": {
					"annotations": [
						{"type":"url_citation","url_citation":{"url":"https://a.example","title":"A","start_index":0,"end_index":1}},
						{"type":"url_citation","url_citation":{"url":"https://b.example","title":"B","start_index":2,"end_index":3}}
					]
				}
			},
			{
				"message": {
					"annotations": [
						{"type":"url_citation","url_citation":{"url":"https://a.example","title":"A2","start_index":4,"end_index":5}}
					]
				}
			}
		],
		"usage": {
			"cost": 0.00552,
			"cost_details": {
				"upstream_inference_cost": 0.00552,
				"upstream_inference_prompt_cost": 0.000016,
				"upstream_inference_completions_cost": 0.005504
			}
		}
	}`)

	out, err := buildOpenRouterSonarClientResponse(base, rawBody)
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, []string{"https://a.example", "https://b.example"}, out.Citations)
	require.Len(t, out.SearchResults, 2)
	assert.Equal(t, "https://a.example", out.SearchResults[0].Url)
	assert.Equal(t, "A", out.SearchResults[0].Title)
	assert.Equal(t, "web", out.SearchResults[0].Source)
	assert.Equal(t, "https://b.example", out.SearchResults[1].Url)
}

func TestBuildOpenRouterSonarClientResponseDoesNotMutateBaseUsageCost(t *testing.T) {
	base := dto.OpenAITextResponse{
		Id:      "resp_1",
		Model:   "perplexity/sonar",
		Object:  "chat.completion",
		Created: 123,
		Usage: dto.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			Cost:             0.00552,
		},
	}

	rawBody := []byte(`{
		"usage": {
			"cost": 0.00552,
			"cost_details": {
				"upstream_inference_cost": 0.00552,
				"upstream_inference_prompt_cost": 0.000016,
				"upstream_inference_completions_cost": 0.005504
			}
		}
	}`)

	out, err := buildOpenRouterSonarClientResponse(base, rawBody)
	require.NoError(t, err)

	costFloat, ok := base.Usage.Cost.(float64)
	require.True(t, ok)
	assert.Equal(t, 0.00552, costFloat)

	body, err := common.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"total_cost":0.00552`)
	assert.Contains(t, string(body), `"input_tokens_cost":0.000016`)
	assert.Contains(t, string(body), `"output_tokens_cost":0.005504`)
}

func TestOpenaiHandlerWritesClientResponseForOpenRouterSonar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_1",
			"object":"chat.completion",
			"created":123,
			"model":"perplexity/sonar",
			"choices":[
				{
					"index":0,
					"message":{
						"role":"assistant",
						"content":"hello",
						"annotations":[
							{"type":"url_citation","url_citation":{"url":"https://a.example","title":"A","start_index":0,"end_index":1}}
						]
					},
					"finish_reason":"stop"
				}
			],
			"usage":{
				"prompt_tokens":10,
				"completion_tokens":5,
				"total_tokens":15,
				"cost":0.00552,
				"cost_details":{
					"upstream_inference_cost":0.00552,
					"upstream_inference_prompt_cost":0.000016,
					"upstream_inference_completions_cost":0.005504
				}
			}
		}`)),
	}

	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatOpenAI,
		UpstreamModelName: "perplexity/sonar",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
	}

	usage, err := OpenaiHandler(ctx, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)

	costFloat, ok := usage.Cost.(float64)
	require.True(t, ok)
	assert.Equal(t, 0.00552, costFloat)

	body := rec.Body.String()
	assert.Contains(t, body, `"citations":["https://a.example"]`)
	assert.Contains(t, body, `"search_results":[`)
	assert.Contains(t, body, `"total_cost":0.00552`)
}
