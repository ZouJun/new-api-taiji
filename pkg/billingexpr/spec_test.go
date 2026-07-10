package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchedTierSpec(t *testing.T) {
	expression := `len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long_context", p * 6 + c * 22.5)`

	assert.Equal(t, "input_tokens<=200k", MatchedTierSpec(expression, "standard"))
	assert.Equal(t, "input_tokens>200k", MatchedTierSpec(expression, "long_context"))
}

func TestMatchedTierSpecNestedConditions(t *testing.T) {
	expression := `
		p < 32000 && c < 200 ? tier("short", p * 2 + c * 8) :
		p < 32000 && c >= 200 ? tier("long_output", p * 3 + c * 14) :
		tier("long_input", p * 4 + c * 16)
	`

	assert.Equal(
		t,
		"!(input_tokens<32k&&output_tokens<200)&&input_tokens<32k&&output_tokens>=200",
		MatchedTierSpec(expression, "long_output"),
	)
}

func TestMatchedTierSpecUnconditionalTier(t *testing.T) {
	assert.Empty(t, MatchedTierSpec(`tier("default", p * 2 + c * 10)`, "default"))
}
