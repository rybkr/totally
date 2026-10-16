# Anthropic model pricing refresh — 2026-10-16

Scope: first-party Claude API token/request pricing missing from the bundled
Anthropic catalog. Sources checked 2026-10-16. These findings compare the current
official model index, individual model specifications, launch announcements,
pricing documentation, and dated release notes; they do not infer invoice totals
or account-specific discounts.

## New cards and standard rates

All amounts below are USD per million tokens. The five rate columns map to
`input_tokens`, `cached_input_tokens`, `cache_write_5m_tokens`,
`cache_write_1h_tokens`, and `output_tokens`, respectively. Dates are release
dates, suitable for the schema's day-granularity schedule boundaries.

| Canonical Claude API ID | Effective from | Input | Cache read | 5m write | 1h write | Output | First-party specifications |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| `claude-opus-5` | 2026-07-24 | 5 | 0.50 | 6.25 | 10 | 25 | [Opus 5](https://platform.claude.com/docs/en/models/opus-5/overview) |
| `claude-fable-5-1` | 2026-09-01 | 10 | 0.25 | 12.50 | 20 | 50 | [Fable 5.1](https://platform.claude.com/docs/en/models/fable-5-1/overview) |
| `claude-mythos-5-1` | 2026-09-01 | 10 | 0.25 | 12.50 | 20 | 50 | [Mythos 5.1](https://platform.claude.com/docs/en/models/mythos-5-1/overview) |
| `claude-opus-5-5` | 2026-09-22 | 4 | 0.20 | 5 | 8 | 20 | [Opus 5.5](https://platform.claude.com/docs/en/models/opus-5-5/overview) |
| `claude-sonnet-5-5` | 2026-09-28 | 2 | 0.20 | 2.50 | 4 | 10 | [Sonnet 5.5 specifications](https://platform.claude.com/docs/en/models/sonnet-5-5/overview), [historical cache price](https://www.anthropic.com/claude-haiku-5-5#further-updates) |
| `claude-sonnet-5-5` | 2026-10-07 | 2 | 0.10 | 2.50 | 4 | 10 | [Sonnet 5.5](https://platform.claude.com/docs/en/models/sonnet-5-5/overview) |
| `claude-haiku-5-5` (prompt ≤100,000 tokens) | 2026-10-07 | 0.10 | 0.01 | 0.125 | 0.20 | 0.50 | [Haiku 5.5](https://platform.claude.com/docs/en/models/haiku-5-5/overview) |
| `claude-haiku-5-5` (prompt >100,000 tokens) | 2026-10-07 | 0.50 | 0.05 | 0.625 | 1 | 2.50 | [Haiku 5.5](https://platform.claude.com/docs/en/models/haiku-5-5/overview) |

The six new IDs are missing from the catalog inspected for this research.
Mythos 5.1 has restricted access but a published price schedule; do not confuse
limited availability with unpriced usage. It shares Fable 5.1's pricing.
[Mythos 5.1 specifications](https://platform.claude.com/docs/en/models/mythos-5-1/overview)

These new IDs do not need invented date-suffixed variants. Since the 4.6
generation, the dateless ID is the pinned snapshot itself. Earlier dateless IDs
can instead be convenience aliases, such as `claude-sonnet-4-5` for a dated
snapshot. [Model IDs and versioning](https://platform.claude.com/docs/en/about-claude/models/model-ids-and-versions)

## Dated facts that a current price table alone misses

- Sonnet 5.5 launched September 28, then its read price fell from $0.20 to $0.10
  on October 7. Close the first schedule at `2026-10-07` and start the replacement
  there. The current launch article has been updated to show $0.10; do not use
  that to backdate the reduction. The dated [October 7 release note](https://platform.claude.com/docs/en/release-notes/overview#october-7-2026)
  and [Haiku announcement](https://www.anthropic.com/claude-haiku-5-5#further-updates)
  establish the actual change.
- Existing `claude-sonnet-5.toml` schedules incorrectly retain the once-planned
  September 1 increase to $3/$15. On August 10 Anthropic canceled that increase,
  leaving $2/$10 permanent. Remove the unrealized higher-price period and extend
  the original standard and batch rates. [August 10 release note](https://platform.claude.com/docs/en/release-notes/overview#august-10-2026)
- Opus 5.5 fast mode was already offered September 22; October 15 ended the
  preview/access restriction, not the earlier rate's validity. [September 22 and
  October 15 release notes](https://platform.claude.com/docs/en/release-notes/overview)
- Existing Opus 4.7 fast mode ended July 24; its checked-in schedule already has
  that exclusive end date. [July 24 release note](https://platform.claude.com/docs/en/release-notes/overview#july-24-2026)

## Batch, fast mode, geography, and context

Batch is supported for all these models. The token discount is 50%, and caching
discounts stack. Consequently multiply each of the five standard token rates by
0.5 when materializing a batch schedule. Fast mode cannot be combined with
batch. [Batch processing](https://platform.claude.com/docs/en/build-with-claude/batch-processing)

US-only inference applies 1.1× to all five token categories on Claude 4.6 and
later, including Haiku 5.5's high-context rates. Caching multipliers stack with
batch and residency. Other than Haiku 5.5, these new models have no long-context
premium. Haiku's threshold counts all input, including cache reads and writes;
it changes the whole request's prices, not just excess tokens. [Pricing:
caching, residency, long context](https://platform.claude.com/docs/en/about-claude/pricing)

Thus materialized token rates for new cards use standard ×1, batch ×0.5,
US-standard ×1.1, and US-batch ×0.55. These are arithmetic consequences of the
published modifiers, not independent price announcements. For Haiku 5.5, attach
a request-level `total_input_tokens > 100000` adjustment, with multiplier `5`
on all five token meters, to all four schedules. The existing schema can express
that rule without modification.

Fast mode is available for Opus 5 at $10/$50 input/output and Opus 5.5 at
$8/$40. Caching and residency modifiers apply to fast prices. This materializes
to [10, 1, 12.5, 20, 50] for Opus 5 and [8, 0.4, 10, 16, 40] for Opus 5.5 in the
table's column order; multiply those arrays by 1.1 for US-fast. The other new
models are absent from the supported fast-mode list, so do not create fast
schedules for them. [Fast mode](https://platform.claude.com/docs/en/build-with-claude/fast-mode)

Opus 5's July 24 announcement explicitly offers fast mode at twice base pricing.
[Opus 5 announcement](https://www.anthropic.com/news/claude-opus-5)
Opus 5.5's launch announcement explicitly supplies $0.20 reads and $8/$40 fast
pricing, confirming that these are launch-era prices rather than later changes.
[Opus 5.5 announcement](https://www.anthropic.com/claude-opus-5-5)

For geography, the February 5 release note introduced US-only inference at 1.1×
for models released after February 1, 2026. This published policy predates every
new model here and supports applying US schedules from their respective launches.
[February 5 release note](https://platform.claude.com/docs/en/release-notes/overview#february-5-2026)
The data-residency guide explicitly permits `inference_geo` in batch requests.
[Data residency](https://platform.claude.com/docs/en/manage-claude/data-residency)

Batch history has weaker evidence: all six current specifications list it, and
the release-specific What's New pages explicitly document batch prices/support
for [Fable/Mythos 5.1](https://platform.claude.com/docs/en/models/fable-5-1/whats-new-fable-5-1),
[Opus 5.5](https://platform.claude.com/docs/en/models/opus-5-5/whats-new-opus-5-5), and
[Sonnet 5.5](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5).
These mutable pages support the rates but do not independently timestamp initial
batch availability. No dated batch-enablement changes were found in release notes.
For this refresh, new batch and US-batch schedules start `2026-10-16`, leaving
earlier batch requests unpriced until historical evidence is located. Standard,
US-standard, and the supported Opus fast/US-fast schedules start at launch. The
existing Sonnet 5 batch schedule keeps its already-established launch boundary
when extending the price that became permanent.

## Tool meters and exclusions

Web search costs $10 per 1,000 searches, including batch requests: the dedicated
tool page explicitly says batch searches use regular Messages pricing. Do not
halve the request-meter price when halving batch token rates.
[Web search: batch requests and pricing](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool)

Both tool guides explicitly support dynamic filtering on Claude 4.6 and later,
which includes all six new models. Opus 5.5 is also the concrete model used in
their current API examples. Their generic compatibility links point to the tool
reference, which points back to individual tool pages rather than supplying an
additional model-by-model matrix. This evidence supports search/fetch prices for
all six; it does not establish each model's first day of tool availability.
[Web search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool),
[web fetch](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-fetch-tool)

Web fetch has no separate invocation charge; content tokens remain billable.
This supports an explicit zero `web_fetch_requests` rate. Code execution's
time-based charges and Managed Agents session runtime remain outside schema v1's
token/request envelope, rather than being inferred as zero. [Pricing:
server tools and Managed Agents](https://platform.claude.com/docs/en/about-claude/pricing)

Use the published token modifiers only on token meters; neither the geography
multiplier nor Haiku's context multiplier is described as changing search fees.
Keep request-meter quantities separate from token counts.

## Coverage limits and review checks

This refresh should cover the six new direct-API IDs and correct Sonnet 5's
canceled price increase. The catalog also lacks old dated snapshots and some
retired model families; those are historical coverage work, not additional
models newly launched since this catalog's previous July retrieval.

The sources provide release/change days, not authoritative UTC effective
timestamps. The proposed boundaries follow the existing schema's midnight-UTC
calendar-date convention; they do not establish exact intraday billing cutovers.
Historical rates are supported by explicit dated changes where found. Current
documentation is mutable and should not be described as an immutable archive.

Suggested focused checks: the Sonnet 5 canceled September boundary; both sides of
the Sonnet 5.5 October 7 read-price boundary; Haiku prompts of 100,000 and 100,001
tokens (including cached/write tokens); output affected by Haiku's threshold;
batch-US composition; Opus fast-US composition; unchanged batch search price;
and no pre-release schedule matches. These validate meaningful billing behavior
rather than only checking that cards parse.
