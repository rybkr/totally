package pricing

import (
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
)

func TestEstimateDoesNotDoubleChargeCachedOrReasoningTokens(t *testing.T) {
	catalog := Catalog{}
	err := catalog.Override(Rate{Provider: "test", Model: "model", InputPerMillionUSD: "2", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "4"})
	if err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{{
		Provider: "test", Model: "model",
		TokenUsage: session.TokenUsage{InputTokens: 1_000_000, CachedInputTokens: 250_000, OutputTokens: 500_000, ReasoningOutputTokens: 100_000},
	}}, time.Time{})
	if estimate.AmountUSD == nil || *estimate.AmountUSD != "3.75" {
		t.Fatalf("unexpected estimate: %+v", estimate)
	}
	if estimate.Status != "complete" {
		t.Fatalf("unexpected status: %s", estimate.Status)
	}
}

func TestEstimateReportsPartialAndUnavailablePricing(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{Provider: "test", Model: "known", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1"}); err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{
		{Provider: "test", Model: "known", TokenUsage: session.TokenUsage{InputTokens: 1_000_000}},
		{Provider: "test", Model: "unknown", TokenUsage: session.TokenUsage{InputTokens: 1}},
	}, time.Time{})
	if estimate.Status != "partial" || estimate.AmountUSD == nil || *estimate.AmountUSD != "1" || len(estimate.Missing) != 1 {
		t.Fatalf("unexpected estimate: %+v", estimate)
	}

	unavailable := catalog.Estimate([]session.UsageSegment{{Provider: "test", Model: "unknown", TokenUsage: session.TokenUsage{InputTokens: 1}}}, time.Time{})
	if unavailable.Status != "unavailable" || unavailable.AmountUSD != nil {
		t.Fatalf("unexpected unavailable estimate: %+v", unavailable)
	}
}
