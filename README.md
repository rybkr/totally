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
--config PATH           Config file path.
--agent all|codex       Agent session format to discover. Default: all.
--home PATH             Agent home directory. May be repeated.
--archived              Include archived sessions.
--since TIME            Include sessions at or after TIME.
--until TIME            Include sessions at or before TIME.
--format table|json     Output format. Default: table.
```

`--since` and `--until` accept:

- Relative durations: `24h`, `7d`, `2w`, `1y`
- Dates: `2026-07-01`
- RFC3339 timestamps: `2026-07-01T12:00:00Z`

For relative years, `1y` means 365 days.

## Configuration

Settings are resolved in this order:

```text
CLI flags > environment variables > config file > built-in defaults
```

By default, `totally` reads `~/.config/totally/config.toml` when it exists. Use
`--config PATH` or `TOTALLY_CONFIG` to choose a different file.

```toml
agent = "all"
home = ["~/.codex"]
archived = false
since = "7d"
format = "table"
```

Environment variables use the `TOTALLY_` prefix:

```text
TOTALLY_CONFIG=/path/to/config.toml
TOTALLY_AGENT=codex
TOTALLY_HOME=/path/one:/path/two
TOTALLY_ARCHIVED=true
TOTALLY_SINCE=7d
TOTALLY_UNTIL=2026-07-01
TOTALLY_FORMAT=json
```

`TOTALLY_HOME` uses the platform path-list separator.

## Examples

```sh
totally files
totally files --limit 10
totally --format json files
totally --agent codex --since 7d files
totally --home ~/.codex --archived files
TOTALLY_FORMAT=json totally files --limit 5
```
