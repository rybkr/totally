package pricing

import (
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
)

func refreshRate(t *testing.T, provider, model, date, mode, geography string) Rate {
	t.Helper()
	at, err := time.Parse(time.DateOnly, date)
	if err != nil {
		t.Fatal(err)
	}
	rate, ok := DefaultCatalog().lookup(provider, model, rateSelector{"first_party", mode, geography}, at)
	if !ok {
		t.Fatalf("missing %s/%s at %s (%s, %s)", provider, model, date, mode, geography)
	}
	return rate
}

func TestOctoberCatalogHistoricalBoundaries(t *testing.T) {
	for _, tt := range []struct {
		provider, model, date, input, read, write, output string
	}{
		{"anthropic", "claude-sonnet-5", "2026-08-31", "2", "0.2", "", "10"},
		{"anthropic", "claude-sonnet-5", "2026-09-01", "2", "0.2", "", "10"},
		{"anthropic", "claude-sonnet-5-5", "2026-10-06", "2", "0.2", "", "10"},
		{"anthropic", "claude-sonnet-5-5", "2026-10-07", "2", "0.1", "", "10"},
		{"openai", "gpt-5.6-sol", "2026-08-20", "5.00", "0.50", "6.25", "30.00"},
		{"openai", "gpt-5.6-sol", "2026-08-21", "4", "0.4", "5", "20"},
		{"openai", "gpt-5.6", "2026-07-09", "5.00", "0.50", "6.25", "30.00"},
		{"openai", "gpt-5.6", "2026-08-21", "4", "0.4", "5", "20"},
		{"openai", "gpt-5.6-terra", "2026-07-29", "2.50", "0.25", "3.125", "15.00"},
		{"openai", "gpt-5.6-terra", "2026-07-30", "2", "0.2", "2.5", "12"},
		{"openai", "gpt-5.6-luna", "2026-07-29", "1.00", "0.10", "1.25", "6.00"},
		{"openai", "gpt-5.6-luna", "2026-07-30", "0.2", "0.02", "0.25", "1.2"},
		{"openai", "gpt-6.1-sol", "2026-09-29", "2", "0.1", "2.5", "10"},
		{"openai", "gpt-rosalind-research", "2026-10-05", "5", "0.5", "", "25"},
	} {
		t.Run(tt.model+"/"+tt.date, func(t *testing.T) {
			r := refreshRate(t, tt.provider, tt.model, tt.date, "standard", "global")
			if r.InputPerMillionUSD != tt.input || r.CachedInputPerMillionUSD != tt.read || r.CacheWritePerMillionUSD != tt.write || r.OutputPerMillionUSD != tt.output {
				t.Fatalf("unexpected historical rate: %+v", r)
			}
		})
	}
}

func TestOctoberCatalogDoesNotBackdateUnverifiedPrices(t *testing.T) {
	for _, tt := range []struct{ provider, model, date, mode string }{
		{"openai", "gpt-6.1-sol", "2026-09-28", "standard"},
		{"openai", "gpt-6-astra", "2026-10-15", "standard"},
		{"openai", "gpt-6-sol", "2026-10-15", "standard"},
		{"openai", "gpt-6-luna", "2026-10-15", "standard"},
		{"openai", "gpt-6.1-sol", "2026-10-15", "ultrafast"},
		{"openai", "gpt-5.6-sol", "2026-07-30", "priority"},
		{"openai", "gpt-5.6-luna", "2026-07-30", "batch"},
		{"openai", "gpt-rosalind-research", "2026-10-04", "standard"},
		{"anthropic", "claude-opus-5", "2026-07-23", "standard"},
		{"anthropic", "claude-haiku-5-5", "2026-10-06", "standard"},
		{"anthropic", "claude-haiku-5-5", "2026-10-15", "batch"},
	} {
		at, _ := time.Parse(time.DateOnly, tt.date)
		if _, ok := DefaultCatalog().lookup(tt.provider, tt.model, rateSelector{"first_party", tt.mode, "global"}, at); ok {
			t.Errorf("unverified rate matched: %+v", tt)
		}
	}
}

func TestOctoberCatalogComposesOnlyTokenModifiers(t *testing.T) {
	for _, tt := range []struct {
		provider, model, mode, geo, input, read, write, write5m, write1h, output string
	}{
		{"anthropic", "claude-haiku-5-5", "batch", "us", "0.055", "0.0055", "", "0.06875", "0.11", "0.275"},
		{"anthropic", "claude-opus-5-5", "fast", "us", "8.8", "0.44", "", "11", "17.6", "44"},
		{"openai", "gpt-6.1-sol", "ultrafast", "regional", "13.2", "0.66", "16.5", "", "", "66"},
		{"openai", "gpt-6-luna", "flex", "regional", "0.055", "0.0055", "0.06875", "", "", "0.275"},
		{"openai", "gpt-5.4-nano-2026-03-17", "batch", "regional", "0.11", "0.011", "", "", "", "0.6875"},
	} {
		r := refreshRate(t, tt.provider, tt.model, "2026-10-16", tt.mode, tt.geo)
		if r.InputPerMillionUSD != tt.input || r.CachedInputPerMillionUSD != tt.read || r.CacheWritePerMillionUSD != tt.write || r.CacheWrite5mPerMillionUSD != tt.write5m || r.CacheWrite1hPerMillionUSD != tt.write1h || r.OutputPerMillionUSD != tt.output {
			t.Errorf("unexpected composed rate for %s: %+v", tt.model, r)
		}
		if r.WebSearchPerThousandUSD != "10" {
			t.Errorf("token modifiers changed search fee for %s: %s", tt.model, r.WebSearchPerThousandUSD)
		}
	}
}

func TestHaiku55ThresholdScalesAllTokenMeters(t *testing.T) {
	r := refreshRate(t, "anthropic", "claude-haiku-5-5", "2026-10-07", "standard", "global")
	base := MeterRates{Input: 100_000_000, CachedInput: 10_000_000, CacheWrite5m: 125_000_000, CacheWrite1h: 200_000_000, Output: 500_000_000}
	for _, input := range []int64{100_000, 100_001} {
		got, err := applyRules(r, PricingContext{InputTokens: input, CachedInputTokens: input - 1}, base)
		if err != nil {
			t.Fatal(err)
		}
		want := base
		if input > 100_000 {
			want = MeterRates{Input: 500_000_000, CachedInput: 50_000_000, CacheWrite5m: 625_000_000, CacheWrite1h: 1_000_000_000, Output: 2_500_000_000}
		}
		if got != want {
			t.Errorf("input %d: rates %+v, want %+v", input, got, want)
		}
	}
	// The threshold uses inclusive input, including reads, per request. Two
	// 100K requests stay in the low tier even though the session exceeds 100K.
	at, _ := time.Parse(time.DateOnly, "2026-10-07")
	usage := session.UsageSegment{Provider: "anthropic", Model: "claude-haiku-5-5", TokenUsage: session.TokenUsage{InputTokens: 100_000, CachedInputTokens: 100_000, OutputTokens: 1_000}}
	estimate := DefaultCatalog().Estimate([]session.UsageSegment{usage, usage}, at)
	if estimate.AmountUSD == nil || *estimate.AmountUSD != "0.003" {
		t.Fatalf("threshold was not applied per request: %+v", estimate)
	}
	usage.TokenUsage.InputTokens++
	usage.TokenUsage.CachedInputTokens++
	estimate = DefaultCatalog().Estimate([]session.UsageSegment{usage}, at)
	if estimate.AmountUSD == nil || *estimate.AmountUSD != "0.00750005" {
		t.Fatalf("cached input did not trigger all-request premium: %+v", estimate)
	}
}

func TestGPT61LongContextIncludesCacheWrites(t *testing.T) {
	r := refreshRate(t, "openai", "gpt-6.1-sol", "2026-09-29", "standard", "global")
	base := MeterRates{Input: 2_000_000_000, CachedInput: 100_000_000, CacheWrite: 2_500_000_000, Output: 10_000_000_000}
	for _, input := range []int64{272_000, 272_001} {
		got, err := applyRules(r, PricingContext{InputTokens: input}, base)
		if err != nil {
			t.Fatal(err)
		}
		want := base
		if input > 272_000 {
			want = MeterRates{Input: 4_000_000_000, CachedInput: 200_000_000, CacheWrite: 5_000_000_000, Output: 15_000_000_000}
		}
		if got != want {
			t.Errorf("input %d: rates %+v, want %+v", input, got, want)
		}
	}
}

func TestProcessingModesAgreeForBundledAndOverrideRates(t *testing.T) {
	for _, tt := range []struct {
		provider, mode string
		valid          bool
	}{
		{"openai", "fast", true}, {"openai", "ultrafast", true}, {"openai", "priority", true},
		{"anthropic", "fast", true}, {"anthropic", "ultrafast", false}, {"anthropic", "priority", false},
		{"openai", "unknown", false},
	} {
		r := Rate{Provider: tt.provider, ProcessingMode: tt.mode}
		bundledErr := validateBundledSelector(tt.provider, bundledSelector{Platform: "first_party", ProcessingMode: tt.mode, InferenceGeography: "global"})
		if (bundledErr == nil) != tt.valid || (len(validateRateSelector(r)) == 0) != tt.valid {
			t.Errorf("inconsistent processing-mode validation: %+v", tt)
		}
	}
}
