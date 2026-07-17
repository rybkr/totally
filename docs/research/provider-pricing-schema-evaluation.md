# OpenAI and Anthropic pricing dimensions: schema evaluation

Research date: 2026-07-16
Scope: first-party OpenAI API and first-party Anthropic/Claude API pricing as it
affects attribution from local agent-session transcripts. Partner cloud pricing
is noted only where the provider itself identifies it as a distinct pricing
context. All sources are first-party and were retrieved on the research date.

## Executive conclusion

The proposed schema in
[`internal/pricing/catalogs/SCHEMA.md`](../../internal/pricing/catalogs/SCHEMA.md)
has a sound **meter layer** for the ordinary, standard-tier text requests that
Totally currently needs:

- `input_tokens`, `cached_input_tokens`, and `output_tokens` correctly model
  OpenAI's familiar three-way token pricing.
- Anthropic's `input_tokens`, `cache_read_input_tokens`,
  `cache_creation_input_tokens`, and nested 5-minute/1-hour cache-creation
  details map cleanly to the proposed non-overlapping token meters.
- A single, unqualified `cache_write_tokens` is now necessary for OpenAI GPT-5.6
  and later families, while TTL-qualified write meters are necessary for
  Anthropic. These should remain distinct.
- `web_search_requests` is a valid request-denominated meter for both providers.
- The OpenAI `>272,000` long-context rule is accurately expressible by the
  narrow `threshold_multiplier`, provided **every affected meter** is targeted:
  all input/cache-write categories at 2x and output at 1.5x.

However, the current schema is not yet a faithful representation of the
providers' complete published API price systems. Its main missing concept is a
**pricing context attached to a request**. Multiple price schedules are valid at
the same instant for the same provider/model depending on:

- processing/service tier: standard, batch, flex, or priority at OpenAI; batch
  and fast mode at Anthropic;
- inference geography: global versus regional processing at OpenAI, and global
  versus US-only inference at Anthropic;
- sometimes tool or modality variants.

The current `(provider, model, basis, time)` lookup requires exactly one
schedule, so it cannot choose among these simultaneous prices. Nor can its only
adjustment kind represent stackable batch, geography, and fast-mode modifiers.
That is the important defect to fix before calling V1 broadly representative.

The recommended V1 posture is:

1. Keep the proposed normalized meters and the deliberately narrow long-context
   adjustment.
2. Add explicit, closed-enum schedule selectors for the few observed billing
   axes (at minimum processing tier and inference geography), and define
   non-overlap per selector tuple rather than only per model/basis.
3. Prefer fully materialized absolute rates for each supported selector tuple.
   Do not turn the catalog into a general rules language merely because the
   provider describes some prices as multipliers.
4. Explicitly define a smaller supported product envelope for V1. Token-only
   Claude Code and Codex sessions can be covered well; storage, containers,
   runtime, audio, generated images, and video require new meter units and, in
   some cases, account-level allowance/allocation rules.
5. Replace the statement that an unreported component is zero with a tri-state
   observability rule: known quantity, known zero, or unknown. Treating unknown
   as zero contradicts Totally's structured-uncertainty contract and the later
   telemetry-capabilities section of SCHEMA.md.

### Document versus current implementation

This note evaluates the proposed contract, not only the current loader. The code
does not yet implement the proposed contract:

- [`session.TokenUsage`](../../internal/session/parser.go) defines Codex
  `InputTokens` as inclusive of `CachedInputTokens`; pricing currently subtracts
  cached input. The proposed catalog meters instead define exclusive quantities.
  The boundary between provider/session telemetry and pricing meters therefore
  needs an explicit normalization type or conversion step, not a silent semantic
  change to the existing field.
- [`internal/pricing/catalog.go`](../../internal/pricing/catalog.go) currently
  accepts only `million_tokens` and only four meters: input, cached input,
  output, and generic cache writes. The documented Anthropic TTL meters and
  request-denominated web meters are not loadable yet.
- `UsageSegment` already has `ServiceTier`, which is a useful start, but there is
  no inference-geography/deployment field and catalog lookup does not select a
  schedule by tier.

Those are implementation gaps, not reasons to discard the proposed schema, but
they should prevent declaring the documented V1 implemented prematurely.

## 1. The providers' common core

### 1.1 Ordinary token pricing

Both providers publish model-specific rates per million input and output tokens.
OpenAI additionally publishes cached-input rates for cache-capable models. Its
current flagship price table has separate columns for input, cached input, cache
writes, and output, and separate tabs for standard, batch, flex, and priority
processing ([OpenAI pricing](https://developers.openai.com/api/docs/pricing)).

Anthropic publishes five columns per model: base input, 5-minute cache writes,
1-hour cache writes, cache hits/refreshes, and output. Representative current
rates are:

| Model | Base input | 5m write | 1h write | Cache read | Output |
|---|---:|---:|---:|---:|---:|
| Claude Opus 4.8 | $5 | $6.25 | $10 | $0.50 | $25 |
| Claude Sonnet 5 (introductory, through 2026-08-31) | $2 | $2.50 | $4 | $0.20 | $10 |
| Claude Sonnet 4.6 | $3 | $3.75 | $6 | $0.30 | $15 |
| Claude Haiku 4.5 | $1 | $1.25 | $2 | $0.10 | $5 |

All figures are USD per million tokens. Sonnet 5 changes to $3/$3.75/$6/$0.30/$15
on 2026-09-01, a useful real example of why half-open effective-date schedules
are necessary ([Anthropic pricing](https://platform.claude.com/docs/en/about-claude/pricing)).

**Schema assessment:** representable. Absolute decimal prices, USD,
`million_tokens`, and half-open effective periods are a good fit. Keeping prices
absolute rather than deriving cache prices from the base input rate also
protects the catalog if a provider changes only one component.

### 1.2 Output and reasoning tokens

OpenAI reasoning usage is reported as a detail within completion/output usage,
and current model price cards publish a single output rate. Anthropic likewise
publishes one output rate rather than a separate thinking-token rate. Treating
reported reasoning tokens as a subset of `output_tokens`, rather than an
additional meter, avoids double charging and matches the published price shape.
OpenAI's own model comparison presents reasoning support alongside only one
output-token price ([OpenAI model comparison](https://developers.openai.com/api/docs/models/compare)).

**Schema assessment:** representable, and the documented non-overlap rule is
important.

## 2. Prompt caching

### 2.1 OpenAI cache reads, writes, and retention

OpenAI prompt caching is automatically available for eligible prompts of at
least 1,024 tokens. Reads are reported as `cached_tokens`. For GPT-5.6 and later
families, writes are separately reported as `cache_write_tokens` and billed at
1.25x the uncached input price; before GPT-5.6, cache writes have no extra fee
([OpenAI prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching)).

Retention is not currently a pricing discriminator at OpenAI:

- GPT-5.6-family `prompt_cache_options.ttl` has one supported value, `30m`, and
  sets a minimum lifetime; OpenAI may retain the prefix longer.
- Earlier models' in-memory cache is typically active for 5–10 minutes of
  inactivity and at most one hour.
- Supported earlier models may use extended retention up to 24 hours, but the
  documentation says in-memory and extended-retention pricing is the same.

([OpenAI prompt-cache retention](https://developers.openai.com/api/docs/guides/prompt-caching#prompt-cache-retention))

**Schema assessment:** the generic `cache_write_tokens` meter is exactly right.
It should not be renamed to a TTL-specific meter: the observed price distinction
does not depend on OpenAI retention. Older model schedules may simply omit the
write rate because no separately billed write quantity exists.

One caution: the schema's subtraction formula is correct only when the provider
field treated as `total_input_tokens` includes the other components. Adapters
should follow each response format rather than apply the formula blindly. More
importantly, an **unreported** component is not generally zero. The adapter must
distinguish a present zero from an absent field or a format incapable of
reporting the dimension. Otherwise a billable but unobservable cache write or
tool call is silently fabricated as zero.

### 2.2 Anthropic cache reads, writes, TTLs, and mixed-TTL requests

Anthropic's usage accounting is explicitly non-overlapping:

```text
total_input_tokens =
    cache_read_input_tokens
  + cache_creation_input_tokens
  + input_tokens
```

The nested `cache_creation` object splits the creation total into
`ephemeral_5m_input_tokens` and `ephemeral_1h_input_tokens`; Anthropic states
that their sum equals `cache_creation_input_tokens`. It also allows both TTLs in
one request and describes billing as cache reads through the highest hit, then
1-hour writes, then 5-minute writes
([Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)).

Pricing is 1.25x base input for 5-minute writes, 2x for 1-hour writes, and 0.1x
for reads. A 5-minute entry is refreshed without an extra write charge when it
is read. Anthropic explicitly says prompt-cache multipliers stack with batch
and data-residency modifiers
([Anthropic pricing](https://platform.claude.com/docs/en/about-claude/pricing#prompt-caching)).

**Schema assessment:** the proposed `cache_write_5m_tokens` and
`cache_write_1h_tokens` meters are necessary and sufficient for base caching.
The rule that these may coexist with one another, but not with the generic
write meter, matches telemetry and billing. The problem appears only when batch
or geography is combined with them; that is a pricing-context issue, not a
cache-meter issue.

## 3. Processing and service tiers

### 3.1 OpenAI standard, batch, flex, and priority

OpenAI has four simultaneously published processing prices for supported
models:

- **Standard**: the ordinary synchronous rate.
- **Batch**: asynchronous completion within 24 hours at a 50% discount
  ([OpenAI pricing](https://developers.openai.com/api/docs/pricing),
  [Batch API guide](https://developers.openai.com/api/docs/guides/batch)).
- **Flex**: lower availability/slower processing, explicitly priced at Batch
  rates and selected with `service_tier: "flex"`
  ([Flex processing](https://developers.openai.com/api/docs/guides/flex-processing)).
- **Priority**: premium per-token rates; cache discounts still apply and
  multimodal/image input is eligible
  ([Priority processing](https://developers.openai.com/api/docs/guides/priority-processing)).

The rates cannot safely be inferred as one universal multiplier. Current tables
show Batch and Flex at roughly half standard, but decimal display/rounding is
material (for example, GPT-5.4 cached input is shown as $0.13/MTok in Batch
versus $0.25 standard), while Priority premiums differ across model generations
(for example, GPT-5.4 is 2x standard but GPT-5.5 is 2.5x). The catalog should
record the published absolute rates.

**Schema assessment:** unrepresentable today. Four schedules with overlapping
dates violate the unique schedule rule, while folding the tier into the model
ID would make a request attribute masquerade as model identity. Add a
processing-tier selector.

### 3.2 Anthropic standard, batch, Priority Tier, and fast mode

Anthropic Batch charges all usage at 50% of standard API prices, including a
published table of batch input/output rates; prompt caching can be combined
with batch ([Anthropic batch processing](https://platform.claude.com/docs/en/build-with-claude/batch-processing)).

Anthropic's older contractual Priority Tier is capacity purchased by input and
output tokens per minute for a term and model, not a public pay-as-you-go
per-request price card. The response usage reports whether a request was served
as `priority` or `standard`, but invoice reconciliation requires the customer's
contract and an allocation policy
([Anthropic service tiers](https://platform.claude.com/docs/en/api/service-tiers)).
This should not be confused with OpenAI's public per-token Priority rates.

Anthropic also has a request `speed: "fast"` mode for selected Opus models. As
of the research date, Opus 4.8 fast mode is $10/MTok input and $50/MTok output,
versus $5/$25 standard. Fast-mode base prices combine with cache multipliers and
the US-only inference multiplier, but cannot be combined with Batch
([Anthropic pricing, fast mode](https://platform.claude.com/docs/en/about-claude/pricing#fast-mode-pricing)).

**Schema assessment:**

- Batch and fast mode are unrepresentable without a pricing-context selector or
  more adjustment kinds.
- Contractual Priority Tier is intentionally outside a public
  `api_equivalent` catalog unless Totally accepts user-provided contract cards
  and defines capacity-cost allocation. The response's `service_tier` remains
  useful exported telemetry even when it does not select a public token rate.

## 4. Long-context pricing

### 4.1 OpenAI's 272K threshold

For current 1.05M-context flagship models, OpenAI says prompts over 272,000
input tokens price the **full** request/session at 2x input and 1.5x output for
standard, batch, and flex. The current pricing table also shows cached-input and
cache-write long-context prices at 2x their short-context rates
([OpenAI GPT-5.4 model card](https://developers.openai.com/api/docs/models/gpt-5.4),
[OpenAI pricing](https://developers.openai.com/api/docs/pricing)).

**Schema assessment:** the proposed `threshold_multiplier` is a good, evidence-
driven exception to the no-rules-language preference:

- request scope, `total_input_tokens`, `gt`, and 272000 match the published
  condition;
- multiplying the entire quantity rather than only excess tokens matches the
  published consequence;
- input, cached input, and any cache-write rate must each be targeted at 2x;
- output must be targeted at 1.5x.

The term "full session" in OpenAI prose should be validated against the actual
request/response grain used by Codex transcripts. If one billed API response
contains several internal iterations, the threshold must be applied to the
provider's billed unit, not an arbitrarily reconstructed chat turn.

### 4.2 Current Anthropic long context

Anthropic currently states that Fable 5, Mythos 5/Preview, Opus 4.6–4.8, Sonnet
5, and Sonnet 4.6 include the full 1M context at standard per-token prices; a
900K request has the same rate as a 9K request. Cache and batch discounts apply
across the full context window
([Anthropic long-context pricing](https://platform.claude.com/docs/en/about-claude/pricing#long-context-pricing)).

**Schema assessment:** no adjustment is needed for these current models. The
schema is also capable of Anthropic's older threshold pricing. Anthropic's
immutable 2026-05-27 list-price PDF records, for example, Claude Opus 4.1 at
$15 input/$75 output through 200K versus $30/$300 in its 1M tier. That is a 2x
multiplier for every input/cache category and 4x for output; Claude 3.7 Sonnet,
3.5 Sonnet, and 3.5 Haiku show the same 2x-input/4x-output pattern. Batch halves
both tiers ([Anthropic List Prices, 2026-05-27](https://www-cdn.anthropic.com/files/4zrzovbb/website/3684c2faafb97418665782cea0001f439f74b1d2.pdf)).
The proposed adjustment can encode this with threshold `200000`, `gt`, explicit
2x targets for base/cache reads/cache writes, and a 4x output target. Again, the
batch context still requires a selector. Historical catalog construction should
prefer immutable sources like this PDF; a mutable current page cannot prove an
old effective interval.

## 5. Inference geography and data residency

### 5.1 OpenAI regional processing

OpenAI's current price table applies a 10% uplift to eligible regional-
processing endpoints for models released on or after 2026-03-05. The data
controls guide distinguishes regional storage from regional processing and
lists model/endpoint/region eligibility
([OpenAI pricing](https://developers.openai.com/api/docs/pricing),
[OpenAI data controls](https://platform.openai.com/docs/models/default-usage-policies-by-endpoint)).

**Schema assessment:** unrepresentable today. Regional and global requests for
the same model are valid at the same time, and this multiplier can coexist with
processing tier and long-context pricing. A geography selector should use the
actual billing distinction (for example, `global`/`regional`) rather than a
large enum of storage regions that all have the same rate.

### 5.2 Anthropic US-only inference

For Claude Opus 4.6, Sonnet 4.6, and later models, `inference_geo: "us"` applies
a 1.1x multiplier to **all** token categories: base input, output, cache writes,
and cache reads. `global` is standard. The multiplier combines with cache and
fast-mode prices. Earlier models reject the parameter
([Anthropic data-residency pricing](https://platform.claude.com/docs/en/about-claude/pricing#data-residency-pricing)).

Anthropic also notes that Bedrock and Google Cloud regional/multi-region
endpoints have a 10% premium for Claude 4.5 and later, but those partner-
operated platforms have their own provider pricing pages and invoice identity
([Anthropic cloud-platform pricing](https://platform.claude.com/docs/en/about-claude/pricing#cloud-platform-pricing)).

**Schema assessment:** first-party US-only pricing is unrepresentable today.
Partner platforms should likely be separate provider/deployment identities, not
silently treated as the first-party `anthropic` provider.

## 6. Tools and non-token charges

### 6.1 Request-priced web tools

Anthropic web search costs $10 per 1,000 successful searches plus token costs;
each search iteration counts separately, errors are not billed, and
`usage.server_tool_use.web_search_requests` reports the quantity. Web fetch
reports `web_fetch_requests` but has no additional request charge; only fetched
content tokens are billed
([Anthropic tool pricing](https://platform.claude.com/docs/en/about-claude/pricing#specific-tool-pricing)).

OpenAI's current web search is also $10 per 1,000 calls plus search-content
tokens at model rates. Older/preview variants differ: non-reasoning preview
search is $25 per 1,000 calls with free search-content tokens, and selected mini
models bill a fixed block of 8,000 input tokens per non-preview call
([OpenAI tool pricing](https://developers.openai.com/api/docs/pricing#tools)).

**Schema assessment:**

- `web_search_requests` at `$10/thousand_requests` is representable for the
  current common case.
- Anthropic `web_fetch_requests` may either have an explicit zero rate (useful
  as a catalog assertion that it was considered) or be exported as telemetry
  not expected by the price schedule. The contract should state which zero-
  priced convention is canonical.
- One undifferentiated `web_search_requests` meter cannot express OpenAI's
  differing tool versions/prices unless tool version becomes a selector or the
  adapter emits distinct normalized meters.
- A fixed 8,000 input-token block per call is safe only if provider usage
  telemetry already includes it. The catalog deliberately should not synthesize
  tokens from tool calls unless that rule is explicitly added and evidence
  shows transcripts omit the billed tokens.

SCHEMA.md currently calls both request meters "billable" invocations. That is
incorrect for Anthropic web fetch, which is officially free apart from content
tokens. Rename the group to server-tool invocation meters and let a schedule's
rate (including explicit zero, if chosen) state billability.

There is also a catalog-locality issue. Web-search price is provider/tool-level,
not inherently a model fact. Repeating the same tool rate in every model card
creates drift risk and gives each model schedule one `source_url` even when the
model-token rates and tool rates come from different official pages. Better
options are a provider tool card referenced during estimation, or per-rate
provenance. If model cards continue to contain tool rates, allow multiple source
records and test that provider-wide tool prices remain consistent.

### 6.2 Token-only client tools and visual input

Anthropic client-side tools carry no distinct invocation fee. Tool definitions,
the provider's tool-use system prompt, tool calls/results, bash/text-editor
definitions, and computer-use screenshots all add normal input/output tokens.
Vision uses visual tokens and prices them at the model's input-token rate
([Anthropic tool-use pricing](https://platform.claude.com/docs/en/about-claude/pricing#tool-use-pricing),
[Anthropic vision](https://platform.claude.com/docs/en/build-with-claude/vision)).
PDFs likewise have no extra PDF fee; extracted text and page images become
ordinary input tokens
([Anthropic PDF support](https://platform.claude.com/docs/en/build-with-claude/pdf-support)).

**Schema assessment:** representable when the provider's usage totals already
include the extra tokens. No separate image meter is needed for ordinary Claude
models because visual tokens share the input rate.

### 6.3 Storage-, capacity-, and time-priced tools

Important published charges outside the proposed units include:

- OpenAI file-search calls: $2.50/1,000 calls (request unit: easy to add).
- OpenAI file-search storage: $0.10/GB-day after a 1 GB allowance.
- OpenAI hosted-shell/code-interpreter containers: size-dependent prices and
  time/session semantics, including a five-minute minimum.
- Anthropic code execution: a five-minute minimum, 1,550 free organization
  hours per month, then $0.05/container-hour; it is free when paired with recent
  web-search/fetch tools.
- Anthropic Managed Agents: $0.08 per running session-hour in addition to model
  tokens, with idle/rescheduling time excluded.

([OpenAI tool pricing](https://developers.openai.com/api/docs/pricing#tools),
[Anthropic specific-tool pricing](https://platform.claude.com/docs/en/about-claude/pricing#specific-tool-pricing),
[Anthropic Managed Agents pricing](https://platform.claude.com/docs/en/about-claude/pricing#claude-managed-agents-pricing))

**Schema assessment:** mostly unrepresentable. New units such as GB-day,
container-minute/hour, and session-hour are mechanically possible, but the
monthly free pool, minimum duration, conditional free execution, and allocation
of account-level storage to sessions are not simple per-request meters. They
belong in a later, explicitly scoped extension. V1 should report these as known
unsupported billable dimensions rather than pretend they are zero.

## 7. Multimodal and specialized model pricing

OpenAI currently has modality-specific prices that cannot be collapsed into the
three generic token meters:

- Realtime models have distinct audio, text, and image input/cache rates and
  audio/text output rates.
- Image-generation models have distinct image-token and text-token rates.
- Video generation is priced per generated second and varies by model and
  resolution.
- Some transcription is token-priced but also published with an estimated
  per-minute cost.

([OpenAI multimodal pricing](https://developers.openai.com/api/docs/pricing#multimodal-models))

For ordinary flagship text models, image input is tokenized and billed at the
model's input rate, so the generic input meter remains adequate. The gap is for
specialized/realtime models where equal token counts have different prices by
modality.

**Schema assessment:** outside the safe V1 envelope. Supporting these requires
modality-qualified token meters plus units such as seconds. This does not block
Claude Code/Codex text-session attribution, but the schema documentation should
avoid claiming general OpenAI API coverage.

## 8. Model IDs, aliases, and snapshots

OpenAI model pages distinguish aliases from dated snapshots and say snapshots
lock behavior; for example, the GPT-5.4 page lists `gpt-5.4` and
`gpt-5.4-2026-03-05`
([OpenAI GPT-5.4](https://developers.openai.com/api/docs/models/gpt-5.4)).

Anthropic's rules are subtler:

- pre-4.6 canonical IDs include a date, while shorter names are moving aliases
  to the latest dated snapshot for that minor version;
- 4.6 and later dateless IDs are themselves fixed canonical snapshots, not
  evergreen aliases.

([Anthropic model IDs and versioning](https://platform.claude.com/docs/en/about-claude/models/model-ids-and-versions))

**Schema assessment:** a card can assign rates to any literal transcript model
string, so basic costing works. But the schema has no alias/canonical mapping,
which creates two risks:

1. the same model is fragmented in exports under alias and canonical IDs;
2. a moving alias can change its target without a pricing change, and a model
   card alone cannot document that history.

Add optional alias metadata with effective intervals, or explicitly state that
catalog identity preserves the transcript's literal model ID and never promises
canonicalization. Do not assume that every dateless Anthropic ID is an alias.

## 9. Subscriptions, contracts, discounts, and invoice reconciliation

API-equivalent cost is not necessarily the amount invoiced for a local Codex or
Claude Code session:

- either product can be used under a subscription with shared allowances;
- enterprise customers can have negotiated volume discounts;
- Anthropic contractual Priority Tier is committed capacity;
- Anthropic marketplace billing may convert rated USD usage to Claude
  Consumption Units, while partner cloud providers issue their own bills;
- free credits and account-level allowances do not map naturally to one
  request.

Anthropic explicitly says volume discounts are negotiated case by case and that
Claude Platform on AWS converts standard per-feature USD usage to CCUs after
discounts ([Anthropic pricing](https://platform.claude.com/docs/en/about-claude/pricing)).

**Schema assessment:** the separation between `api_equivalent` model cards and
reserved subscription plan cards is correct. The plan shape is not yet enough
for invoice allocation, and SCHEMA.md correctly says so. Totally should retain
API-equivalent cost even when subscription allocation is unavailable and label
the basis, rather than silently treating API equivalent as invoice cost.

## 10. Concrete V1 fit matrix

| Published pricing case | Current proposal | Assessment |
|---|---|---|
| Standard OpenAI text input/cache read/output | Direct rates | Good |
| OpenAI GPT-5.6+ cache writes | `cache_write_tokens` | Good |
| Anthropic 5m/1h writes and reads | TTL write meters + cached input | Good |
| Anthropic mixed TTLs in one request | Both TTL meters may coexist | Good |
| OpenAI >272K whole-request uplift | Threshold adjustment with explicit targets | Good |
| Current Anthropic 1M context at flat rates | No adjustment | Good |
| Older Anthropic >200K 1M tier | Threshold adjustment with 2x input/4x output targets | Good |
| Current $10/1K web search | Request meter | Good |
| Anthropic free web fetch | Zero rate or telemetry-only convention needed | Ambiguous |
| OpenAI Batch/Flex/Priority | No schedule selector | Not representable |
| Anthropic Batch/Fast | No schedule selector/composition | Not representable |
| OpenAI regional processing | No geography selector | Not representable |
| Anthropic US-only inference | No geography selector | Not representable |
| Versioned web-search variants | One undifferentiated request meter | Not representable |
| File-search calls | No meter yet, but same unit works | Easy extension |
| Storage, containers, code execution, agent runtime | No units/allocation rules | Not representable |
| Realtime modality-specific tokens | Generic token meters | Not representable |
| Generated video seconds/resolution | No seconds/config selector | Not representable |
| Moving model aliases | Literal model cards only | Costable but not normalized |
| Public API-equivalent subscription use | Separate basis | Correctly distinct |

## 11. Recommended schema changes before freezing V1

### Required for faithful OpenAI + Anthropic text API pricing

1. **Add processing-tier selection.** A schedule should optionally declare a
   closed value such as `standard`, `batch`, `flex`, `priority`, or `fast`, with
   provider validation restricting which values make sense. The request cost
   input must carry the observed/resolved tier. `standard` should be explicit in
   serialized data even if omitted in TOML for convenience.

2. **Add inference-geography selection.** A small billing-semantic enum such as
   `global`, `regional`, and `us` is preferable to geographic region names when
   multiple regions share the same uplift. Provider validation should prevent
   nonsensical combinations.

3. **Make schedule uniqueness conditional on selectors.** The invariant should
   become: exactly one schedule matches `(provider, model, basis, instant,
   processing_tier, inference_geography, ...)` within the supported envelope.

4. **Materialize absolute rates.** For example, an Anthropic Sonnet batch+US
   context can carry the final absolute input/read/write/output rates. This
   avoids generalized modifier composition, makes source review easy, and
   preserves the existing "self-contained schedule" virtue. A catalog generator
   may calculate these values, but the checked contract should contain decimals.

5. **Define unknown-selector behavior.** If a transcript omits a billable
   selector, the result is not automatically standard. It should be known only
   when provider/product semantics prove a default; otherwise cost is partial or
   unavailable with a structured limitation.

One concrete, deliberately small shape would be:

```toml
[[schedules]]
effective_from = "2026-05-27"
basis = "api_equivalent"
currency = "USD"
source_url = "https://example.com/immutable-provider-price-list"
source_retrieved_at = "2026-07-16"

[schedules.selector]
platform = "first_party"
processing_mode = "batch"
inference_geography = "us"

[[schedules.rates]]
meter = "input_tokens"
unit = "million_tokens"
price = "1.65"
```

Here `processing_mode` is a normalized billing selector, not a copy of any one
provider field. Its closed values can cover the mutually exclusive public
price paths currently observed (`standard`, `batch`, `flex`, `priority`, and
`fast`), while provider adapters retain raw fields such as Anthropic
`service_tier` separately. This matters because Anthropic's contractual
Priority Tier and OpenAI's pay-as-you-go Priority processing share a word but
not a price model. `platform` also makes the direct-provider scope explicit
without pretending that Bedrock, Vertex AI, or Foundry prices are the same
facts.

If omitted selectors are allowed as catalog shorthand, loading should
materialize their documented defaults before matching. The matching invariant
should operate only on the materialized tuple; wildcard precedence would create
an implicit rules language and make ambiguity harder to reject.

### Recommended clarity changes

6. State that `web_fetch_requests` may be emitted even when no extra fee exists,
   and choose either an explicit-zero-rate or telemetry-only convention.
7. Replace "unreported component is zero" with explicit known/known-zero/unknown
   quantity semantics and carry unknowns into structured cost limitations.
8. Decide whether provider-wide tool prices live in separate tool cards or in
   model schedules; support provenance for every distinct pricing fact rather
   than forcing one schedule URL to source unrelated rates.
9. State the V1 supported envelope (for example, token-priced API-equivalent
   agent messages plus selected per-call web tools), with known exclusions for
   storage/runtime and specialized media.
10. Add alias metadata or explicitly preserve literal transcript IDs.
11. Require immutable/archive sources when asserting historical price periods
   where feasible; a retrieval date on a mutable page proves only what was
   observed then, not the claimed historical boundary.
12. Keep the threshold adjustment narrow. None of the other findings justify a
    general expression language if context-specific absolute schedules are
    available.

## Final judgment

SCHEMA.md's documented idea is **good at the meter level and good for the
immediate standard-tier Claude Code addition**, especially its non-overlap
rules, Anthropic TTL-specific cache writes, OpenAI generic cache writes, explicit
tool-request quantities, and narrow long-context adjustment.

It is **not yet good enough as a general V1 OpenAI/Anthropic pricing contract**
because it assumes time is the only dimension selecting a rate schedule. The
providers now price the same model at the same time differently by processing
tier and inference geography, and Anthropic explicitly composes those choices
with caching and fast mode. Add request pricing context before freezing the
contract, or explicitly narrow V1's promise to standard/global text-agent
requests and surface every other context as unsupported/partial.
