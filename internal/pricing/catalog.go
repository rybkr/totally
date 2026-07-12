package pricing

import (
	"bytes"
	"embed"
	"fmt"
	"path"

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
	result := struct {
		version string
		rates   []Rate
	}{version: manifest.CatalogVersion}
	for _, name := range manifest.Files {
		card := decodeBundledTOML[bundledCard](path.Join("catalogs", name))
		if card.SchemaVersion != 1 || card.Model.Provider == "" || card.Model.ID == "" {
			panic(fmt.Sprintf("pricing: invalid embedded model card %q", name))
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
				if adjustment.Kind != "threshold_multiplier" || adjustment.Scope != "request" || adjustment.Measure != "total_input_tokens" || adjustment.Operator != "gt" {
					panic(fmt.Sprintf("pricing: unsupported adjustment in %q", name))
				}
				rate.LongContextThreshold = adjustment.Threshold
				for _, target := range adjustment.Targets {
					switch target.Meter {
					case "input_tokens":
						rate.LongContextInputScale = target.Multiplier
					case "cached_input_tokens":
						rate.LongContextCachedInputScale = target.Multiplier
					case "output_tokens":
						rate.LongContextOutputScale = target.Multiplier
					}
				}
			}
			if _, err := parseRate(rate); err != nil {
				panic(fmt.Sprintf("pricing: invalid embedded model card %q: %v", name, err))
			}
			result.rates = append(result.rates, rate)
		}
	}
	return result
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
