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
	Provider                 string        `json:"provider" mapstructure:"provider"`
	Model                    string        `json:"model" mapstructure:"model"`
	InputPerMillionUSD       string        `json:"input_per_million_usd" mapstructure:"input_per_million_usd"`
	CachedInputPerMillionUSD string        `json:"cached_input_per_million_usd" mapstructure:"cached_input_per_million_usd"`
	OutputPerMillionUSD      string        `json:"output_per_million_usd" mapstructure:"output_per_million_usd"`
	Source                   string        `json:"source" mapstructure:"source"`
	EffectiveFrom            string        `json:"effective_from" mapstructure:"effective_from"`
	EffectiveUntil           string        `json:"effective_until,omitempty" mapstructure:"effective_until"`
	CacheWriteInputScale     string        `json:"cache_write_input_scale,omitempty" mapstructure:"cache_write_input_scale"`
	CacheWritePerMillionUSD  string        `json:"cache_write_per_million_usd,omitempty" mapstructure:"cache_write_per_million_usd"`
	Rules                    []PricingRule `json:"rules,omitempty" mapstructure:"-"`
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

type Catalog struct {
	rates       []Rate
	earlyAccess bool
}

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

// SetEarlyAccess allows the earliest known rate for a model to price sessions
// that predate its first catalog schedule. It is intended for organizations
// that had access to a model before its public release.
func (c *Catalog) SetEarlyAccess(enabled bool) {
	c.earlyAccess = enabled
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
	for _, value := range []struct{ field, value string }{{"cache_write_input_scale", rate.CacheWriteInputScale}} {
		if value.value != "" {
			add(value.field, func() error { _, err := parseScale(value.value); return err }())
		}
	}
	for _, rule := range rate.Rules {
		if rule == nil {
			issues = append(issues, ValidationIssue{Field: "rules", Message: "must not contain null rules"})
			continue
		}
		issues = append(issues, rule.Validate()...)
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

// Overlay replaces pricing only for rate's effective interval, preserving the
// surrounding history for the same provider and model.
func (c *Catalog) Overlay(rate Rate) error {
	if rate.Provider == "" || rate.Model == "" {
		return fmt.Errorf("price override requires provider and model")
	}
	if issues := ValidateRate(rate); len(issues) > 0 {
		return fmt.Errorf("invalid %s/%s %s: %s", rate.Provider, rate.Model, issues[0].Field, issues[0].Message)
	}
	if rate.Source == "" {
		rate.Source = "user"
	}
	from, _ := time.Parse(time.DateOnly, rate.EffectiveFrom)
	var until time.Time
	if rate.EffectiveUntil != "" {
		until, _ = time.Parse(time.DateOnly, rate.EffectiveUntil)
	}
	kept := make([]Rate, 0, len(c.rates)+2)
	for _, existing := range c.rates {
		if existing.Provider != rate.Provider || existing.Model != rate.Model {
			kept = append(kept, existing)
			continue
		}
		existingFrom, _ := time.Parse(time.DateOnly, existing.EffectiveFrom)
		var existingUntil time.Time
		if existing.EffectiveUntil != "" {
			existingUntil, _ = time.Parse(time.DateOnly, existing.EffectiveUntil)
		}
		if !rateIntervalsOverlap(existingFrom, existingUntil, from, until) {
			kept = append(kept, existing)
			continue
		}
		if existingFrom.Before(from) {
			left := existing
			left.EffectiveUntil = rate.EffectiveFrom
			kept = append(kept, left)
		}
		if !until.IsZero() && (existingUntil.IsZero() || until.Before(existingUntil)) {
			right := existing
			right.EffectiveFrom = rate.EffectiveUntil
			kept = append(kept, right)
		}
	}
	c.rates = append(kept, rate)
	return nil
}

func rateIntervalsOverlap(firstFrom, firstUntil, secondFrom, secondUntil time.Time) bool {
	return (firstUntil.IsZero() || secondFrom.Before(firstUntil)) &&
		(secondUntil.IsZero() || firstFrom.Before(secondUntil))
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
	parsed, err := meterRatesFor(rate, usage)
	if err != nil {
		return 0, 0, err
	}
	rates := []int64{parsed.Input, parsed.CachedInput, parsed.Output}
	cacheWriteRate, err := cacheWriteRateNanos(parsed.Input, rate)
	if err != nil {
		return 0, 0, err
	}
	if cacheWriteRate != 0 {
		rates = append(rates, cacheWriteRate)
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
	parsed, err := meterRatesFor(rate, usage)
	if err != nil {
		return 0, err
	}
	uncached := usage.InputTokens - usage.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	inputRate := parsed.Input
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
	var earliest Rate
	var earliestFrom time.Time
	foundEarliest := false
	for _, rate := range c.rates {
		if rate.Provider != provider || rate.Model != model {
			continue
		}
		if c.earlyAccess && !at.IsZero() {
			from, err := time.Parse(time.DateOnly, rate.EffectiveFrom)
			if err == nil && (!foundEarliest || from.Before(earliestFrom)) {
				earliest = rate
				earliestFrom = from
				foundEarliest = true
			}
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
	if c.earlyAccess && foundEarliest && at.Before(earliestFrom) {
		return earliest, true
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

func meterRatesFor(rate Rate, usage session.TokenUsage) (MeterRates, error) {
	parsed, err := parseRate(rate)
	if err != nil {
		return MeterRates{}, err
	}
	return applyRules(rate, PricingContext{
		InputTokens:       usage.InputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		OutputTokens:      usage.OutputTokens,
		TotalTokens:       usage.TotalTokens,
	}, MeterRates{Input: parsed.input, CachedInput: parsed.cached, Output: parsed.output})
}

func costNanos(usage session.TokenUsage, rate Rate) (int64, error) {
	parsed, err := meterRatesFor(rate, usage)
	if err != nil {
		return 0, err
	}
	uncached := usage.InputTokens - usage.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	inputCost := uncached * parsed.Input
	inputCost += usage.CachedInputTokens * parsed.CachedInput
	outputCost := usage.OutputTokens * parsed.Output
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
