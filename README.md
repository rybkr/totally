# Totally

`totally` is a local CLI for understanding agent sessions: what you worked on,
how much it used and cost, and where the underlying transcript lives.

> **Status:** This README defines the intended public CLI. The current
> implementation is being brought into line with it.

## Commands

| Command | Purpose |
| --- | --- |
| `totally` | Alias for `totally sessions` |
| `totally sessions` | Find and browse sessions |
| `totally show <session-id>` | Explain one session |
| `totally stats` | Aggregate and compare usage and estimated cost |
| `totally prices` | Show model pricing assumptions and rates |
| `totally files` | Inspect raw transcript discovery and storage |

## Common filters

These filters apply wherever they are meaningful:

```text
--cwd PATH                      Limit to a working directory
--since TIME, --after TIME      Records at or after TIME
--until TIME, --before TIME     Records at or before TIME
--model MODEL                   Limit to a model
--provider PROVIDER             Limit to a provider
--archived                      Include archived sessions
--limit N                       Limit listed rows
--format table|json             Select output format (default: table)
```

`--after` is an alias for `--since`; `--before` is an alias for `--until`.
Time values accept relative durations (`7d`), dates (`2026-07-01`), RFC3339
timestamps, and `today`, `yesterday`, or `now`.

## Data sources

```text
--agent AGENT                   Read a supported agent session format
--home PATH                     Agent data directory; may be repeated
--config PATH                   Configuration file path
```

By default, `totally` discovers supported local agent homes. Use `--home` to
inspect a specific home or combine several homes in one report.

## Find sessions

`sessions` answers “which session was that?” Its table includes the session ID,
working directory, first prompt/task descriptor, start time, model, token use,
estimated cost, and duration.

```sh
totally sessions
totally sessions --cwd . --since 7d
totally sessions --prompt "command set"
totally sessions --model gpt-5 --sort cost --limit 10
totally sessions --full
```

Session-specific options:

```text
--prompt TEXT                   Match the first prompt/task descriptor
--sort started|updated|cost|tokens|duration
--latest                        Select the most recently updated session
--full                          Do not truncate display values in table output
```

## Show a session

`show` answers “what happened, and what did it cost?” An unambiguous session ID
prefix is accepted.

```sh
totally show 019f44e4
totally show --latest
totally show --latest --cwd . --provider openai --model gpt-5
totally show 019f44e4 --full
```

`--cwd`, `--provider`, and `--model` narrow `--latest` to the most recently
updated matching session. They can be combined and require `--latest`.

The report includes session metadata, working directory, first prompt/task descriptor,
transcript location, model/provider use, prompts, turns, messages, tool calls,
duration, token breakdown, and estimated cost.

Table output shows a shortened prompt preview by default. Use `--full` to show
the complete prompt and other untruncated display values. JSON output always
contains the complete prompt.

## Compare usage and cost

`stats` reports session count, prompts, tokens, estimated cost, and duration.
Use `--by` to compare one dimension at a time.

```sh
totally stats --since 7d
totally stats --cwd . --since 30d
totally stats --since 30d --by cwd
totally stats --since 30d --by model
totally stats --cwd . --by day --by model
```

```text
--by cwd|model|provider|day|week|month|session  (may be repeated for composite groups)
--pretty                        Terminal-oriented table output
```

The currently available session selectors are `--cwd`, `--provider`, and
`--model` (along with the global `--since`, `--until`, `--archived`, `--home`,
and `--format` flags). `--by cwd` groups by session working directory. Repeat
`--by` to group by a combination, such as `--by day --by model`.
For `--by model`, tokens and cost are attributed from per-request usage
segments. Session-level measures (sessions, prompts, duration, and activity)
are assigned to the model with the most attributed tokens (with first-seen
breaking ties), so group totals remain additive.

## Pricing

```sh
totally prices
totally prices --model gpt-5
totally prices --provider openai --model gpt-5
totally prices --format json
totally prices verify
totally prices verify --provider openai --model gpt-5
```

Pricing output shows the configured rates per million tokens for input, cached
input, and output, plus the source and effective date/version.
Costs are estimates based on this local price table, not vendor invoice
reconciliation.

`totally prices verify` validates configured pricing overrides and prints
field-level diagnostics for malformed keys, unknown fields, invalid monetary
values, scales, and effective dates. Use `--provider` and/or `--model` to
validate only matching overrides. It does not scan session files.

Built-in prices can be overridden for a date range in the Totally TOML
configuration. Override keys use `provider/model`, and monetary values are
decimal strings. Surrounding bundled history is retained:

```toml
[prices."openai/gpt-5"]
input_per_million_usd = "1.25"
cached_input_per_million_usd = "0.125"
output_per_million_usd = "10.00"
effective_from = "2025-08-07"
source = "user"
```

Set `replace = true` to replace a model's entire bundled pricing history.

Session costs use the API-equivalent basis. Cached input is excluded from
regular input before applying its lower rate, and reasoning tokens are not
charged separately because they are included in output tokens. If usage cannot
be attributed to a priced model, `show` reports the estimate as unavailable or
partial rather than treating it as zero. JSON includes the structured `cost`
object and retains `cost_usd` as a compatibility field.

When a transcript identifies a priced model but reports only `total_tokens`,
Totally bounds the estimate by assigning those tokens to the cheapest and most
expensive billable meters, then uses the midpoint. The JSON cost object records
the resulting lower and upper bounds.

The bundled catalog also records conditional long-context multipliers. They are
applied per request when a model charges more above its input-token threshold.
GPT-5.6 cache writes carry an additional surcharge, but Codex transcripts do
not currently distinguish cache-write tokens. Totally bounds the possible
amount, uses its midpoint as the estimate, and marks it `partial`; terminal
output shows the half-range as `±`, and JSON includes the explicit bounds.

## Raw files

```sh
totally files
totally files --archived
```

`files` is a diagnostic command for transcript discovery, compression, paths,
and storage. Use `sessions` for normal work.

## Automation

Terminal tables are the default. Use JSON for scripts and integrations:

```sh
totally stats --cwd . --since 30d --format json
totally sessions --since 7d --format json
```
