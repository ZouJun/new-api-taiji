package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

type openRouterURLCitation struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
}

type openRouterAnnotation struct {
	Type        string                 `json:"type"`
	URLCitation *openRouterURLCitation `json:"url_citation,omitempty"`
}

type openRouterSonarPayload struct {
	Choices []struct {
		Message struct {
			Annotations []openRouterAnnotation `json:"annotations"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		Cost        *float64 `json:"cost"`
		CostDetails struct {
			UpstreamInferenceCost            *float64 `json:"upstream_inference_cost"`
			UpstreamInferencePromptCost      *float64 `json:"upstream_inference_prompt_cost"`
			UpstreamInferenceCompletionsCost *float64 `json:"upstream_inference_completions_cost"`
		} `json:"cost_details"`
	} `json:"usage"`
}

func isOpenRouterSonarModel(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.HasPrefix(name, "sonar")
}

func buildOpenRouterSonarClientResponse(base dto.OpenAITextResponse, rawBody []byte) (*dto.OpenAIClientTextResponse, error) {
	var payload openRouterSonarPayload
	if err := common.Unmarshal(rawBody, &payload); err != nil {
		return nil, err
	}

	citations, results := extractSonarCitationsAndResults(payload)
	out := &dto.OpenAIClientTextResponse{
		Id:      base.Id,
		Model:   base.Model,
		Object:  base.Object,
		Created: base.Created,
		Choices: base.Choices,
		Error:   base.Error,
		Usage:   cloneUsageForClient(base.Usage),
	}
	if len(citations) > 0 {
		out.Citations = citations
	}
	if len(results) > 0 {
		out.SearchResults = results
	}
	out.Usage.Cost = buildPerplexityStyleCost(payload, base.Usage.Cost)
	return out, nil
}

func cloneUsageForClient(usage dto.Usage) dto.OpenAIClientUsage {
	return dto.OpenAIClientUsage{
		PromptTokens:                usage.PromptTokens,
		CompletionTokens:            usage.CompletionTokens,
		TotalTokens:                 usage.TotalTokens,
		PromptCacheHitTokens:        usage.PromptCacheHitTokens,
		UsageSemantic:               usage.UsageSemantic,
		UsageSource:                 usage.UsageSource,
		PromptTokensDetails:         usage.PromptTokensDetails,
		CompletionTokenDetails:      usage.CompletionTokenDetails,
		InputTokens:                 usage.InputTokens,
		OutputTokens:                usage.OutputTokens,
		InputTokensDetails:          usage.InputTokensDetails,
		ClaudeCacheCreation5mTokens: usage.ClaudeCacheCreation5mTokens,
		ClaudeCacheCreation1hTokens: usage.ClaudeCacheCreation1hTokens,
		Cost:                        usage.Cost,
	}
}

func extractSonarCitationsAndResults(payload openRouterSonarPayload) ([]string, []dto.SearchResult) {
	citations := make([]string, 0)
	results := make([]dto.SearchResult, 0)
	seenCitation := make(map[string]struct{})
	seenResult := make(map[string]struct{})

	for _, choice := range payload.Choices {
		for _, annotation := range choice.Message.Annotations {
			if annotation.Type != "url_citation" || annotation.URLCitation == nil {
				continue
			}
			url := strings.TrimSpace(annotation.URLCitation.URL)
			if url == "" {
				continue
			}
			if _, ok := seenCitation[url]; !ok {
				seenCitation[url] = struct{}{}
				citations = append(citations, url)
			}
			if _, ok := seenResult[url]; ok {
				continue
			}
			seenResult[url] = struct{}{}
			results = append(results, dto.SearchResult{
				Title:  strings.TrimSpace(annotation.URLCitation.Title),
				Url:    url,
				Source: "web",
			})
		}
	}

	return citations, results
}

func buildPerplexityStyleCost(payload openRouterSonarPayload, fallback any) any {
	if payload.Usage.Cost == nil &&
		payload.Usage.CostDetails.UpstreamInferenceCost == nil &&
		payload.Usage.CostDetails.UpstreamInferencePromptCost == nil &&
		payload.Usage.CostDetails.UpstreamInferenceCompletionsCost == nil {
		return fallback
	}

	out := dto.PerplexityStyleCost{}
	if payload.Usage.Cost != nil {
		out.TotalCost = *payload.Usage.Cost
	} else if payload.Usage.CostDetails.UpstreamInferenceCost != nil {
		out.TotalCost = *payload.Usage.CostDetails.UpstreamInferenceCost
	}
	if payload.Usage.CostDetails.UpstreamInferencePromptCost != nil {
		out.InputTokensCost = *payload.Usage.CostDetails.UpstreamInferencePromptCost
	}
	if payload.Usage.CostDetails.UpstreamInferenceCompletionsCost != nil {
		out.OutputTokensCost = *payload.Usage.CostDetails.UpstreamInferenceCompletionsCost
	}
	if payload.Usage.CostDetails.UpstreamInferenceCost != nil {
		requestCost := *payload.Usage.CostDetails.UpstreamInferenceCost - out.InputTokensCost - out.OutputTokensCost
		if requestCost > 0 {
			out.RequestCost = requestCost
		}
	}

	return out
}
