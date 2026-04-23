# Design: `backtest --json` mode

**Date:** 2026-04-22
**Status:** Approved

## Overview

Add a `--json` flag to the `backtest` subcommand that switches stdout to a JSON Lines stream. All log messages and lifecycle/progress events are emitted as single-line JSON objects. This is intended for consumption by a web UI that spawns the backtest process and reads its stdout. Full metrics and the equity curve are fetched from the SQLite output file after the run completes.

## Activation

`--json` is a boolean flag on `newBacktestCmd()`. When set:

- The bubble tea TUI is skipped (equivalent to `--no-progress`)
- zerolog is configured with `zerolog.New(os.Stdout)` (native JSON output, no console writer)
- `summary.Render()` is suppressed
- A `jsonReporter` helper (private, in `cli/`) handles all structured writes to stdout

The flag is mutually exclusive with the interactive TUI. If both would otherwise activate, `--json` wins.

## Message Schema

All messages are single-line JSON objects. A `type` field discriminates between message kinds. Consumers should ignore unknown `type` values to allow forward compatibility.

### `status` — lifecycle events

Emitted at key points in the run lifecycle.

**started** — emitted before engine execution begins:
```json
{"type":"status","event":"started","run_id":"abc12","strategy":"momentum-factor","start":"2020-01-01","end":"2025-01-01","cash":100000,"output":"/path/to/run.db","time":"2026-04-22T14:01:02Z"}
```

**completed** — emitted after SQLite output is written:
```json
{"type":"status","event":"completed","run_id":"abc12","output":"/path/to/run.db","elapsed_ms":14200,"time":"2026-04-22T14:01:16Z"}
```

**error** — emitted instead of `completed` when the run fails:
```json
{"type":"status","event":"error","run_id":"abc12","error":"data provider unavailable","time":"2026-04-22T14:01:06Z"}
```

### `progress` — backtest execution progress

Emitted on each progress callback invocation from the engine. Fields mirror what the bubble tea TUI displays.

```json
{"type":"progress","step":1234,"total_steps":5000,"current_date":"2023-05-15","target_date":"2025-01-01","pct":24.68,"elapsed_ms":3200,"eta_ms":9800,"measurements":45231}
```

Fields:
- `step` / `total_steps` — current and total engine steps
- `current_date` — the trading date currently being processed (YYYY-MM-DD)
- `target_date` — the backtest end date (YYYY-MM-DD)
- `pct` — completion percentage (0–100, two decimal places)
- `elapsed_ms` — wall-clock milliseconds since engine start
- `eta_ms` — estimated milliseconds remaining (0 if unknown)
- `measurements` — number of data measurements processed so far

### `log` — structured log messages

zerolog's native JSON output. All log events that would normally go to stderr as formatted text instead go to stdout as JSON.

```json
{"type":"log","level":"info","time":"2026-04-22T14:01:05Z","message":"running backtest","strategy":"momentum-factor"}
```

Additional zerolog fields (e.g. `strategy`, `start`, `end`, `path`) are included as top-level keys when present.

## Implementation

### Files changed

**`cli/backtest.go`**
- Register `--json` boolean flag in `newBacktestCmd()`
- In `runBacktest()`: when `--json` is set, configure zerolog as `zerolog.New(os.Stdout).With().Str("type", "log").Logger()` (adds `"type":"log"` to every log line), construct a `jsonReporter`, emit `started`, swap the progress callback to call `reporter.Progress()`, skip `summary.Render()`, emit `completed` or `error` at the end

**`cli/json_reporter.go`** (new file)
- `jsonReporter` struct with an `io.Writer` (stdout)
- `Status(event string, fields map[string]any)` — marshals and writes a status line
- `Progress(update progressUpdateMsg)` — marshals and writes a progress line
- All writes are a single `json.Marshal` + `fmt.Fprintln`; no buffering

### No changes needed

`engine/`, `portfolio/`, `summary/`, and `cli/progress.go` are unchanged. The existing progress callback abstraction is sufficient.

## Constraints

- Output must be JSON Lines (one JSON object per line, newline-terminated)
- No ANSI escape codes in JSON mode
- `elapsed_ms` and `eta_ms` are integers (milliseconds); consumers should not rely on sub-millisecond precision
- Dates are RFC 3339 for timestamps, YYYY-MM-DD for trading dates
- Unknown fields should be tolerated by consumers (forward compatibility)
