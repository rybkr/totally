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
		"gpt-5.4-mini",
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

func TestDefaultCatalogIsLoadedFromEmbeddedManifestAndCards(t *testing.T) {
	manifest := decodeBundledTOML[bundledManifest]("catalogs/catalog.toml")
	if CatalogVersion != manifest.CatalogVersion {
		t.Fatalf("catalog version = %q, manifest version = %q", CatalogVersion, manifest.CatalogVersion)
	}

	var luna Rate
	for _, rate := range DefaultCatalog().Rates() {
		if rate.Provider == "openai" && rate.Model == "gpt-5.6-luna" {
			luna = rate
			break
		}
	}
	if luna.InputPerMillionUSD != "1.00" || luna.CacheWritePerMillionUSD != "1.25" || len(luna.Rules) != 1 {
		t.Fatalf("embedded gpt-5.6-luna card was not translated correctly: %+v", luna)
	}
}

func TestLookupHonorsEffectiveUntil(t *testing.T) {
	catalog := Catalog{rates: []Rate{{
		Provider: "test", Model: "model", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1",
		EffectiveFrom: "2026-01-01", EffectiveUntil: "2026-02-01",
	}}}
	if _, ok := catalog.lookup("test", "model", time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)); !ok {
		t.Fatal("rate should apply before effective_until")
	}
	if _, ok := catalog.lookup("test", "model", time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("rate should not apply at effective_until")
	}
}

func TestLookupEarlyAccessUsesEarliestKnownRateOnlyBeforeFirstSchedule(t *testing.T) {
	catalog := Catalog{rates: []Rate{
		{Provider: "test", Model: "model", EffectiveFrom: "2026-02-01", EffectiveUntil: "2026-03-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1"},
		{Provider: "test", Model: "model", EffectiveFrom: "2026-03-01", InputPerMillionUSD: "2", CachedInputPerMillionUSD: "2", OutputPerMillionUSD: "2"},
	}}
	beforeFirst, _ := time.Parse(time.DateOnly, "2026-01-01")
	if _, ok := catalog.lookup("test", "model", beforeFirst); ok {
		t.Fatal("early access must be disabled by default")
	}
	catalog.SetEarlyAccess(true)
	if rate, ok := catalog.lookup("test", "model", beforeFirst); !ok || rate.InputPerMillionUSD != "1" {
		t.Fatalf("early-access rate = %+v, %v; want earliest rate", rate, ok)
	}
	atSecond, _ := time.Parse(time.DateOnly, "2026-03-15")
	if rate, ok := catalog.lookup("test", "model", atSecond); !ok || rate.InputPerMillionUSD != "2" {
		t.Fatalf("rate after scheduled change = %+v, %v; want second rate", rate, ok)
	}
}

func TestEstimateAppliesLongContextPricingPerRequest(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{
		Provider: "test", Model: "long", EffectiveFrom: "2020-01-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "0.1", OutputPerMillionUSD: "2",
		Rules: []PricingRule{LongContextRule{Type: "long_context", ThresholdTokens: 272_000, InputScale: "2", OutputScale: "1.5"}},
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

func TestOverlayPreservesSurroundingModelHistory(t *testing.T) {
	catalog := Catalog{rates: []Rate{
		{Provider: "test", Model: "model", EffectiveFrom: "2025-01-01", EffectiveUntil: "2025-06-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1"},
		{Provider: "test", Model: "model", EffectiveFrom: "2025-06-01", InputPerMillionUSD: "2", CachedInputPerMillionUSD: "2", OutputPerMillionUSD: "2"},
	}}
	if err := catalog.Overlay(Rate{Provider: "test", Model: "model", EffectiveFrom: "2025-03-01", EffectiveUntil: "2025-09-01", InputPerMillionUSD: "9", CachedInputPerMillionUSD: "9", OutputPerMillionUSD: "9"}); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		at   string
		want string
	}{
		{"2025-02-01", "1"}, {"2025-04-01", "9"}, {"2025-07-01", "9"}, {"2025-10-01", "2"},
	} {
		at, _ := time.Parse(time.DateOnly, check.at)
		rate, ok := catalog.lookup("test", "model", at)
		if !ok || rate.InputPerMillionUSD != check.want {
			t.Errorf("rate at %s = %+v, %v; want input %s", check.at, rate, ok, check.want)
		}
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEstimateMarksUnknownCacheWriteSurchargePartial(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{
		Provider: "test", Model: "writes", EffectiveFrom: "2020-01-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "0.1", OutputPerMillionUSD: "2", CacheWriteInputScale: "1.25",
	}); err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{{Provider: "test", Model: "writes", TokenUsage: session.TokenUsage{InputTokens: 1_000_000}}}, time.Time{})
	if estimate.Status != "partial" || len(estimate.Limitations) != 1 || estimate.AmountUSD == nil || *estimate.AmountUSD != "1.125" {
		t.Fatalf("unexpected cache-write estimate: %+v", estimate)
	}
	if estimate.LowerBoundUSD == nil || *estimate.LowerBoundUSD != "1" || estimate.UpperBoundUSD == nil || *estimate.UpperBoundUSD != "1.25" || estimate.UncertaintyUSD == nil || *estimate.UncertaintyUSD != "0.125" {
		t.Fatalf("unexpected cache-write bounds: %+v", estimate)
	}
}

func TestEstimateDoesNotDoubleChargeCachedOrReasoningTokens(t *testing.T) {
	catalog := Catalog{}
	err := catalog.Override(Rate{Provider: "test", Model: "model", EffectiveFrom: "2020-01-01", InputPerMillionUSD: "2", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "4"})
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
	if err := catalog.Override(Rate{Provider: "test", Model: "known", EffectiveFrom: "2020-01-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1"}); err != nil {
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

func TestEstimateBoundsUsageWithOnlyTotalTokens(t *testing.T) {
	catalog := Catalog{}
	if err := catalog.Override(Rate{Provider: "test", Model: "known", EffectiveFrom: "2020-01-01", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "0.1", OutputPerMillionUSD: "2"}); err != nil {
		t.Fatal(err)
	}
	estimate := catalog.Estimate([]session.UsageSegment{
		{Provider: "test", Model: "known", TokenUsage: session.TokenUsage{InputTokens: 1_000_000, TotalTokens: 1_000_000}},
		{Provider: "test", Model: "known", TokenUsage: session.TokenUsage{TotalTokens: 5_003}},
	}, time.Time{})
	if estimate.Status != "partial" || estimate.AmountUSD == nil || *estimate.AmountUSD != "1.00525315" || len(estimate.Components) != 2 || len(estimate.Limitations) != 1 {
		t.Fatalf("unexpected bounded total-only estimate: %+v", estimate)
	}
	if estimate.LowerBoundUSD == nil || *estimate.LowerBoundUSD != "1.0005003" || estimate.UpperBoundUSD == nil || *estimate.UpperBoundUSD != "1.010006" || estimate.UncertaintyUSD == nil || *estimate.UncertaintyUSD != "0.00475285" {
		t.Fatalf("unexpected total-only bounds: %+v", estimate)
	}
}
