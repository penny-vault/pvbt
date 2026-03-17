# Returns Sections Redesign

## Summary

Replace the single "Trailing Returns" table in the terminal report with two
tables that use better time periods and annualize longer-horizon returns.

## Current State

One section, "Trailing Returns", shows TWRR for 1M, 3M, 6M, YTD, 1Y, and
Since Inception. All values are non-annualized. Column width is 12 characters,
too narrow for large portfolio values in formatted output.

## New Design

### Recent Returns

Non-annualized TWRR over short horizons:

| | 1D | 1W | 1M | WTD | MTD | YTD |
|---|---|---|---|---|---|---|
| Strategy | | | | | | |
| Benchmark | | | | | | |
| +/- | | | | | | |

Periods use existing constructors: `Days(1)`, `Days(7)`, `Months(1)`,
`WTD()`, `MTD()`, `YTD()`.

`Days(1)` computes one calendar day back. On a Monday this looks back to
Sunday, but the DataFrame's time-based slicing snaps to the nearest available
trading day, so the resulting return is effectively one trading day. This
matches the existing behavior for all day-based periods and is acceptable.

### Returns

Annualized TWRR over longer horizons:

| | 1Y | 3Y | 5Y | 10Y | Since Inception |
|---|---|---|---|---|---|
| Strategy | | | | | |
| Benchmark | | | | | |
| +/- | | | | | |

Annualization formula: given TWRR `r` over a window spanning `y` years,
annualized return = `(1 + r)^(1/y) - 1`.

Year count `y` for annualization:
- For labeled periods (1Y, 3Y, 5Y, 10Y), use the nominal year count
  (1, 3, 5, 10) to avoid subtle off-by-one-day errors from
  weekends/holidays.
- For "Since Inception", compute from actual dates:
  `perfData.End().Sub(perfData.Start()).Hours() / 24 / 365.25`.

If the backtest is shorter than 1 year, "Since Inception" shows
non-annualized TWRR to avoid misleading extrapolation (e.g. a 2-month
backtest with 5% TWRR would otherwise show ~34% annualized).

### N/A Detection

For periods where the backtest is shorter than the requested window (e.g.
a 2-year backtest has no 3Y, 5Y, or 10Y data), show N/A.

Detection: `buildReturns()` computes the backtest start and end from
`acct.PerfData()`. For each period, it checks whether
`period.Before(end)` falls before `start`. If so, it emits `math.NaN()`
for that period instead of calling the metric. The renderer already
displays NaN as "N/A".

### +/- Row

The +/- row logic carries over unchanged from the current implementation.
If either the strategy or benchmark value is NaN, the difference is also
NaN (displayed as N/A).

## Data Structures

Use a single shared type for both tables:

```go
type ReturnTable struct {
    Periods   []string
    Strategy  []float64
    Benchmark []float64
}
```

The `Report` struct holds two fields:

```go
RecentReturns ReturnTable
Returns       ReturnTable
```

## Changes by File

### `report/report.go`

- Replace `TrailingReturns` struct with `ReturnTable`.
- Replace `buildTrailingReturns()` with `buildRecentReturns()` and
  `buildReturns()`.
- `buildRecentReturns()` computes TWRR for each short-horizon period.
- `buildReturns()` computes TWRR for each long-horizon period, checks
  N/A via the detection described above, then annualizes using nominal
  year counts (or date-derived for Since Inception).
- Update the `Report` struct: `RecentReturns ReturnTable` and
  `Returns ReturnTable` replace `TrailingReturns`.
- Update `Build()` to call both new builders.

### `report/terminal/returns.go`

- Replace `renderTrailingReturns()` with `renderRecentReturns()` and
  `renderReturns()`. Both can share a common `renderReturnTable()`
  helper since the rendering logic is identical.
- Increase `colWidth` from 12 to 16 to accommodate large numbers
  (e.g. long ADM runs producing values like 15M).
- Section titles: "Recent Returns" and "Returns".

### `report/terminal/renderer.go`

- Replace the `renderTrailingReturns` call with calls to
  `renderRecentReturns` and `renderReturns`.
- Render order: Recent Returns, Returns, then Annual Returns (preserving
  the position in the existing report flow).

### `report/terminal/renderer_test.go`

- Update test fixtures to use the new `ReturnTable` struct.

### No changes needed

- `data/period.go` -- `Days(n)`, `WTD()`, `MTD()`, `YTD()`, `Months(n)`,
  `Years(n)` all already exist.
- `portfolio/twrr.go` -- TWRR computation is unchanged; annualization is
  done in the report builder.
- `portfolio/cagr_metric.go` -- not used by this change.

## Testing

- Existing report tests updated for new struct shapes.
- Annualization math: e.g. 10% TWRR over 2 years produces ~4.88%
  annualized return.
- N/A shown for periods longer than backtest duration.
- +/- row produces N/A when either strategy or benchmark is N/A.
- Since Inception shows non-annualized TWRR when backtest < 1 year.
