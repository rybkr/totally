package pricing

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/pelletier/go-toml/v2"
)

//go:embed catalogs/catalog.toml catalogs/*/*.toml
var catalogFiles embed.FS

type bundledManifest struct {
	SchemaVersion  int      `toml:"schema_version"`
	CatalogVersion string   `toml:"catalog_version"`
	Files          []string `toml:"files"`
}

type bundledCard struct {
	SchemaVersion int `toml:"schema_version"`
	Model         struct {
		Provider string `toml:"provider"`
		ID       string `toml:"id"`
	} `toml:"model"`
	Schedules []bundledSchedule `toml:"schedules"`
}

type bundledSchedule struct {
	EffectiveFrom  string              `toml:"effective_from"`
	EffectiveUntil string              `toml:"effective_until"`
	Basis          string              `toml:"basis"`
	Currency       string              `toml:"currency"`
	SourceURL      string              `toml:"source_url"`
	Rates          []bundledRate       `toml:"rates"`
	Adjustments    []bundledAdjustment `toml:"adjustments"`
	// Provenance is retained in the TOML cards; Rate exposes the source URL.
	SourceRetrievedAt string `toml:"source_retrieved_at"`
	SourceArchiveURL  string `toml:"source_archive_url"`
}

type bundledRate struct {
	Meter string `toml:"meter"`
	Unit  string `toml:"unit"`
	Price string `toml:"price"`
}

type bundledAdjustment struct {
	Kind      string          `toml:"kind"`
	Scope     string          `toml:"scope"`
	Measure   string          `toml:"measure"`
	Operator  string          `toml:"operator"`
	Threshold int64           `toml:"threshold"`
	Targets   []bundledTarget `toml:"targets"`
}

type bundledTarget struct {
	Meter      string `toml:"meter"`
	Multiplier string `toml:"multiplier"`
}

var loadedBundledCatalog = mustLoadBundledCatalog()

func mustLoadBundledCatalog() struct {
	version string
	rates   []Rate
} {
	manifest := decodeBundledTOML[bundledManifest]("catalogs/catalog.toml")
	if manifest.SchemaVersion != 1 || manifest.CatalogVersion == "" {
		panic("pricing: invalid embedded catalog manifest")
	}
	if err := validateBundledManifest(manifest); err != nil {
		panic("pricing: " + err.Error())
	}
	result := struct {
		version string
		rates   []Rate
	}{version: manifest.CatalogVersion}
	for _, name := range manifest.Files {
		card := decodeBundledTOML[bundledCard](path.Join("catalogs", name))
		if card.SchemaVersion != 1 || card.Model.Provider == "" || card.Model.ID == "" {
			panic(fmt.Sprintf("pricing: invalid embedded model card %q", name))
		}
		if err := validateBundledCard(name, card); err != nil {
			panic("pricing: " + err.Error())
		}
		for _, schedule := range card.Schedules {
			if schedule.Basis != "api_equivalent" || schedule.Currency != "USD" || schedule.SourceURL == "" || schedule.SourceRetrievedAt == "" {
				panic(fmt.Sprintf("pricing: invalid embedded schedule in %q", name))
			}
			rate := Rate{Provider: card.Model.Provider, Model: card.Model.ID, Source: schedule.SourceURL, EffectiveFrom: schedule.EffectiveFrom, EffectiveUntil: schedule.EffectiveUntil}
			for _, item := range schedule.Rates {
				if item.Unit != "million_tokens" {
					panic(fmt.Sprintf("pricing: unsupported rate unit %q in %q", item.Unit, name))
				}
				switch item.Meter {
				case "input_tokens":
					rate.InputPerMillionUSD = item.Price
				case "cached_input_tokens":
					rate.CachedInputPerMillionUSD = item.Price
				case "output_tokens":
					rate.OutputPerMillionUSD = item.Price
				case "cache_write_tokens":
					rate.CacheWritePerMillionUSD = item.Price
				default:
					panic(fmt.Sprintf("pricing: unsupported meter %q in %q", item.Meter, name))
				}
			}
			for _, adjustment := range schedule.Adjustments {
				rule, err := bundledPricingRule(adjustment)
				if err != nil {
					panic(fmt.Sprintf("pricing: unsupported adjustment in %q: %v", name, err))
				}
				rate.Rules = append(rate.Rules, rule)
			}
			if _, err := parseRate(rate); err != nil {
				panic(fmt.Sprintf("pricing: invalid embedded model card %q: %v", name, err))
			}
			result.rates = append(result.rates, rate)
		}
	}
	return result
}

func bundledPricingRule(adjustment bundledAdjustment) (PricingRule, error) {
	if adjustment.Kind != "threshold_multiplier" || adjustment.Scope != "request" || adjustment.Measure != "total_input_tokens" || adjustment.Operator != "gt" {
		return nil, fmt.Errorf("unknown rule kind %q", adjustment.Kind)
	}
	rule := LongContextRule{Type: "long_context", ThresholdTokens: adjustment.Threshold}
	for _, target := range adjustment.Targets {
		switch target.Meter {
		case "input_tokens":
			rule.InputScale = target.Multiplier
		case "cached_input_tokens":
			rule.CachedInputScale = target.Multiplier
		case "output_tokens":
			rule.OutputScale = target.Multiplier
		default:
			return nil, fmt.Errorf("unknown target meter %q", target.Meter)
		}
	}
	return rule, nil
}

func validateBundledManifest(manifest bundledManifest) error {
	seen := make(map[string]bool, len(manifest.Files))
	for _, name := range manifest.Files {
		if name == "" || seen[name] {
			return fmt.Errorf("invalid or duplicate manifest file %q", name)
		}
		seen[name] = true
	}
	entries, err := catalogFiles.ReadDir("catalogs")
	if err != nil {
		return err
	}
	var actual []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		children, err := catalogFiles.ReadDir(path.Join("catalogs", entry.Name()))
		if err != nil {
			return err
		}
		for _, child := range children {
			if !child.IsDir() && path.Ext(child.Name()) == ".toml" {
				actual = append(actual, path.Join(entry.Name(), child.Name()))
			}
		}
	}
	sort.Strings(actual)
	declared := append([]string(nil), manifest.Files...)
	sort.Strings(declared)
	if len(actual) != len(declared) {
		return fmt.Errorf("manifest does not list every model card")
	}
	for i := range actual {
		if actual[i] != declared[i] {
			return fmt.Errorf("manifest membership mismatch: %q is not %q", declared[i], actual[i])
		}
	}
	return nil
}

func validateBundledCard(name string, card bundledCard) error {
	if path.Base(name) != card.Model.ID+".toml" {
		return fmt.Errorf("model card filename %q does not match model %q", name, card.Model.ID)
	}
	if len(card.Schedules) == 0 {
		return fmt.Errorf("model card %q has no schedules", name)
	}
	for i, schedule := range card.Schedules {
		from, err := time.Parse(time.DateOnly, schedule.EffectiveFrom)
		if err != nil {
			return fmt.Errorf("invalid effective_from in %q: %w", name, err)
		}
		if schedule.EffectiveUntil != "" {
			until, err := time.Parse(time.DateOnly, schedule.EffectiveUntil)
			if err != nil || !from.Before(until) {
				return fmt.Errorf("invalid effective_until in %q", name)
			}
		}
		meters := make(map[string]bool)
		for _, rate := range schedule.Rates {
			if !knownBundledMeter(rate.Meter) || rate.Unit != "million_tokens" {
				return fmt.Errorf("invalid rate meter or unit in %q", name)
			}
			if meters[rate.Meter] {
				return fmt.Errorf("duplicate meter %q in %q", rate.Meter, name)
			}
			meters[rate.Meter] = true
			if _, err := parseUSDNanos(rate.Price); err != nil {
				return fmt.Errorf("invalid price for %s in %q: %w", rate.Meter, name, err)
			}
		}
		adjusted := make(map[string]bool)
		for _, adjustment := range schedule.Adjustments {
			if adjustment.Kind != "threshold_multiplier" || adjustment.Scope != "request" || adjustment.Measure != "total_input_tokens" || adjustment.Operator != "gt" || adjustment.Threshold < 0 {
				return fmt.Errorf("invalid adjustment in %q", name)
			}
			for _, target := range adjustment.Targets {
				if !longContextTargetMeter(target.Meter) || !meters[target.Meter] || adjusted[target.Meter] {
					return fmt.Errorf("invalid adjustment target %q in %q", target.Meter, name)
				}
				adjusted[target.Meter] = true
				if _, err := parseUSDNanos(target.Multiplier); err != nil {
					return fmt.Errorf("invalid adjustment multiplier in %q: %w", name, err)
				}
			}
		}
		matches := 0
		for _, other := range card.Schedules {
			start, _ := time.Parse(time.DateOnly, other.EffectiveFrom)
			if !start.After(from) && (other.EffectiveUntil == "" || from.Before(mustCatalogDate(other.EffectiveUntil))) {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("schedule %d in %q does not have exactly one match at its effective date", i, name)
		}
	}
	return nil
}

func mustCatalogDate(value string) time.Time {
	parsed, _ := time.Parse(time.DateOnly, value)
	return parsed
}

func knownBundledMeter(meter string) bool {
	return meter == "input_tokens" || meter == "cached_input_tokens" || meter == "output_tokens" || meter == "cache_write_tokens"
}

func longContextTargetMeter(meter string) bool {
	return meter == "input_tokens" || meter == "cached_input_tokens" || meter == "output_tokens"
}

func decodeBundledTOML[T any](name string) T {
	data, err := catalogFiles.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("pricing: read embedded catalog %q: %v", name, err))
	}
	var value T
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		panic(fmt.Sprintf("pricing: decode embedded catalog %q: %v", name, err))
	}
	return value
}
