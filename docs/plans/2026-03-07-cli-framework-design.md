# CLI Framework Design

## Overview

A reusable CLI framework in the `cli` package that strategy authors embed to run backtests from the command line. Uses cobra/viper for CLI, charmbracelet for TUI, and automatically wires up the PVDataProvider.

## Strategy Author Usage

```go
func main() {
    cli.Run(&MyStrategy{})
}
```

## Commands

```
mystrategy backtest [flags]
mystrategy live [flags]          # stub for now
```

## Backtest Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--start` | date | 5 years ago | Backtest start date |
| `--end` | date | today | Backtest end date |
| `--cash` | float | 100000 | Initial cash balance |
| `--output` | string | `{strategy}-backtest-{YYYYMMDD}-{YYYYMMDD}-{5char}.jsonl` | Results file path |
| `--output-transactions` | bool | false | Write transaction log |
| `--output-holdings` | bool | false | Write holdings snapshots |
| `--output-metrics` | bool | false | Write rolling performance metrics |
| `--tui` | bool | false | Enable interactive TUI |
| `--log-level` | string | `"info"` | zerolog level |

Strategy-specific flags are generated via reflection from the strategy struct's exported fields using `pvbt` struct tags.

## Data Provider

Automatically creates `PVDataProvider` from `~/.pvdata.toml`. No explicit wiring needed by strategy authors.

## Output Files

All output files share a base name derived from `--output`. Format inferred from file extension (`.jsonl` or `.parquet`). Default extension is `.jsonl`.

Default base: `{strategy}-backtest-{YYYYMMDD}-{YYYYMMDD}-{5char}`

Example: `momentum-backtest-20200101-20250101-a3f2b.jsonl`

The 5-char suffix is the first 5 characters of a UUID v4. The full UUID is stored in the metadata line of the output file.

### Portfolio (always written)

File: `{base}.jsonl` (or `.parquet`)

Line 1 -- metadata:
```json
{"type": "metadata", "run_id": "a3f2b1c4-89de-4f01-b234-567890abcdef", "strategy": "momentum", "start": "2020-01-01", "end": "2025-01-01", "cash": 100000, "params": {"lookback": 90}}
```

Lines 2+ -- one per time step:
```json
{"date": "2024-01-02", "value": 105234.50, "cash": 12340.00, "invested": 92894.50, "daily_return": 0.0023, "cumulative_return": 0.0523}
```

### Transactions (opt-in: `--output-transactions`)

File: `{base}-transactions.jsonl`

```json
{"date": "2024-01-02", "action": "buy", "ticker": "AAPL", "figi": "BBG000B9XRY4", "quantity": 50, "price": 185.50, "commission": 0.00, "total": 9275.00}
```

### Holdings (opt-in: `--output-holdings`)

File: `{base}-holdings.jsonl`

```json
{"date": "2024-01-02", "holdings": [{"ticker": "AAPL", "figi": "BBG000B9XRY4", "quantity": 50, "price": 185.50, "value": 9275.00, "weight": 0.088}]}
```

### Performance Metrics (opt-in: `--output-metrics`)

File: `{base}-metrics.jsonl`

Line 1 -- metadata listing included metric groups.

Lines 2+ -- rolling values per time step:
```json
{"date": "2024-01-02", "summary": {"twrr": 0.052, "mwrr": 0.051, "sharpe": 1.23, "sortino": 1.45, "calmar": 0.89, "max_drawdown": -0.12, "std_dev": 0.04}, "risk": {"beta": 0.85, "alpha": 0.02, "tracking_error": 0.03, "downside_deviation": 0.02, "information_ratio": 0.8, "treynor": 0.15, "ulcer_index": 0.05, "excess_kurtosis": 0.3, "skewness": -0.1, "r_squared": 0.92, "value_at_risk": -0.03, "upside_capture": 1.1, "downside_capture": 0.8}, "trade": {"win_rate": 0.62, "average_win": 0.03, "average_loss": -0.015, "profit_factor": 1.8, "average_holding_period": 45, "turnover": 0.6, "n_positive_periods": 0.58, "gain_loss_ratio": 2.0}, "withdrawal": {"safe_withdrawal_rate": 0.04, "perpetual_withdrawal_rate": 0.035, "dynamic_withdrawal_rate": 0.045}}
```

## Stdout Summary

Pretty-printed summary using lipgloss after the backtest completes, showing final scalar values from all four metric groups: Summary, RiskMetrics, TradeMetrics, WithdrawalMetrics.

## TUI Mode (opt-in: `--tui`)

When enabled, replaces default log output with a bubbletea-based TUI. Uses ntcharts for the equity curve chart.

Layout:
```
+-- Equity Curve ----------------------------+-- Metrics --------------+
|                                            | Value:   $132,450      |
|                              /\  /\        | Return:  32.4%         |
|                    /\/\/\/\//  \/  \       | CAGR:    12.1%         |
|/\/\/\/\/\/\/\/\/\//                        | Sharpe:  1.42          |
|                                            | Sortino: 1.89          |
|                                            | MaxDD:   -8.2%         |
|                                            | Beta:    0.85          |
|                                            | Alpha:   2.1%          |
+-- Logs ------------------------------------+------------------------+
| 2023-06-15 INF bought AAPL qty=50 price=185.50                      |
| 2023-06-15 INF sold MSFT qty=30 price=342.10                        |
| 2023-06-14 INF rebalance complete                                    |
| 2023-06-14 INF computing signals                                     |
+----------------------------------------------------------------------+
| 2023-06-15  ================----------  64%               ETA 12s    |
+----------------------------------------------------------------------+
```

When `--tui` is not set: zerolog writes to stderr, summary prints to stdout at end.

## Architecture

| File | Contents |
|------|----------|
| `cli/run.go` | `Run(strategy)`, root cobra command, viper setup, zerolog init |
| `cli/backtest.go` | backtest subcommand: parse flags, create provider, run engine, print summary, write results |
| `cli/live.go` | live subcommand stub |
| `cli/flags.go` | Reflection-based flag generation from strategy struct fields |
| `cli/output.go` | JSONL and Parquet writers for all output streams |
| `cli/tui.go` | Bubbletea model with ntcharts equity curve, metrics sidebar, log pane, progress bar |

## Dependencies

- `cobra` / `viper` -- CLI framework
- `charmbracelet/bubbletea` -- TUI framework
- `charmbracelet/lipgloss` -- styled terminal output
- `NimbleMarkets/ntcharts` -- terminal charts for equity curve
- `parquet-go` -- Parquet file writing
- `google/uuid` -- run ID generation
- `zerolog` -- logging (already in use)
