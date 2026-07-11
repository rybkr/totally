package pricing

import (
	"slices"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
)

func TestDefaultCatalogContainsSupportedCodexAndRelatedModels(t *testing.T) {
	rates := DefaultCatalog().Rates()
	models := make([]string, 0, len(rates))
	for _, rate := range rates {
		if rate.Provider != "openai" {
			t.Fatalf("unexpected provider in default catalog: %+v", rate)
		}
		models = append(models, rate.Model)
	}
	for _, model := range []string{
		"codex-mini-latest",
		"gpt-4.1",
		"gpt-5",
		"gpt-5-codex",
		"gpt-5.1-codex",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.5",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"o3",
		"o4-mini",
	} {
		if !slices.Contains(models, model) {
			t.Errorf("default catalog is missing %q", model)
		}
	}
}

func TestEstimateAppliesLongContextPricingPerRequest(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{
		Provider: "test", Model: "long", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "0.1", OutputPerMillionUSD: "2",
		LongContextThreshold: 272_000, LongContextInputScale: "2", LongContextOutputScale: "1.5",
	}); err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{
		{Provider: "test", Model: "long", TokenUsage: session.TokenUsage{InputTokens: 300_000, OutputTokens: 100_000}},
		{Provider: "test", Model: "long", TokenUsage: session.TokenUsage{InputTokens: 100_000, OutputTokens: 100_000}},
	}, time.Time{})
	// First request: .6 input + .3 output. Second request: .1 input + .2 output.
	if estimate.AmountUSD == nil || *estimate.AmountUSD != "1.2" {
		t.Fatalf("unexpected long-context estimate: %+v", estimate)
	}
}

func TestEstimateMarksUnknownCacheWriteSurchargePartial(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{
		Provider: "test", Model: "writes", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "0.1", OutputPerMillionUSD: "2", CacheWriteInputScale: "1.25",
	}); err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{{Provider: "test", Model: "writes", TokenUsage: session.TokenUsage{InputTokens: 1_000_000}}}, time.Time{})
	if estimate.Status != "partial" || len(estimate.Limitations) != 1 {
		t.Fatalf("unexpected cache-write estimate: %+v", estimate)
	}
}

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
