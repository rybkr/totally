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
	LowerBoundUSD  *string       `json:"lower_bound,omitempty"`
	UpperBoundUSD  *string       `json:"upper_bound,omitempty"`
	UncertaintyUSD *string       `json:"uncertainty,omitempty"`
	Status         string        `json:"status"`
	Basis          string        `json:"basis"`
	PricingVersion string        `json:"pricing_version"`
	Components     []Component   `json:"components,omitempty"`
	Missing        []MissingRate `json:"missing,omitempty"`
	Limitations    []string      `json:"limitations,omitempty"`
}

type Catalog struct{ rates []Rate }

// ValidationIssue identifies one invalid field in a configured rate.
type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

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

// Validate checks that every effective rate can be used to estimate a cost.
// It is intentionally exported so callers that assemble a catalog from user
// configuration can diagnose it without attempting an estimate first.
func (c Catalog) Validate() error {
	for _, rate := range c.rates {
		if issues := ValidateRate(rate); len(issues) > 0 {
			return fmt.Errorf("invalid %s/%s %s: %s", rate.Provider, rate.Model, issues[0].Field, issues[0].Message)
		}
	}
	for i, rate := range c.rates {
		from, _ := time.Parse(time.DateOnly, rate.EffectiveFrom)
		if from.IsZero() {
			continue
		}
		matches := 0
		for _, candidate := range c.rates {
			if candidate.Provider != rate.Provider || candidate.Model != rate.Model {
				continue
			}
			candidateFrom, _ := time.Parse(time.DateOnly, candidate.EffectiveFrom)
			if candidateFrom.IsZero() || candidateFrom.After(from) {
				continue
			}
			if candidate.EffectiveUntil == "" {
				matches++
				continue
			}
			until, _ := time.Parse(time.DateOnly, candidate.EffectiveUntil)
			if from.Before(until) {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("expected exactly one matching schedule for %s/%s at %s (rate %d)", rate.Provider, rate.Model, rate.EffectiveFrom, i)
		}
	}
	return nil
}

// ValidateRate returns every invalid field in a configured rate.
func ValidateRate(rate Rate) []ValidationIssue {
	var issues []ValidationIssue
	add := func(field string, err error) {
		if err != nil {
			issues = append(issues, ValidationIssue{Field: field, Message: err.Error()})
		}
	}
	if rate.Provider == "" {
		issues = append(issues, ValidationIssue{Field: "provider", Message: "is required"})
	}
	if rate.Model == "" {
		issues = append(issues, ValidationIssue{Field: "model", Message: "is required"})
	}
	var from, until time.Time
	if rate.EffectiveFrom == "" {
		issues = append(issues, ValidationIssue{Field: "effective_from", Message: "is required"})
	} else {
		parsed, err := time.Parse(time.DateOnly, rate.EffectiveFrom)
		add("effective_from", err)
		from = parsed
	}
	if rate.EffectiveUntil != "" {
		parsed, err := time.Parse(time.DateOnly, rate.EffectiveUntil)
		add("effective_until", err)
		until = parsed
	}
	if !from.IsZero() && !until.IsZero() && !from.Before(until) {
		issues = append(issues, ValidationIssue{Field: "effective_until", Message: "must be after effective_from"})
	}
	for _, value := range []struct{ field, value string }{{"input_per_million_usd", rate.InputPerMillionUSD}, {"cached_input_per_million_usd", rate.CachedInputPerMillionUSD}, {"output_per_million_usd", rate.OutputPerMillionUSD}, {"cache_write_per_million_usd", rate.CacheWritePerMillionUSD}} {
		if value.value != "" {
			add(value.field, func() error { _, err := parseUSDNanos(value.value); return err }())
		} else if value.field != "cache_write_per_million_usd" {
			issues = append(issues, ValidationIssue{Field: value.field, Message: "is required"})
		}
	}
	for _, value := range []struct{ field, value string }{{"long_context_input_scale", rate.LongContextInputScale}, {"long_context_cached_input_scale", rate.LongContextCachedInputScale}, {"long_context_output_scale", rate.LongContextOutputScale}, {"cache_write_input_scale", rate.CacheWriteInputScale}} {
		if value.value != "" {
			add(value.field, func() error { _, err := parseScale(value.value); return err }())
		}
	}
	return issues
}

func (c *Catalog) Override(rate Rate) error {
	if rate.Provider == "" || rate.Model == "" {
		return fmt.Errorf("price override requires provider and model")
	}
	if issues := ValidateRate(rate); len(issues) > 0 {
		return fmt.Errorf("invalid %s/%s %s: %s", rate.Provider, rate.Model, issues[0].Field, issues[0].Message)
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
	var lowerTotal, upperTotal int64
	bounded := false
	unbounded := false
	for _, segment := range segments {
		rate, ok := c.lookup(segment.Provider, segment.Model, at)
		if !ok {
			result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
			unbounded = true
			continue
		}
		if hasUnpriceableTokenBreakdown(segment.TokenUsage) {
			lower, upper, err := tokenBreakdownBoundsNanos(segment.TokenUsage, rate)
			if err != nil {
				result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
				unbounded = true
				continue
			}
			lowerTotal += lower
			upperTotal += upper
			componentAmount := (lower + upper) / 2
			result.Components = append(result.Components, Component{Provider: segment.Provider, Model: segment.Model, TokenUsage: segment.TokenUsage, AmountUSD: formatNanos(componentAmount)})
			bounded = bounded || lower != upper
			addUniqueString(&result.Limitations, "some usage segments report total tokens without a billable token breakdown; estimate uses the midpoint of the possible meter allocation")
			continue
		}
		amount, err := costNanos(segment.TokenUsage, rate)
		if err != nil {
			result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
			unbounded = true
			continue
		}
		lowerTotal += amount
		upperTotal += amount
		componentAmount := amount
		if rate.CacheWriteInputScale != "" || rate.CacheWritePerMillionUSD != "" {
			additional, err := cacheWriteUncertaintyNanos(segment.TokenUsage, rate)
			if err != nil {
				result.Missing = append(result.Missing, MissingRate{Provider: segment.Provider, Model: segment.Model})
				unbounded = true
				continue
			}
			if additional >= 0 {
				upperTotal += additional
				componentAmount = amount + additional/2
			} else {
				lowerTotal += additional
				componentAmount = amount + additional/2
			}
			bounded = true
			addUniqueString(&result.Limitations, "cache-write tokens are not identified in the session transcript; estimate uses the midpoint of the possible surcharge")
		}
		result.Components = append(result.Components, Component{Provider: segment.Provider, Model: segment.Model, TokenUsage: segment.TokenUsage, AmountUSD: formatNanos(componentAmount)})
	}
	if len(result.Components) > 0 {
		amount := formatNanos((lowerTotal + upperTotal) / 2)
		result.AmountUSD = &amount
		if bounded && !unbounded {
			lower := formatNanos(lowerTotal)
			upper := formatNanos(upperTotal)
			uncertainty := formatNanos((upperTotal - lowerTotal) / 2)
			result.LowerBoundUSD = &lower
			result.UpperBoundUSD = &upper
			result.UncertaintyUSD = &uncertainty
		}
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

// tokenBreakdownBoundsNanos prices total-only telemetry by assigning every
// token to the cheapest, then the most expensive, billable meter. This is a
// bounded allocation assumption, not an observed input/output breakdown.
func tokenBreakdownBoundsNanos(usage session.TokenUsage, rate Rate) (int64, int64, error) {
	parsed, err := parseRate(rate)
	if err != nil {
		return 0, 0, err
	}
	rates := []int64{parsed.input, parsed.cached, parsed.output}
	cacheWriteRate, err := cacheWriteRateNanos(parsed.input, rate)
	if err != nil {
		return 0, 0, err
	}
	if cacheWriteRate != 0 {
		rates = append(rates, cacheWriteRate)
	}
	if rate.LongContextThreshold > 0 && usage.TotalTokens > rate.LongContextThreshold {
		inputScale, err := parseScale(rate.LongContextInputScale)
		if err != nil {
			return 0, 0, err
		}
		cachedScale, err := parseScale(rate.LongContextCachedInputScale)
		if err != nil {
			return 0, 0, err
		}
		outputScale, err := parseScale(rate.LongContextOutputScale)
		if err != nil {
			return 0, 0, err
		}
		rates = append(rates, parsed.input*inputScale/1_000, parsed.cached*cachedScale/1_000, parsed.output*outputScale/1_000)
		if cacheWriteRate != 0 && rate.CacheWriteInputScale != "" {
			writeScale, err := parseScale(rate.CacheWriteInputScale)
			if err != nil {
				return 0, 0, err
			}
			rates = append(rates, parsed.input*inputScale/1_000*writeScale/1_000)
		}
	}
	minimum, maximum := rates[0], rates[0]
	for _, candidate := range rates[1:] {
		if candidate < minimum {
			minimum = candidate
		}
		if candidate > maximum {
			maximum = candidate
		}
	}
	return usage.TotalTokens * minimum / tokensPerRate, usage.TotalTokens * maximum / tokensPerRate, nil
}

func cacheWriteRateNanos(inputRate int64, rate Rate) (int64, error) {
	if rate.CacheWritePerMillionUSD != "" {
		return parseUSDNanos(rate.CacheWritePerMillionUSD)
	}
	if rate.CacheWriteInputScale == "" {
		return 0, nil
	}
	scale, err := parseScale(rate.CacheWriteInputScale)
	if err != nil {
		return 0, err
	}
	return inputRate * scale / 1_000, nil
}

// cacheWriteUncertaintyNanos returns the greatest possible change from the
// base estimate when any portion of uncached input could instead be cache
// writes. Codex telemetry does not identify that portion.
func cacheWriteUncertaintyNanos(usage session.TokenUsage, rate Rate) (int64, error) {
	parsed, err := parseRate(rate)
	if err != nil {
		return 0, err
	}
	uncached := usage.InputTokens - usage.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	inputScale := int64(1_000)
	if rate.LongContextThreshold > 0 && usage.InputTokens > rate.LongContextThreshold {
		inputScale, err = parseScale(rate.LongContextInputScale)
		if err != nil {
			return 0, err
		}
	}
	inputRate := parsed.input * inputScale / 1_000
	cacheWriteRate := int64(0)
	if rate.CacheWritePerMillionUSD != "" {
		cacheWriteRate, err = parseUSDNanos(rate.CacheWritePerMillionUSD)
		if err != nil {
			return 0, err
		}
	} else {
		writeScale, err := parseScale(rate.CacheWriteInputScale)
		if err != nil {
			return 0, err
		}
		cacheWriteRate = inputRate * writeScale / 1_000
	}
	return uncached * (cacheWriteRate - inputRate) / tokensPerRate, nil
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
