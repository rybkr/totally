// Package pricing estimates API-equivalent session cost from model token usage.
package pricing

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/session"
)

const (
	nanosPerUSD   = int64(1_000_000_000)
	tokensPerRate = int64(1_000_000)
)

var CatalogVersion = loadedBundledCatalog.version

type Rate struct {
	Provider                    string `json:"provider" mapstructure:"provider"`
	Model                       string `json:"model" mapstructure:"model"`
	InputPerMillionUSD          string `json:"input_per_million_usd" mapstructure:"input_per_million_usd"`
	CachedInputPerMillionUSD    string `json:"cached_input_per_million_usd" mapstructure:"cached_input_per_million_usd"`
	OutputPerMillionUSD         string `json:"output_per_million_usd" mapstructure:"output_per_million_usd"`
	Source                      string `json:"source" mapstructure:"source"`
	EffectiveFrom               string `json:"effective_from" mapstructure:"effective_from"`
	EffectiveUntil              string `json:"effective_until,omitempty" mapstructure:"effective_until"`
	LongContextThreshold        int64  `json:"long_context_threshold,omitempty" mapstructure:"long_context_threshold"`
	LongContextInputScale       string `json:"long_context_input_scale,omitempty" mapstructure:"long_context_input_scale"`
	LongContextCachedInputScale string `json:"long_context_cached_input_scale,omitempty" mapstructure:"long_context_cached_input_scale"`
	LongContextOutputScale      string `json:"long_context_output_scale,omitempty" mapstructure:"long_context_output_scale"`
	CacheWriteInputScale        string `json:"cache_write_input_scale,omitempty" mapstructure:"cache_write_input_scale"`
	CacheWritePerMillionUSD     string `json:"cache_write_per_million_usd,omitempty" mapstructure:"cache_write_per_million_usd"`
}

type MissingRate struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type Component struct {
	Provider   string             `json:"provider"`
	Model      string             `json:"model"`
	TokenUsage session.TokenUsage `json:"token_usage"`
	AmountUSD  string             `json:"amount_usd"`
}

type Estimate struct {
	Currency       string        `json:"currency"`
	AmountUSD      *string       `json:"amount"`
	Status         string        `json:"status"`
	Basis          string        `json:"basis"`
	PricingVersion string        `json:"pricing_version"`
	Components     []Component   `json:"components,omitempty"`
	Missing        []MissingRate `json:"missing,omitempty"`
	Limitations    []string      `json:"limitations,omitempty"`
}

type Catalog struct{ rates []Rate }

func DefaultCatalog() Catalog {
	return Catalog{rates: append([]Rate(nil), loadedBundledCatalog.rates...)}
}

func (c Catalog) Rates() []Rate {
	rates := append([]Rate(nil), c.rates...)
	sort.Slice(rates, func(i, j int) bool {
		return rates[i].Provider+"/"+rates[i].Model < rates[j].Provider+"/"+rates[j].Model
	})
	return rates
}

func (c *Catalog) Override(rate Rate) error {
	if rate.Provider == "" || rate.Model == "" {
		return fmt.Errorf("price override requires provider and model")
	}
	if _, err := parseRate(rate); err != nil {
		return err
	}
	if rate.Source == "" {
		rate.Source = "user"
	}
	kept := c.rates[:0]
	for _, existing := range c.rates {
		if existing.Provider != rate.Provider || existing.Model != rate.Model {
			kept = append(kept, existing)
		}
	}
	c.rates = append(kept, rate)
	return nil
}

func (c Catalog) Estimate(segments []session.UsageSegment, at time.Time) Estimate {
	result := Estimate{Currency: "USD", Status: "unavailable", Basis: "api_equivalent", PricingVersion: CatalogVersion}
	var total int64
	for _, segment := range segments {
		if hasUnpriceableTokenBreakdown(segment.TokenUsage) {
			addUniqueString(&result.Limitations, "some usage segments report total tokens without a billable token breakdown; their cost is excluded")
			continue
		}
		rate, ok := c.lookup(segment.Provider, segment.Model, at)
		if !ok {
			result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
			continue
		}
		amount, err := costNanos(segment.TokenUsage, rate)
		if err != nil {
			result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
			continue
		}
		total += amount
		result.Components = append(result.Components, Component{Provider: segment.Provider, Model: segment.Model, TokenUsage: segment.TokenUsage, AmountUSD: formatNanos(amount)})
		if rate.CacheWriteInputScale != "" || rate.CacheWritePerMillionUSD != "" {
			addUniqueString(&result.Limitations, "cache-write tokens are not identified in the session transcript; any cache-write surcharge is excluded")
		}
	}
	if len(result.Components) > 0 {
		amount := formatNanos(total)
		result.AmountUSD = &amount
		result.Status = "complete"
		if len(result.Missing) > 0 {
			result.Status = "partial"
		}
		if len(result.Limitations) > 0 {
			result.Status = "partial"
		}
	} else if len(result.Limitations) > 0 {
		result.Status = "partial"
	}
	return result
}

// hasUnpriceableTokenBreakdown reports a usage record that proves tokens were
// consumed but cannot be priced because the transcript omitted every billable
// counter. total_tokens alone is not enough to assign an input/output price.
func hasUnpriceableTokenBreakdown(usage session.TokenUsage) bool {
	return usage.TotalTokens != 0 &&
		usage.InputTokens == 0 &&
		usage.CachedInputTokens == 0 &&
		usage.OutputTokens == 0 &&
		usage.ReasoningOutputTokens == 0
}

func addUniqueString(values *[]string, value string) {
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

func (c Catalog) lookup(provider, model string, at time.Time) (Rate, bool) {
	for _, rate := range c.rates {
		if rate.Provider != provider || rate.Model != model {
			continue
		}
		if rate.EffectiveFrom != "" && !at.IsZero() {
			effective, err := time.Parse(time.DateOnly, rate.EffectiveFrom)
			if err != nil || at.Before(effective) {
				continue
			}
		}
		if rate.EffectiveUntil != "" && !at.IsZero() {
			until, err := time.Parse(time.DateOnly, rate.EffectiveUntil)
			if err != nil || !at.Before(until) {
				continue
			}
		}
		return rate, true
	}
	return Rate{}, false
}

type parsedRate struct{ input, cached, output int64 }

func parseRate(rate Rate) (parsedRate, error) {
	values := []*int64{}
	parsed := parsedRate{}
	values = append(values, &parsed.input, &parsed.cached, &parsed.output)
	texts := []string{rate.InputPerMillionUSD, rate.CachedInputPerMillionUSD, rate.OutputPerMillionUSD}
	for i, text := range texts {
		value, err := parseUSDNanos(text)
		if err != nil {
			return parsedRate{}, fmt.Errorf("invalid price for %s/%s: %w", rate.Provider, rate.Model, err)
		}
		*values[i] = value
	}
	return parsed, nil
}

func costNanos(usage session.TokenUsage, rate Rate) (int64, error) {
	parsed, err := parseRate(rate)
	if err != nil {
		return 0, err
	}
	uncached := usage.InputTokens - usage.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	inputScale, cachedScale, outputScale := int64(1_000), int64(1_000), int64(1_000)
	if rate.LongContextThreshold > 0 && usage.InputTokens > rate.LongContextThreshold {
		var err error
		inputScale, err = parseScale(rate.LongContextInputScale)
		if err != nil {
			return 0, err
		}
		cachedScale, err = parseScale(rate.LongContextCachedInputScale)
		if err != nil {
			return 0, err
		}
		outputScale, err = parseScale(rate.LongContextOutputScale)
		if err != nil {
			return 0, err
		}
	}
	inputCost := uncached * parsed.input * inputScale / 1_000
	inputCost += usage.CachedInputTokens * parsed.cached * cachedScale / 1_000
	outputCost := usage.OutputTokens * parsed.output * outputScale / 1_000
	numerator := inputCost + outputCost
	return (numerator + tokensPerRate/2) / tokensPerRate, nil
}

func parseScale(text string) (int64, error) {
	if text == "" {
		return 1_000, nil
	}
	nanos, err := parseUSDNanos(text)
	if err != nil {
		return 0, fmt.Errorf("invalid pricing scale %q: %w", text, err)
	}
	return nanos / 1_000_000, nil
}

func parseUSDNanos(text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "-") {
		return 0, fmt.Errorf("expected a non-negative USD amount")
	}
	parts := strings.Split(text, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("expected a non-negative USD amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("expected a non-negative USD amount")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 9 {
		return 0, fmt.Errorf("expected at most 9 decimal places")
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	fractionNanos := int64(0)
	if fraction != "" {
		fractionNanos, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("expected a non-negative USD amount")
		}
	}
	return whole*nanosPerUSD + fractionNanos, nil
}

func formatNanos(value int64) string {
	whole, fraction := value/nanosPerUSD, value%nanosPerUSD
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%d.%09d", whole, fraction), "0"), ".")
}

func FloatAmount(estimate Estimate) float64 {
	if estimate.AmountUSD == nil {
		return 0
	}
	value, _ := strconv.ParseFloat(*estimate.AmountUSD, 64)
	return value
}
