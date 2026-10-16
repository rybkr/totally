# OpenAI model pricing research — 2026-10-16

Checked against official OpenAI documentation. Scope: newly missing text/reasoning
models, new pricing tiers, and changed rates in the existing GPT-5.6 family.
Amounts below are USD per million tokens. Tuple order is **uncached input,
cached input, cache write, output**; `—` is unpublished/not applicable, never an
assertion of a zero rate. This note is evidence for catalog maintenance, not a
claim that Codex transcripts observe every billable meter.

## New GPT-6 models

| Model | Standard | Batch and Flex (each) | Fast | Ultrafast |
| --- | --- | --- | --- | --- |
| `gpt-6-astra` | 10 / 1 / 12.5 / 50 | 5 / .5 / 6.25 / 25 | 20 / 2 / 25 / 100 | 60 / 6 / 75 / 300 |
| `gpt-6-sol` | 2 / .2 / 2.5 / 10 | 1 / .1 / 1.25 / 5 | 4 / .4 / 5 / 20 | — |
| `gpt-6-luna` | .1 / .01 / .125 / .5 | .05 / .005 / .0625 / .25 | .2 / .02 / .25 / 1 | — |
| `gpt-6.1-sol` | 2 / .1 / 2.5 / 10 | 1 / .05 / 1.25 / 5 | 4 / .2 / 5 / 20 | 12 / .6 / 15 / 60 |

Each row is supported by its model page's explicit rates and model-specific
processing rules: [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra),
[Sol](https://developers.openai.com/api/docs/models/gpt-6-sol),
[Luna](https://developers.openai.com/api/docs/models/gpt-6-luna),
[6.1 Sol](https://developers.openai.com/api/docs/models/gpt-6.1-sol).
The complete [official pricing Markdown](https://developers.openai.com/api/docs/pricing.md)
independently lists these absolute amounts, including GPT-6 Sol, which the HTML
page initially hides behind “All models.” These are not inferred universal tier
multipliers.

For all four models, **strictly more than 272,000 input tokens** changes the
whole request's input, cache-read, and cache-write rates to 2× and output to
1.5×. All four cache meters must be represented appropriately: the single
`cache_write_tokens` meter applies here, not Anthropic TTL meters. The model
pages explicitly include cache rates in the long-context rule. Only literal
undated identifiers appear in these models' snapshot sections; do not invent
date-suffixed GPT-6 IDs. Sources: the four model pages above.

The API changelog establishes Astra's release on **2026-09-03**, Sol and Luna on
**2026-09-22**, and 6.1 Sol on **2026-09-29**. It explicitly establishes Ultrafast
for Astra on **2026-09-29** and 6.1 Sol on **2026-10-08**. The latter announcements
also establish global/US support for Astra Ultrafast and global/US/EU support
for 6.1 Sol Ultrafast. [API changelog](https://developers.openai.com/api/docs/changelog)

The September 29 entry prints all four 6.1 Sol Standard rates. The September 22
entry prints Sol/Luna input, cached-input, and output rates, but refers readers
to pricing for cache writes. Astra's September 3 entry establishes release
without numeric rates. Consequently, backdating currently published Astra
rates or Sol/Luna write rates to launch is an inference, unlike the explicitly
quoted launch prices. [API changelog](https://developers.openai.com/api/docs/changelog)

**Historical evidence gap:** current model pages establish Batch/Flex/Fast and
regional availability, but do not separately date the start of those offerings.
Using the model release date for every current selector is an inference, not a
verified historical fact. A conservative catalog can start newly discovered
selectors on the verification date, 2026-10-16. The Ultrafast dates establish
availability, not independently the historical price; using October 16 for
those price schedules is also the conservative choice.

Regional processing incurs a 10% premium where supported. GPT-6 Sol, Luna, and
6.1 Sol support EU Standard/Fast/Flex/Batch. Astra Fast excludes EU; supported
regional combinations must follow the model availability matrix.
[Sol model](https://developers.openai.com/api/docs/models/gpt-6-sol),
[Luna model](https://developers.openai.com/api/docs/models/gpt-6-luna),
[6.1 Sol model](https://developers.openai.com/api/docs/models/gpt-6.1-sol),
[data controls](https://developers.openai.com/api/docs/guides/your-data).

The data-controls supported-model tables explicitly include the four GPT-6
models, all three GPT-5.6 models, and `gpt-5.4-nano-2026-03-17` for Responses and
Chat Completions. Thus regional eligibility is sourced for those models rather
than inferred from the general premium rule. Eligibility remains constrained
by processing tier and location as described above.
[Data controls supported models](https://developers.openai.com/api/docs/guides/your-data)

## Fast and Priority represent the same OpenAI billing path

OpenAI renamed Priority Processing to Fast on July 30. Both request values
`service_tier: "priority"` and `"fast"` select Fast. GPT-5.6 and earlier return
`priority` in responses even for requests specifying `fast`. Requests downgraded
to Standard report `default` and receive Standard pricing. Use the resolved
response tier, not merely the requested tier. Fast is not uniformly 2× for older
models. [Fast mode guide](https://developers.openai.com/api/docs/guides/fast-mode)

The repository's initial schema restricted `fast` to Anthropic and did not have
`ultrafast`. Supporting these OpenAI facts requires an explicit selector/schema
change, or a documented normalization to the existing Priority billing path.
Adding a similarly named price card alone cannot solve tier selection.

## Existing GPT-5.6 prices changed

| Model | Current Standard | Current Batch/Flex | Current Fast |
| --- | --- | --- | --- |
| `gpt-5.6-sol` | 4 / .4 / 5 / 20 | 2 / .2 / 2.5 / 10 | 8 / .8 / 10 / 40 |
| `gpt-5.6-terra` | 2 / .2 / 2.5 / 12 | 1 / .1 / 1.25 / 6 | 4 / .4 / 5 / 24 |
| `gpt-5.6-luna` | .2 / .02 / .25 / 1.2 | .1 / .01 / .125 / .6 | .4 / .04 / .5 / 2.4 |

The [complete pricing tables](https://developers.openai.com/api/docs/pricing.md)
list every amount above. Standard rates/cache-write rules are also stated on
the [Sol](https://developers.openai.com/api/docs/models/gpt-5.6-sol),
[Terra](https://developers.openai.com/api/docs/models/gpt-5.6-terra), and
[Luna](https://developers.openai.com/api/docs/models/gpt-5.6-luna) pages.
The complete tables show long-context input/read/write ×2 and output ×1.5 for
all these current tiers.

The changelog dates Terra/Luna price reductions to **July 30**, Sol's reduction
to **August 21**, and Fast long-context support to **August 5**. Preserve earlier
schedules rather than overwrite historical prices. Sol's promotion is promised
at least through November 21; that is not an announced expiration date.
[API changelog](https://developers.openai.com/api/docs/changelog)

`gpt-5.6` is an explicitly documented alias for `gpt-5.6-sol`, established in the
July 9 changelog; the literal alias
needs its own price card under the repository's matching contract.
[Sol model](https://developers.openai.com/api/docs/models/gpt-5.6-sol),
[July 9 changelog](https://developers.openai.com/api/docs/changelog)

## Additional missing models

`gpt-5.4-nano` and its explicit snapshot `gpt-5.4-nano-2026-03-17` have Standard
input/read/output rates **.20 / .02 / 1.25**, no published cache-write rate, and
a 10% regional premium. The model page publishes no long-context premium.
[Nano model](https://developers.openai.com/api/docs/models/gpt-5.4-nano)
Batch and Flex are each **.10 / .01 / .625**; Nano is absent from the Fast table.
[Pricing](https://developers.openai.com/api/docs/pricing.md)
The release date is March 17. [Changelog](https://developers.openai.com/api/docs/changelog)
Nano's Tools section explicitly supports web search; the model page's snapshot
section identifies the dated ID above.
[Nano model](https://developers.openai.com/api/docs/models/gpt-5.4-nano)

`gpt-rosalind-research` has Standard input/read/output rates **5 / .50 / 25**;
billing begins **2026-10-05**, and cache-write pricing explicitly does not apply.
`gpt-rosalind-discovery` is separately listed at **5 / .50 / 25**, but its
effective date was not established. Do not assign Research's billing-start date
to Discovery. Neither has sourced nonstandard rates here.
[Pricing](https://developers.openai.com/api/docs/pricing)

`chat-latest` is a rolling Instant alias at **5 / .50 / 30** input/read/output.
Its snapshot section gives no additional literal ID. Current rates are verified;
the fetched model page does not establish their historical effective date.
[Chat Latest](https://developers.openai.com/api/docs/models/chat-latest)
Its Tools section explicitly supports web search. Neither Rosalind model's
web-search support was established by the official sources consulted; do not
infer that capability from its research-oriented name.
[Chat Latest tools](https://developers.openai.com/api/docs/models/chat-latest)

## Deliberately unresolved

`gpt-5.6-cyber` Standard is **12.50 / 1.25 / 15.625 / 75**, but its model page
says requests over 272K receive a long-context premium while the main pricing
table displays dashes for every long-context cell. The model page also describes
a 400K context window. Do not silently resolve this conflict or claim a
complete Cyber card. [Cyber model](https://developers.openai.com/api/docs/models/gpt-5.6-cyber),
[pricing](https://developers.openai.com/api/docs/pricing)

The pricing inventory also contains `gpt-5.5-cyber`, several Pro models, and
older models missing from the repository. Pro rows have no published cached-input
rate; do not fabricate one to satisfy a loader invariant. Realtime/audio/image,
fine-tuned, partner-platform, and subscription billing involve quantities or
contracts outside this text-token update. The current model inventory was
reviewed for this scope; that does not establish exhaustive historical pricing.
[All models](https://developers.openai.com/api/docs/models/all),
[pricing](https://developers.openai.com/api/docs/pricing.md)

For web-search-capable cards, the published invocation rate is $10 per thousand
calls, with search-content tokens billed at model rates. Per-call fees should
not inherit token-processing multipliers.
[Pricing tools table](https://developers.openai.com/api/docs/pricing#tools)

All sources were retrieved 2026-10-16. Current mutable pages are not archival
proof of unchanged rates since a model's release. Effective-date uncertainty and
unsupported selector combinations should remain visible rather than be filled
with guessed history.

## Catalog decisions for this update

These are conservative implementation choices based on the evidence above,
not additional provider claims:

- Complete Standard schedules for Astra, GPT-6 Sol, and GPT-6 Luna start on
  **2026-10-16**. GPT-6.1 Sol starts on **2026-09-29**, whose announcement prints
  all four base token rates. Sol/Luna's earlier three-rate announcement is not
  used to create an interim schedule that silently omits cache-write charges.
- Both Nano IDs start on **2026-10-16**. Rechecking the March 17 launch entry
  found release/capability information, but no numeric launch rates.
- The literal `gpt-5.6` alias mirrors Sol's schedules from **July 9**, when the
  changelog explicitly established that mapping. `chat-latest` and
  `gpt-rosalind-discovery` start on **2026-10-16**. Rosalind Research starts on **2026-10-05**, the
  explicitly published billing start.
- Existing GPT-5.6 Standard schedules retain earlier history and change at the
  documented reduction dates. Their historical Batch/Flex schedules end at
  those changes; historical Priority schedules end at the **July 30** rename.
  Newly verified current mode schedules start on **October 16**. The resulting
  historical gaps deliberately yield unavailable pricing rather than asserting
  unsourced rates for the gap.
- Newly sourced regional schedules start on **October 16**, including GPT-6,
  existing GPT-5.6 models, its newly added alias, and Nano. Chat Latest and
  Rosalind receive no regional schedules because eligibility was not established.
- Both Nano IDs and Chat Latest include the supported web-search meter.
  Rosalind web-search support remains unknown, so its cards omit that meter;
  the omission does not establish that Rosalind can never incur tool charges.
- Current Fast and Ultrafast rates also start on **October 16**; dated launch
  announcements establish availability without independently preserving those
  tiers' historical prices.
