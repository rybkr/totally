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
	CatalogVersion = "2026-07-10"
	nanosPerUSD    = int64(1_000_000_000)
	tokensPerRate  = int64(1_000_000)
)

type Rate struct {
	Provider                 string `json:"provider" mapstructure:"provider"`
	Model                    string `json:"model" mapstructure:"model"`
	InputPerMillionUSD       string `json:"input_per_million_usd" mapstructure:"input_per_million_usd"`
	CachedInputPerMillionUSD string `json:"cached_input_per_million_usd" mapstructure:"cached_input_per_million_usd"`
	OutputPerMillionUSD      string `json:"output_per_million_usd" mapstructure:"output_per_million_usd"`
	Source                   string `json:"source" mapstructure:"source"`
	EffectiveFrom            string `json:"effective_from" mapstructure:"effective_from"`
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
}

type Catalog struct{ rates []Rate }

func DefaultCatalog() Catalog {
	return Catalog{rates: []Rate{
		{Provider: "openai", Model: "gpt-5", InputPerMillionUSD: "1.25", CachedInputPerMillionUSD: "0.125", OutputPerMillionUSD: "10.00", Source: "https://developers.openai.com/api/docs/models/gpt-5", EffectiveFrom: "2025-08-07"},
		{Provider: "openai", Model: "gpt-5-mini", InputPerMillionUSD: "0.25", CachedInputPerMillionUSD: "0.025", OutputPerMillionUSD: "2.00", Source: "https://developers.openai.com/api/docs/models/gpt-5-mini", EffectiveFrom: "2025-08-07"},
	}}
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
	for i := range c.rates {
		if c.rates[i].Provider == rate.Provider && c.rates[i].Model == rate.Model {
			c.rates[i] = rate
			return nil
		}
	}
	c.rates = append(c.rates, rate)
	return nil
}

func (c Catalog) Estimate(segments []session.UsageSegment, at time.Time) Estimate {
	result := Estimate{Currency: "USD", Status: "unavailable", Basis: "api_equivalent", PricingVersion: CatalogVersion}
	var total int64
	for _, segment := range segments {
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
	}
	if len(result.Components) > 0 {
		amount := formatNanos(total)
		result.AmountUSD = &amount
		result.Status = "complete"
		if len(result.Missing) > 0 {
			result.Status = "partial"
		}
	}
	return result
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
	numerator := uncached*parsed.input + usage.CachedInputTokens*parsed.cached + usage.OutputTokens*parsed.output
	return (numerator + tokensPerRate/2) / tokensPerRate, nil
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
