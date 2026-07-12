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
--project NAME|PATH             Limit to a project
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
project, first prompt/task descriptor, start time, model, estimated cost, and
token use.

```sh
totally sessions
totally sessions --project totally --since 7d
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

The report includes session metadata, project, first prompt/task descriptor,
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
totally stats --project totally --since 30d
totally stats --since 30d --by project
totally stats --since 30d --by model
totally stats --project totally --by day --pretty
```

```text
--by project|model|provider|day|week|month|session
--pretty                        Terminal-oriented presentation and, for time
                                groupings, an ASCII chart
```

## Pricing

```sh
totally prices
totally prices --model gpt-5
totally prices --format json
```

Pricing output shows the configured rates per million tokens for input, cached
input, and output, plus the source and effective date/version.
Costs are estimates based on this local price table, not vendor invoice
reconciliation.

Built-in prices can be replaced in the Totally TOML configuration. Override
keys use `provider/model`, and monetary values are decimal strings:

```toml
[prices."openai/gpt-5"]
input_per_million_usd = "1.25"
cached_input_per_million_usd = "0.125"
output_per_million_usd = "10.00"
effective_from = "2025-08-07"
source = "user"
```

Session costs use the API-equivalent basis. Cached input is excluded from
regular input before applying its lower rate, and reasoning tokens are not
charged separately because they are included in output tokens. If usage cannot
be attributed to a priced model, `show` reports the estimate as unavailable or
partial rather than treating it as zero. JSON includes the structured `cost`
object and retains `cost_usd` as a compatibility field.

The bundled catalog also records conditional long-context multipliers. They are
applied per request when a model charges more above its input-token threshold.
GPT-5.6 cache writes carry an additional surcharge, but Codex transcripts do
not currently distinguish cache-write tokens; estimates for those models are
therefore marked `partial` and include that limitation in JSON.

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
totally stats --project totally --since 30d --format json
totally sessions --since 7d --format json
```
