# Totally

`totally`: a terminal utility for analyzing local agent session files.

## Usage

```sh
go run ./cmd/totally --help
go run ./cmd/totally files --help
```

## Commands

```sh
totally [global flags] files [--limit N]
```

`files` discovers local session transcript files and prints them as a table by
default.

## Global Flags

```text
--agent all|codex       Agent session format to discover. Default: all.
--home PATH             Agent home directory. May be repeated.
--archived              Include archived sessions.
--since TIME            Include sessions at or after TIME.
--until TIME            Include sessions at or before TIME.
--format table|json     Output format. Default: table.
```

`--since` and `--until` accept relative durations (`24h`, `7d`, `30d`), dates
(`2026-07-01`), or RFC3339 timestamps (`2026-07-01T12:00:00Z`).

## Examples

```sh
totally files
totally files --limit 10
totally --format json files
totally --agent codex --since 7d files
totally --home ~/.codex --archived files
```
