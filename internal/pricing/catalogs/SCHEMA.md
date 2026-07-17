# Pricing catalog schema

The bundled catalog records traceable pricing facts. It does not describe what
a particular transcript format can observe and is not a general-purpose rules
language.

All schema versions are integers. Decimal monetary values and multipliers are
strings so readers can preserve their exact values. Dates are ISO 8601 calendar
dates interpreted at midnight UTC.

## Supported pricing envelope

Schema version 1 covers **token-priced, API-equivalent agent requests** — the
per-request token charges and per-invocation server-tool charges that a local
agent transcript can attribute — across the processing-mode and
inference-geography selectors defined below. It deliberately does not model:

- storage-, capacity-, or time-priced resources (file-search storage,
  code-execution containers, agent session-hours);
- modality-specific rates (audio, generated images, video seconds) where equal
  token counts price differently by modality;
- subscription allowances and account-level credits (reserved as separate plan
  cards; see [Subscription plans](#subscription-plans));
- negotiated volume discounts and contractual committed-capacity tiers.

A billable dimension outside this envelope is reported as an explicit, structured
limitation on the affected cost, never priced as zero. Widening the envelope is a
deliberate, sourced schema change, not an implicit consequence of adding a rate.

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

Each model card owns one provider/model pair and one or more schedules. A card's
path is `<provider>/<model id>.toml`, spelling the provider's model identifier
verbatim (e.g. `openai/gpt-4.1.toml`, `anthropic/claude-opus-4-6.toml`). The
catalog never normalizes provider identifiers: the filename, the `[model] id`
field, and the identifier a transcript reports are the same string.

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
for the same model, basis, and selector tuple (see [Schedule
selectors](#schedule-selectors)) must not overlap. Exactly one schedule may
match a given instant and selector tuple; no match means pricing is unavailable.
File order has no effect.

`basis`, `currency`, `source_url`, and `source_retrieved_at` are required.
`source_retrieved_at` is the date on which the source was checked and uses the
same ISO 8601 calendar-date representation as schedule boundaries. Together
with the catalog release history, it records where and when a pricing assertion
was checked. It does not claim that a mutable provider page preserves its
historical contents. When an immutable or archived copy of the source exists,
a schedule may additionally record it as `source_archive_url`.

Schema version 1 supports the `api_equivalent` basis and the `USD` currency.
Each meter is denominated in exactly one unit: token-denominated meters use
`million_tokens`; request-denominated meters (per-invocation server tools) use
`thousand_requests`. A schedule must contain at least one rate and must not
repeat a meter/unit pair.

Meter quantities are billable, non-overlapping quantities after a provider
adapter has normalized its telemetry. A given token quantity is emitted under
exactly one token meter; an adapter must never report the same tokens under two
meters.

Token-denominated meters (`million_tokens`):

- `input_tokens`: input tokens not served from a cache and not written to one.
- `cached_input_tokens`: input tokens served from a cache (a cache *read*),
  billed at the discounted cache-read rate.
- `output_tokens`: all generated output tokens, including reasoning tokens
  when a provider reports reasoning as a subset of output.
- `cache_write_tokens`: input tokens written to a provider cache and billed at
  a single cache-write rate. Use this for providers that do not price cache
  writes by retention.
- `cache_write_5m_tokens` / `cache_write_1h_tokens`: input tokens written to a
  provider cache at a specific retention (5-minute / 1-hour TTL), billed at
  distinct rates. Use these for providers that price cache writes by retention
  (e.g. Anthropic's ephemeral cache). A schedule prices writes either with the
  single `cache_write_tokens` meter *or* with the TTL-specific meters, never
  both — the quantities do not overlap.

Request-denominated meters (`thousand_requests`) count server-tool invocations,
independent of any tokens the tool's results contribute (those flow through the
token meters above). Whether an invocation is billable is stated by the
schedule's rate, not by the meter itself: some server tools carry a
per-invocation charge, others are free apart from their result tokens.

- `web_search_requests`: web-search invocations. Anthropic and current OpenAI
  web search bill per invocation.
- `web_fetch_requests`: web-fetch invocations. Anthropic web fetch has no
  per-invocation charge — only its fetched-content tokens are billed. A schedule
  may price this meter at an explicit `0` to assert the rate was considered, or
  omit it and treat the count as exported telemetry the schedule does not price.

These token meters are non-overlapping by construction: for a request,
`total_input_tokens = input_tokens + cached_input_tokens + (all cache-write
meter quantities)`. The catalog consumes meters a provider adapter has already
normalized to this exclusive form; it must never re-derive one meter from the
others or infer a subtraction from the mere presence of multiple rates. How an
adapter produces these meters from raw telemetry — including how it represents a
quantity the transcript cannot observe — is a normalization concern outside this
schema. Request-denominated meters are not tokens and never enter this identity.

`price` is an absolute, non-negative decimal amount in the schedule currency.
Rates cannot refer to or derive their price from other rates. This keeps every
schedule self-contained when a provider changes one component independently.

### Schedule selectors

Time is not the only dimension that chooses a schedule. A provider can publish
several prices for the same model at the same instant, chosen by how the request
was processed. A schedule may carry a `[schedules.selector]` table of closed-enum
billing axes:

```toml
[schedules.selector]
platform = "first_party"
processing_mode = "standard"
inference_geography = "global"
```

Each field is a normalized billing distinction, not a copy of any single
provider field:

- `platform`: which price system issued the rate. `first_party` is the direct
  provider API. Partner-operated platforms (e.g. Amazon Bedrock, Google Vertex
  AI, Microsoft Foundry) bill different facts under their own invoice identity
  and are distinct platforms, not the first-party provider.
- `processing_mode`: the mutually exclusive public price path a request took —
  `standard`, `batch`, `flex`, `priority`, or `fast`. This is deliberately
  normalized: OpenAI's pay-as-you-go priority processing and Anthropic's
  contractual Priority Tier share a word but not a price model, so an adapter's
  raw field (e.g. Anthropic `service_tier`) is retained separately and mapped to
  this closed value.
- `inference_geography`: the billing-semantic geography that changes the rate —
  `global`, `regional`, or `us`. This encodes the billing distinction, not a
  list of storage regions that happen to share one rate.

Every field defaults to its standard value (`first_party`, `standard`,
`global`). A schedule that omits the selector table, or any field of it, is
equivalent to one that states the defaults; loading materializes the defaults
before matching, so the checked-in contract need not repeat them.

The uniqueness invariant is conditional on the full selector tuple: exactly one
schedule matches `(provider, model, basis, instant, platform, processing_mode,
inference_geography)`. Two schedules that differ only in a selector value may
therefore share an effective interval; two that share the entire tuple must not
overlap.

Provider validation restricts which selector values are meaningful for a given
provider and basis — `flex` is an OpenAI processing mode, `fast` is an Anthropic
one, and Anthropic's contractual Priority Tier is not an `api_equivalent` public
rate at all. An unsupported combination is a catalog error, not a silently
ignored field.

Rates are materialized absolutely for each selector tuple. The catalog does not
compose modifiers at lookup time: an Anthropic Sonnet batch-plus-US schedule
carries the final input/read/write/output decimals, even though the provider
describes them as the standard rate times batch and residency multipliers. A
catalog generator may compute these values, but the checked-in schedule holds
decimals — this keeps every schedule self-contained and independently
sourceable, and avoids turning the catalog into a modifier-composition language.

Schedule lookup takes the request's *resolved* selector tuple and matches it
against the materialized tuple above; a tuple with no matching schedule is
unpriced, exactly as with time. Resolving that tuple from a transcript — and
deciding what an unreported selector means — is a normalization concern outside
this schema. The catalog neither infers a default for a missing selector nor
observes the request.

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
`input_tokens + cached_input_tokens` plus every cache-write meter quantity
(`cache_write_tokens`, `cache_write_5m_tokens`, `cache_write_1h_tokens`).
Request-denominated meters are not tokens and never contribute. Because
normalized meters do not overlap, no input token is counted twice. `threshold`
is a non-negative integer.

Each target names one rate in the same schedule and supplies a non-negative
decimal multiplier. Only named meters are adjusted: adjustments are not
inherited by related or derived meters. Any cache-write rate
(`cache_write_tokens` or a TTL-specific `cache_write_5m_tokens` /
`cache_write_1h_tokens`) therefore needs its own target when long-context
pricing changes that rate. Multiple matching adjustments for one meter are
invalid in version 1 rather than implicitly composed.

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
schedule and reports missing billable meters as an explicit limitation.
Observability differs by adapter: a Codex transcript may omit `cache_write_tokens`
even though the model schedule correctly prices them, whereas a Claude Code
transcript reports cache writes split by retention (`cache_write_5m_tokens` /
`cache_write_1h_tokens`) and per-invocation `web_search_requests` /
`web_fetch_requests`. A meter priced by a schedule but never emitted by the
adapter is a reported limitation, not a silent zero.
