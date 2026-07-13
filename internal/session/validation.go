package session

import "fmt"

// TokenUsageIssue describes an internally inconsistent token counter reported
// by a transcript. InputTokens includes CachedInputTokens in Codex telemetry.
type TokenUsageIssue struct {
	Field   string
	Message string
}

// ValidateTokenUsage reports impossible token-counter combinations. A usage
// record with only TotalTokens is valid: some transcript events intentionally
// omit the billable input/output breakdown.
func ValidateTokenUsage(usage TokenUsage) []TokenUsageIssue {
	values := []struct {
		field string
		value int64
	}{
		{"input_tokens", usage.InputTokens},
		{"cached_input_tokens", usage.CachedInputTokens},
		{"output_tokens", usage.OutputTokens},
		{"reasoning_output_tokens", usage.ReasoningOutputTokens},
		{"total_tokens", usage.TotalTokens},
	}
	var issues []TokenUsageIssue
	for _, value := range values {
		if value.value < 0 {
			issues = append(issues, TokenUsageIssue{Field: value.field, Message: "must be non-negative"})
		}
	}
	if usage.InputTokens >= 0 && usage.CachedInputTokens > usage.InputTokens {
		issues = append(issues, TokenUsageIssue{Field: "cached_input_tokens", Message: fmt.Sprintf("must not exceed input_tokens (%d)", usage.InputTokens)})
	}
	if usage.OutputTokens >= 0 && usage.ReasoningOutputTokens > usage.OutputTokens {
		issues = append(issues, TokenUsageIssue{Field: "reasoning_output_tokens", Message: fmt.Sprintf("must not exceed output_tokens (%d)", usage.OutputTokens)})
	}
	hasBreakdown := usage.InputTokens != 0 || usage.CachedInputTokens != 0 || usage.OutputTokens != 0 || usage.ReasoningOutputTokens != 0
	if hasBreakdown && usage.InputTokens >= 0 && usage.OutputTokens >= 0 && usage.TotalTokens >= 0 && usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		issues = append(issues, TokenUsageIssue{Field: "total_tokens", Message: fmt.Sprintf("must equal input_tokens + output_tokens (%d)", usage.InputTokens+usage.OutputTokens)})
	}
	return issues
}
