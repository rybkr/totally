package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestValidateTokenUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage TokenUsage
		want  int
	}{
		{"valid breakdown", TokenUsage{InputTokens: 100, CachedInputTokens: 40, OutputTokens: 25, ReasoningOutputTokens: 5, TotalTokens: 125}, 0},
		{"total only is allowed", TokenUsage{TotalTokens: 5_003}, 0},
		{"cached input exceeds input", TokenUsage{InputTokens: 10, CachedInputTokens: 11, TotalTokens: 10}, 1},
		{"reasoning output exceeds output", TokenUsage{InputTokens: 10, OutputTokens: 2, ReasoningOutputTokens: 3, TotalTokens: 12}, 1},
		{"inconsistent total", TokenUsage{InputTokens: 10, OutputTokens: 2, TotalTokens: 11}, 1},
		{"negative counter", TokenUsage{InputTokens: -1}, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(ValidateTokenUsage(test.usage)); got != test.want {
				t.Fatalf("ValidateTokenUsage(%+v) returned %d issues, want %d", test.usage, got, test.want)
			}
		})
	}
}

func TestUsageSegmentOccurredAtJSONOmitsZero(t *testing.T) {
	encoded, err := json.Marshal(UsageSegment{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "occurred_at") {
		t.Fatalf("zero occurrence time was not omitted: %s", encoded)
	}

	occurredAt := time.Date(2026, time.July, 9, 3, 20, 46, 0, time.UTC)
	encoded, err = json.Marshal(UsageSegment{OccurredAt: occurredAt})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"occurred_at":"2026-07-09T03:20:46Z"`) {
		t.Fatalf("non-zero occurrence time was not encoded: %s", encoded)
	}
}
