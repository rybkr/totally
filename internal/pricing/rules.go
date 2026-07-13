package pricing

import "fmt"

// PricingContext is the request usage available to pricing rules.
type PricingContext struct {
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64
	TotalTokens       int64
}

// MeterRates contains the nanos-of-a-dollar per million tokens used while a
// price is being calculated. Rules return a modified copy; they must not
// mutate shared catalog state.
type MeterRates struct {
	Input       int64
	CachedInput int64
	Output      int64
	CacheWrite  int64
}

// PricingRule changes the applicable meter rates for one usage segment.
// Rules are applied in declaration order, allowing a catalog to add new rule
// kinds without expanding Rate.
type PricingRule interface {
	Apply(PricingContext, MeterRates) (MeterRates, error)
	Validate() []ValidationIssue
}

// LongContextRule applies alternate multipliers when a request exceeds its
// input-token threshold. Empty scales mean no change to that meter.
type LongContextRule struct {
	Type             string `json:"type"`
	ThresholdTokens  int64  `json:"threshold_tokens"`
	InputScale       string `json:"input_scale,omitempty"`
	CachedInputScale string `json:"cached_input_scale,omitempty"`
	OutputScale      string `json:"output_scale,omitempty"`
	CacheWriteScale  string `json:"cache_write_scale,omitempty"`
}

func (r LongContextRule) Apply(ctx PricingContext, rates MeterRates) (MeterRates, error) {
	inputTokens := ctx.InputTokens
	if inputTokens == 0 {
		inputTokens = ctx.TotalTokens
	}
	if inputTokens <= r.ThresholdTokens {
		return rates, nil
	}
	for _, target := range []struct {
		name  string
		scale string
		rate  *int64
	}{{"input_scale", r.InputScale, &rates.Input}, {"cached_input_scale", r.CachedInputScale, &rates.CachedInput}, {"output_scale", r.OutputScale, &rates.Output}, {"cache_write_scale", r.CacheWriteScale, &rates.CacheWrite}} {
		scale, err := parseScale(target.scale)
		if err != nil {
			return MeterRates{}, fmt.Errorf("%s: %w", target.name, err)
		}
		*target.rate = *target.rate * scale / 1_000
	}
	return rates, nil
}

func applyRules(rate Rate, usage PricingContext, rates MeterRates) (MeterRates, error) {
	for _, rule := range rate.Rules {
		var err error
		rates, err = rule.Apply(usage, rates)
		if err != nil {
			return MeterRates{}, err
		}
	}
	return rates, nil
}

func (r LongContextRule) Validate() []ValidationIssue {
	var issues []ValidationIssue
	if r.Type != "" && r.Type != "long_context" {
		issues = append(issues, ValidationIssue{Field: "rules.type", Message: "must be long_context"})
	}
	if r.ThresholdTokens < 0 {
		issues = append(issues, ValidationIssue{Field: "rules.threshold_tokens", Message: "must be non-negative"})
	}
	for _, value := range []struct{ field, value string }{{"rules.input_scale", r.InputScale}, {"rules.cached_input_scale", r.CachedInputScale}, {"rules.output_scale", r.OutputScale}, {"rules.cache_write_scale", r.CacheWriteScale}} {
		if value.value != "" {
			if _, err := parseScale(value.value); err != nil {
				issues = append(issues, ValidationIssue{Field: value.field, Message: err.Error()})
			}
		}
	}
	return issues
}
