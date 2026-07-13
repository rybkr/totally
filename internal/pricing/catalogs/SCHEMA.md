# Pricing catalog schema

The bundled catalog records traceable pricing facts. It does not describe what
a particular transcript format can observe and is not a general-purpose rules
language.

All schema versions are integers. Decimal monetary values and multipliers are
strings so readers can preserve their exact values. Dates are ISO 8601 calendar
dates interpreted at midnight UTC.

## Catalog manifest

`catalog.toml` identifies one reproducible release of the bundled catalog:

```toml
schema_version = 1
catalog_version = "2026-07-11"
files = ["openai/gpt-5.toml"]
```

Every bundled price card must occur exactly once in `files`. A release changes
whenever a listed card or the manifest changes.

## API-equivalent model cards

Each model card owns one provider/model pair and one or more schedules:

```toml
schema_version = 1

[model]
provider = "openai"
id = "example-model"

[[schedules]]
effective_from = "2026-01-01"
effective_until = "2026-06-01"
basis = "api_equivalent"
currency = "USD"
source_url = "https://example.com/pricing"
source_retrieved_at = "2026-07-11"

[[schedules.rates]]
meter = "input_tokens"
unit = "million_tokens"
price = "1.25"
```

`effective_from` is required and inclusive. `effective_until` is optional and
exclusive, making the interval `[effective_from, effective_until)`. Schedules
for the same model and basis must not overlap. Exactly one schedule may match a
given instant; no match means pricing is unavailable. File order has no effect.

`basis`, `currency`, `source_url`, and `source_retrieved_at` are required.
`source_retrieved_at` is the date on which the source was checked and uses the
same ISO 8601 calendar-date representation as schedule boundaries. Together
with the catalog release history, it records where and when a pricing assertion
was checked. It does not claim that a mutable provider page preserves its
historical contents. When an immutable or archived copy of the source exists,
a schedule may additionally record it as `source_archive_url`.

Schema version 1 supports the `api_equivalent` basis, the `USD` currency, and
the `million_tokens` unit. A schedule must contain at least one rate and must
not repeat a meter/unit pair.

Meter quantities are billable, non-overlapping quantities after a provider
adapter has normalized its telemetry:

- `input_tokens`: input tokens not served from a cache.
- `cached_input_tokens`: input tokens served from a cache.
- `output_tokens`: all generated output tokens, including reasoning tokens
  when a provider reports reasoning as a subset of output.
- `cache_write_tokens`: input tokens written to a provider cache and billed at
  the cache-write rate. These are separate from uncached input tokens only when
  the provider bills them separately; an adapter must not emit the same token
  quantity under both meters.

If provider telemetry reports total input including cached input and cache
writes, normalization is `input_tokens = max(total_input_tokens -
cached_input_tokens - cache_write_tokens, 0)`; an unreported component is zero.
The catalog consumes normalized meters and must never infer this subtraction
from the mere presence of multiple rates.

`price` is an absolute, non-negative decimal amount in the schedule currency.
Rates cannot refer to or derive their price from other rates. This keeps every
schedule self-contained when a provider changes one component independently.

### Adjustments

Adjustments are deliberately constrained. Schema version 1 supports only
`threshold_multiplier`:

```toml
[[schedules.adjustments]]
kind = "threshold_multiplier"
scope = "request"
measure = "total_input_tokens"
operator = "gt"
threshold = 272000

[[schedules.adjustments.targets]]
meter = "input_tokens"
multiplier = "2"
```

For version 1, `scope` must be `request`, `measure` must be
`total_input_tokens`, and `operator` must be `gt`. `total_input_tokens` includes
all normalized input quantities for that request that contribute to context:
`input_tokens + cached_input_tokens + cache_write_tokens`. Because normalized
meters do not overlap, no input token is counted twice. `threshold` is a
non-negative integer.

Each target names one rate in the same schedule and supplies a non-negative
decimal multiplier. Only named meters are adjusted: adjustments are not
inherited by related or derived meters. A `cache_write_tokens` rate therefore
needs its own target when long-context pricing changes that rate. Multiple
matching adjustments for one meter are invalid in version 1 rather than
implicitly composed.

When the predicate matches, each multiplier applies to the target meter's
entire billable quantity for that request, not only to the quantity above the
threshold. For example, targeting `input_tokens` does not change the
`cached_input_tokens` rate. This rule is evaluated independently for every
request before session costs are aggregated.

New adjustment kinds should be added only for observed provider pricing, with
their scope, operands, composition, and supported targets defined here.

## Subscription plans

Subscription pricing is represented by separate plan cards rather than token
rate schedules. The initial plan shape is reserved as follows; no bundled plan
is asserted until its allowances and allocation behavior can be sourced:

```toml
schema_version = 1

[plan]
provider = "example"
id = "pro"
name = "Pro"

[[schedules]]
effective_from = "2026-01-01"
currency = "USD"
billing_period = "month"
fixed_price = "20.00"

[[schedules.allowances]]
resource = "agent_usage"
quantity = "included"
```

Plan schedules use the same half-open interval and non-overlap rules. Before
plan cards are loaded, the schema must additionally define allowance units,
finite quantities, credits, overages, and how shared subscription cost is
allocated to sessions. API-equivalent cost and allocated subscription cost are
distinct estimates and must not be silently combined.

## Telemetry capabilities

Whether a meter is observable belongs to the transcript/provider adapter, not
to a price card. An estimator compares its observed meters with the applicable
schedule and reports missing billable meters as an explicit limitation. For
example, a Codex transcript may omit `cache_write_tokens` even though the model
schedule correctly prices them.
