package dto

type SearchResult struct {
	Title       string `json:"title,omitempty"`
	Url         string `json:"url,omitempty"`
	Date        string `json:"date,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	Source      string `json:"source,omitempty"`
}

type PerplexityStyleCost struct {
	InputTokensCost  float64 `json:"input_tokens_cost,omitempty"`
	OutputTokensCost float64 `json:"output_tokens_cost,omitempty"`
	RequestCost      float64 `json:"request_cost,omitempty"`
	TotalCost        float64 `json:"total_cost"`
}

type OpenAIClientUsage struct {
	PromptTokens                int                `json:"prompt_tokens"`
	CompletionTokens            int                `json:"completion_tokens"`
	TotalTokens                 int                `json:"total_tokens"`
	PromptCacheHitTokens        int                `json:"prompt_cache_hit_tokens,omitempty"`
	UsageSemantic               string             `json:"usage_semantic,omitempty"`
	UsageSource                 string             `json:"usage_source,omitempty"`
	PromptTokensDetails         InputTokenDetails  `json:"prompt_tokens_details"`
	CompletionTokenDetails      OutputTokenDetails `json:"completion_tokens_details"`
	InputTokens                 int                `json:"input_tokens"`
	OutputTokens                int                `json:"output_tokens"`
	InputTokensDetails          *InputTokenDetails `json:"input_tokens_details,omitempty"`
	ClaudeCacheCreation5mTokens int                `json:"claude_cache_creation_5_m_tokens,omitempty"`
	ClaudeCacheCreation1hTokens int                `json:"claude_cache_creation_1_h_tokens,omitempty"`
	Cost                        any                `json:"cost,omitempty"`
}

type OpenAIClientTextResponse struct {
	Id            string                     `json:"id"`
	Model         string                     `json:"model"`
	Object        string                     `json:"object"`
	Created       any                        `json:"created"`
	Choices       []OpenAITextResponseChoice `json:"choices"`
	Error         any                        `json:"error,omitempty"`
	Usage         OpenAIClientUsage          `json:"usage"`
	Citations     []string                   `json:"citations,omitempty"`
	SearchResults []SearchResult             `json:"search_results,omitempty"`
}
