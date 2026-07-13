package pricing

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type catalogManifest struct {
	SchemaVersion  int      `toml:"schema_version"`
	CatalogVersion string   `toml:"catalog_version"`
	Files          []string `toml:"files"`
}

type modelCard struct {
	SchemaVersion int `toml:"schema_version"`
	Model         struct {
		Provider string `toml:"provider"`
		ID       string `toml:"id"`
	} `toml:"model"`
	Schedules []priceSchedule `toml:"schedules"`
}

type priceSchedule struct {
	EffectiveFrom     string           `toml:"effective_from"`
	EffectiveUntil    string           `toml:"effective_until"`
	Basis             string           `toml:"basis"`
	Currency          string           `toml:"currency"`
	SourceURL         string           `toml:"source_url"`
	SourceRetrievedAt string           `toml:"source_retrieved_at"`
	SourceArchiveURL  string           `toml:"source_archive_url"`
	Rates             []catalogRate    `toml:"rates"`
	Adjustments       []rateAdjustment `toml:"adjustments"`
}

type catalogRate struct {
	Meter string `toml:"meter"`
	Unit  string `toml:"unit"`
	Price string `toml:"price"`
}

type rateAdjustment struct {
	Kind      string             `toml:"kind"`
	Scope     string             `toml:"scope"`
	Measure   string             `toml:"measure"`
	Operator  string             `toml:"operator"`
	Threshold int64              `toml:"threshold"`
	Targets   []adjustmentTarget `toml:"targets"`
}

type adjustmentTarget struct {
	Meter      string `toml:"meter"`
	Multiplier string `toml:"multiplier"`
}

func TestBundledCatalogConformsToSchema(t *testing.T) {
	root := "catalogs"
	manifest := decodeTOML[catalogManifest](t, filepath.Join(root, "catalog.toml"))
	if manifest.SchemaVersion != 1 || manifest.CatalogVersion == "" {
		t.Fatalf("invalid catalog manifest metadata: %+v", manifest)
	}

	actual, err := filepath.Glob(filepath.Join(root, "*", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range actual {
		actual[i], err = filepath.Rel(root, actual[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(actual)
	want := slices.Clone(manifest.Files)
	slices.Sort(want)
	if !slices.Equal(actual, want) {
		t.Fatalf("manifest files = %v, catalog files = %v", want, actual)
	}

	for _, name := range manifest.Files {
		t.Run(name, func(t *testing.T) {
			validateModelCard(t, name, decodeTOML[modelCard](t, filepath.Join(root, name)))
		})
	}
}

func TestValidateBundledCardAcceptsCacheWriteAdjustmentTarget(t *testing.T) {
	card := bundledCard{SchemaVersion: 1}
	card.Model.Provider = "test"
	card.Model.ID = "model"
	card.Schedules = []bundledSchedule{{
		EffectiveFrom: "2026-01-01",
		Rates: []bundledRate{
			{Meter: "input_tokens", Unit: "million_tokens", Price: "1"},
			{Meter: "cached_input_tokens", Unit: "million_tokens", Price: "0.1"},
			{Meter: "output_tokens", Unit: "million_tokens", Price: "2"},
			{Meter: "cache_write_tokens", Unit: "million_tokens", Price: "1.25"},
		},
		Adjustments: []bundledAdjustment{{Kind: "threshold_multiplier", Scope: "request", Measure: "total_input_tokens", Operator: "gt", Targets: []bundledTarget{{Meter: "cache_write_tokens", Multiplier: "2"}}}},
	}}
	if err := validateBundledCard("test/model.toml", card); err != nil {
		t.Fatalf("cache-write adjustment target was rejected: %v", err)
	}
	rule, err := bundledPricingRule(card.Schedules[0].Adjustments[0])
	if err != nil {
		t.Fatal(err)
	}
	longContext, ok := rule.(LongContextRule)
	if !ok || longContext.CacheWriteScale != "2" {
		t.Fatalf("cache-write adjustment was not translated: %+v", rule)
	}
}

func decodeTOML[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func validateModelCard(t *testing.T, name string, card modelCard) {
	t.Helper()
	if card.SchemaVersion != 1 || card.Model.Provider == "" || card.Model.ID == "" || len(card.Schedules) == 0 {
		t.Fatalf("invalid model card metadata: %+v", card)
	}
	if filepath.Base(name) != card.Model.ID+".toml" {
		t.Errorf("filename does not match model id %q", card.Model.ID)
	}

	type interval struct{ from, until time.Time }
	intervals := make([]interval, 0, len(card.Schedules))
	for _, schedule := range card.Schedules {
		from := parseCatalogDate(t, schedule.EffectiveFrom)
		var until time.Time
		if schedule.EffectiveUntil != "" {
			until = parseCatalogDate(t, schedule.EffectiveUntil)
			if !from.Before(until) {
				t.Error("effective_until must be after effective_from")
			}
		}
		intervals = append(intervals, interval{from: from, until: until})
		if schedule.Basis != "api_equivalent" || schedule.Currency != "USD" || schedule.SourceURL == "" || schedule.SourceRetrievedAt == "" {
			t.Errorf("invalid schedule metadata: %+v", schedule)
		}
		parseCatalogDate(t, schedule.SourceRetrievedAt)
		if len(schedule.Rates) == 0 {
			t.Error("schedule must contain at least one rate")
		}

		rates := make(map[string]bool)
		for _, rate := range schedule.Rates {
			if !knownMeter(rate.Meter) || rate.Unit != "million_tokens" {
				t.Errorf("invalid rate meter or unit: %+v", rate)
			}
			if rates[rate.Meter] {
				t.Errorf("duplicate rate for meter %q", rate.Meter)
			}
			rates[rate.Meter] = true
			if _, err := parseUSDNanos(rate.Price); err != nil {
				t.Errorf("invalid rate price %q: %v", rate.Price, err)
			}
		}

		adjusted := make(map[string]bool)
		for _, adjustment := range schedule.Adjustments {
			if adjustment.Kind != "threshold_multiplier" || adjustment.Scope != "request" || adjustment.Measure != "total_input_tokens" || adjustment.Operator != "gt" || adjustment.Threshold < 0 {
				t.Errorf("invalid adjustment: %+v", adjustment)
			}
			for _, target := range adjustment.Targets {
				if !rates[target.Meter] || adjusted[target.Meter] {
					t.Errorf("invalid or repeated adjustment target %q", target.Meter)
				}
				adjusted[target.Meter] = true
				if _, err := parseUSDNanos(target.Multiplier); err != nil {
					t.Errorf("invalid multiplier %q: %v", target.Multiplier, err)
				}
			}
		}
	}
	for i := range intervals {
		for j := i + 1; j < len(intervals); j++ {
			if intervalsOverlap(intervals[i].from, intervals[i].until, intervals[j].from, intervals[j].until) {
				t.Errorf("schedules %d and %d overlap", i, j)
			}
		}
	}
}

func intervalsOverlap(aFrom, aUntil, bFrom, bUntil time.Time) bool {
	aEndsAfterBStarts := aUntil.IsZero() || bFrom.Before(aUntil)
	bEndsAfterAStarts := bUntil.IsZero() || aFrom.Before(bUntil)
	return aEndsAfterBStarts && bEndsAfterAStarts
}

func parseCatalogDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("invalid catalog date %q: %v", value, err)
	}
	return parsed
}

func knownMeter(meter string) bool {
	return meter == "input_tokens" || meter == "cached_input_tokens" || meter == "output_tokens" || meter == "cache_write_tokens"
}
