package billingexpr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

var (
	pricingSpecIntegerPattern = regexp.MustCompile(`\b\d{4,}\b`)
	pricingSpecLenPattern     = regexp.MustCompile(`\blen\b`)
	pricingSpecPromptPattern  = regexp.MustCompile(`\bp\b`)
	pricingSpecOutputPattern  = regexp.MustCompile(`\bc\b`)
)

// MatchedTierSpec returns the condition that selected a tier in a conditional
// billing expression. An unconditional tier has no limiting specification.
func MatchedTierSpec(exprString, matchedTier string) string {
	if strings.TrimSpace(exprString) == "" || strings.TrimSpace(matchedTier) == "" {
		return ""
	}
	_, body := ParseExprVersion(exprString)
	tree, err := parser.Parse(body)
	if err != nil {
		return ""
	}

	conditions, ok := findTierConditions(tree.Node, matchedTier, nil)
	if !ok {
		var result []string
		ast.Find(tree.Node, func(node ast.Node) bool {
			conditional, isConditional := node.(*ast.ConditionalNode)
			if !isConditional {
				return false
			}
			if found, matched := findTierConditions(conditional, matchedTier, nil); matched {
				result = found
				return true
			}
			return false
		})
		conditions = result
	}
	if len(conditions) == 0 {
		return ""
	}

	for index, condition := range conditions {
		conditions[index] = formatTierCondition(condition)
	}
	return strings.Join(conditions, "&&")
}

func findTierConditions(node ast.Node, matchedTier string, inherited []string) ([]string, bool) {
	conditional, ok := node.(*ast.ConditionalNode)
	if !ok {
		return inherited, containsTier(node, matchedTier)
	}

	if containsTier(conditional.Exp1, matchedTier) {
		path := append(append([]string{}, inherited...), conditional.Cond.String())
		if nestedPath, found := findTierConditions(conditional.Exp1, matchedTier, path); found {
			return nestedPath, true
		}
		return path, true
	}
	if containsTier(conditional.Exp2, matchedTier) {
		path := append(append([]string{}, inherited...), negateTierCondition(conditional.Cond))
		if nestedPath, found := findTierConditions(conditional.Exp2, matchedTier, path); found {
			return nestedPath, true
		}
		return path, true
	}
	return nil, false
}

func containsTier(node ast.Node, matchedTier string) bool {
	found := false
	ast.Find(node, func(candidate ast.Node) bool {
		call, ok := candidate.(*ast.CallNode)
		if !ok || len(call.Arguments) < 1 {
			return false
		}
		callee, ok := call.Callee.(*ast.IdentifierNode)
		if !ok || callee.Value != "tier" {
			return false
		}
		name, ok := call.Arguments[0].(*ast.StringNode)
		if ok && name.Value == matchedTier {
			found = true
			return true
		}
		return false
	})
	return found
}

func negateTierCondition(node ast.Node) string {
	binary, ok := node.(*ast.BinaryNode)
	if !ok {
		return "!(" + node.String() + ")"
	}
	inverse := map[string]string{
		"<":  ">=",
		"<=": ">",
		">":  "<=",
		">=": "<",
		"==": "!=",
		"!=": "==",
	}
	operator, ok := inverse[binary.Operator]
	if !ok {
		return "!(" + node.String() + ")"
	}
	return fmt.Sprintf("%s %s %s", binary.Left.String(), operator, binary.Right.String())
}

func formatTierCondition(condition string) string {
	condition = pricingSpecLenPattern.ReplaceAllString(condition, "input_tokens")
	condition = pricingSpecPromptPattern.ReplaceAllString(condition, "input_tokens")
	condition = pricingSpecOutputPattern.ReplaceAllString(condition, "output_tokens")
	condition = pricingSpecIntegerPattern.ReplaceAllStringFunc(condition, func(raw string) string {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return raw
		}
		if value%1_000_000 == 0 {
			return strconv.FormatInt(value/1_000_000, 10) + "m"
		}
		if value%1000 == 0 {
			return strconv.FormatInt(value/1000, 10) + "k"
		}
		return raw
	})
	return strings.ReplaceAll(condition, " ", "")
}
