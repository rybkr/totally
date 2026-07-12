package session

import "testing"

func TestValidateTokenUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage TokenUsage
		want  int
	}{
		{"valid breakdown", TokenUsage{InputTokens: 100, CachedInputTokens: 40, OutputTokens: 25, ReasoningOutputTokens: 5, TotalTokens: 125}, 0},
		{"total only is allowed", TokenUsage{TotalTokens: 5_003}, 0},
		{"cached input exceeds input", TokenUsage{InputTokens: 10, CachedInputTokens: 11, TotalTokens: 10}, 1},
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
