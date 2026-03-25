package playbook

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
)

// Interpret evaluates the result of a step against its interpretation rules.
// Returns (verdict, message). Scope "first" only for MVP (C3).
func Interpret(spec InterpretationSpec, columns []string, rows [][]any, rowCount int) (string, string) {
	// 1. Check row count rules first.
	for _, rule := range spec.RowCountRules {
		if evaluateNumeric(float64(rowCount), rule.Operator, rule.Value) {
			return rule.Verdict, expandTemplate(rule.Message, map[string]any{"row_count": rowCount})
		}
	}

	// 2. Check column-based rules against the first row only (C3: scope "first" only).
	if len(rows) > 0 {
		rowMap := zipColumnsAndRow(columns, rows[0])
		for _, rule := range spec.Rules {
			// C3: Log warning if scope is "any" or "all", fallback to "first".
			if rule.Scope != "" && rule.Scope != "first" {
				slog.Warn("interpretation scope not supported in MVP, falling back to first",
					"scope", rule.Scope)
			}

			val, ok := rowMap[rule.Column]
			if !ok {
				continue
			}
			if evaluateCondition(val, rule.Operator, rule.Value) {
				return rule.Verdict, expandTemplate(rule.Message, rowMap)
			}
		}
	}

	// 3. Default.
	return spec.DefaultVerdict, spec.DefaultMessage
}

// EvaluateBranch determines the next step based on branch rules and the current result.
func EvaluateBranch(step Step, verdict string, columns []string, rows [][]any) int {
	if len(step.BranchRules) == 0 {
		if step.NextStepDefault != nil {
			return *step.NextStepDefault
		}
		return 0 // playbook complete
	}

	var rowMap map[string]any
	if len(rows) > 0 {
		rowMap = zipColumnsAndRow(columns, rows[0])
	}

	for _, br := range step.BranchRules {
		if matchBranchCondition(br.Condition, verdict, rowMap) {
			return br.GotoStep
		}
	}

	if step.NextStepDefault != nil {
		return *step.NextStepDefault
	}
	return 0
}

func matchBranchCondition(cond BranchCondition, verdict string, rowMap map[string]any) bool {
	// Verdict-based branch.
	if cond.Verdict != "" {
		return cond.Verdict == verdict
	}
	// Column-based branch.
	if cond.Column != "" && rowMap != nil {
		val, ok := rowMap[cond.Column]
		if !ok {
			return false
		}
		return evaluateCondition(val, cond.Operator, cond.Value)
	}
	return false
}

func zipColumnsAndRow(columns []string, row []any) map[string]any {
	m := make(map[string]any, len(columns))
	for i, col := range columns {
		if i < len(row) {
			m[col] = row[i]
		}
	}
	return m
}

func evaluateCondition(actual any, operator string, expected any) bool {
	switch operator {
	case "is_null":
		return actual == nil
	case "is_not_null":
		return actual != nil
	}

	if actual == nil {
		return false
	}

	return evaluateNumeric(toFloat64(actual), operator, expected)
}

func evaluateNumeric(actual float64, operator string, expected any) bool {
	exp := toFloat64(expected)
	switch operator {
	case ">":
		return actual > exp
	case "<":
		return actual < exp
	case ">=":
		return actual >= exp
	case "<=":
		return actual <= exp
	case "==":
		return actual == exp
	case "!=":
		return actual != exp
	default:
		return false
	}
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case int16:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	case json_number:
		f, _ := val.Float64()
		return f
	default:
		s := fmt.Sprintf("%v", v)
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
}

// json_number is a type alias to match encoding/json Number values.
type json_number interface {
	Float64() (float64, error)
}

var templateRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

func expandTemplate(tpl string, values map[string]any) string {
	return templateRe.ReplaceAllStringFunc(tpl, func(match string) string {
		key := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		if val, ok := values[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}
