# Explore Command Design

## Overview

A standalone CLI tool (`pvbt explore`) for querying and visualizing data from the PVDataProvider. Primarily a debugging tool for inspecting what the data provider returns for given tickers, metrics, and date ranges.

## Invocation

```
pvbt explore AAPL,MSFT AdjClose,Volume [flags]
pvbt explore --list-metrics
```

- First positional arg: comma-separated tickers
- Second positional arg: comma-separated metric names (exact `data.Metric` constant names)
- `--start`: start date (default: 1 year ago, format YYYY-MM-DD)
- `--end`: end date (default: today, format YYYY-MM-DD)
- `--graph`: launch full-screen TUI with line chart instead of printing table
- `--list-metrics`: print all available metric constant names and exit
- `--log-level`: zerolog level (default: info)

Metric names use the exact Go constant names: `MetricOpen`, `MetricHigh`, `MetricLow`, `MetricClose`, `AdjClose`, `Volume`, `PE`, `MarketCap`, etc.

## Default Output (stdout table)

Lipgloss-styled aligned columns. Dates down the left, one column per ticker/metric pair:

```
Date        AAPL AdjClose  AAPL Volume   MSFT AdjClose  MSFT Volume
2024-01-02  185.50         50,234,100    375.20         22,145,300
2024-01-03  186.10         48,123,400    376.80         20,987,600
```

## Graph Mode (`--graph`)

Full-screen bubbletea TUI with an ntcharts line chart. One series per ticker/metric combination. Useful for visually inspecting price series, spotting gaps, or comparing assets.

## `--list-metrics`

Prints all available `data.Metric` constants grouped by source (EOD, Valuation, Fundamentals, etc.) and exits. No tickers or date range required.

## Data Flow

1. Parse positional args into ticker list and metric list.
2. Create `PVDataProvider` from `~/.pvdata.toml` (same as backtest command).
3. Resolve each ticker via `PVDataProvider.LookupAsset()` to get `asset.Asset` with composite FIGI.
4. Map metric name strings to `data.Metric` constants.
5. Build a `data.DataRequest` and call `PVDataProvider.Fetch()`.
6. Extract the DataFrame into rows for table display or time series for graphing.

## Architecture

| File | Contents |
|------|----------|
| `cmd/explore/main.go` | Standalone binary: `func main() { cli.RunExplore() }` |
| `cli/explore.go` | `RunExplore()`, cobra command, arg parsing, data fetch, table rendering |

## Dependencies

No new dependencies. Uses existing: cobra, viper, lipgloss, bubbletea, ntcharts, zerolog, pgx.
