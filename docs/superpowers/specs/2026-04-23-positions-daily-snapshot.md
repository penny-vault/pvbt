# Design: per-ticker daily positions in snapshot (`positions_daily`)

**Date:** 2026-04-23
**Status:** Implemented
**Consumer:** `pv-api` — spec at
`pv-api/docs/superpowers/specs/2026-04-23-portfolio-holdings-impact.md`

## Overview

pv-api needs a new endpoint `GET /portfolios/{slug}/holdings-impact` that
reports each ticker's contribution to portfolio return over canonical
periods (YTD, 1Y, 3Y, 5Y, inception). Contribution math requires a per-day,
per-ticker market-value series:

```
pnl_i(t0, t1] = (mv_i(t1) - mv_i(t0)) - netFlows_i(t0, t1]
contribution_i = pnl_i / V(t0)
```

That series is not in the snapshot today:

- `perf_data` stores portfolio-aggregate metrics only (`PortfolioEquity`,
  `PortfolioBenchmark`, etc.) — `writePerfData` in `portfolio/sqlite.go`
  iterates `MetricList()` but only reads `Column(portfolioAsset, m)`.
- `holdings` is a current-day snapshot (last row-set only).
- `transactions` is an event log without daily valuations.
- `Account.perfData` in memory is keyed by `(asset, metric)` but only the
  `portfolioAsset` column is ever written.

pv-api cannot compute contribution without replaying prices against
quantities, which defeats the point of the snapshot being self-contained.
Fix by recording per-ticker daily position value at backtest time.

## Goals

1. Persist, for every trading day in the run, one row per held position
   capturing market value and quantity.
2. No breaking changes to any existing snapshot reader.
3. Deterministic: re-running the same backtest produces identical rows.
4. $CASH participates like any other ticker (so cash drag/interest gets a
   contribution line in the API rather than being silently absorbed).

## Non-goals

- Per-ticker *returns*, weights, or attribution math. pv-api computes those
  at read time.
- Per-ticker indicator metrics (that belongs in the existing `metrics`
  table, unchanged).
- Backfilling existing snapshots. Snapshots older than this change won't
  carry the new table; consumers handle absence.

## Schema changes

### New table (added to `createSchema` in `portfolio/sqlite.go`)

```sql
CREATE TABLE positions_daily (
  date         TEXT NOT NULL,
  ticker       TEXT NOT NULL,
  figi         TEXT NOT NULL,
  market_value REAL NOT NULL,
  quantity     REAL NOT NULL,
  PRIMARY KEY (date, ticker, figi)
);

CREATE INDEX idx_positions_daily_ticker ON positions_daily (ticker, date);
```

Bump `schemaVersion` from `"4"` to `"5"`.

### Row semantics

- One row per `(date, ticker, figi)` where the account held a non-zero
  position at market close on `date` *or* the position was closed during
  `date` (still emit with `quantity=0`, `market_value=0` so pv-api can see
  the exit day). A ticker the account has never touched produces no rows.
- `market_value` uses the same `data.MetricClose` price source
  `writeHoldings` uses for its end-of-run snapshot. If close is NaN for a
  given day (e.g., market holiday, stale data), use the last known close —
  if none exists yet, skip the row for that day and log at DEBUG.
- `$CASH` emits a row every trading day for the life of the account with
  `quantity = market_value = cash_balance`, `figi = ''`. This includes days
  where cash balance is zero (emit the row so pv-api sees the zero and
  doesn't treat it as "no data").
- Splits: because we store the *day's* market value (post-split price ×
  post-split quantity), splits are transparent. No special handling.
- Dividend: on the ex/pay date, the dividend creates a cash flow in
  `transactions` and bumps `$CASH.market_value` accordingly. The paying
  ticker's own row reflects its post-ex-date market value. pv-api credits
  dividends back to the paying ticker via the transactions table; pvbt
  does not need to encode that attribution here.

## Implementation

### Tracking per-ticker daily MV at run time

`Account.perfData` today is a `data.DataFrame` keyed by `(asset, metric)`
but only the `portfolioAsset` row is populated. Extend the account's
per-step bookkeeping (wherever `perfData` for the aggregate is stamped each
simulation step — likely in `account.go`'s step/mark-to-market path) to
also insert, for each currently-held asset:

- `perfData.Insert(ast, data.PositionMarketValue, series)`
- `perfData.Insert(ast, data.PositionQuantity,    series)`

Two new metric names added to `data/`:

```go
const (
    PositionMarketValue Metric = "PositionMarketValue"
    PositionQuantity    Metric = "PositionQuantity"
)
```

Alternative considered: maintain a side-car `map[asset.Asset][]positionDay`
on the `Account`, independent of `perfData`. Rejected — `perfData` already
has the date-index machinery, DataFrame slicing, NaN-aware iteration, and
is what `writePerfData` consumes. Reusing it keeps one code path, and the
columns pay for themselves if anyone later wants a per-ticker chart.

### New writer: `writePositionsDaily`

In `portfolio/sqlite.go`, next to `writePerfData` / `writeHoldings`:

```go
func (a *Account) writePositionsDaily(tx *sql.Tx) error {
    if a.perfData == nil {
        return nil
    }

    stmt, err := tx.Prepare(`INSERT INTO positions_daily
        (date, ticker, figi, market_value, quantity) VALUES (?, ?, ?, ?, ?)`)
    if err != nil {
        return fmt.Errorf("prepare positions_daily: %w", err)
    }
    defer stmt.Close()

    times := a.perfData.Times()
    for _, ast := range a.perfData.Assets() {
        if ast == portfolioAsset {
            continue
        }
        mvCol := a.perfData.Column(ast, data.PositionMarketValue)
        qCol  := a.perfData.Column(ast, data.PositionQuantity)
        for i := range times {
            mv, q := mvCol[i], qCol[i]
            if math.IsNaN(mv) && math.IsNaN(q) {
                continue
            }
            if math.IsNaN(mv) { mv = 0 }
            if math.IsNaN(q)  { q = 0 }
            d := times[i].Format(dateFormat)
            if _, err := stmt.Exec(d, ast.Ticker, ast.CompositeFigi, mv, q); err != nil {
                return fmt.Errorf("insert positions_daily: %w", err)
            }
        }
    }
    return nil
}
```

Call order in `Account.Save` (or whichever top-level writer composes the
per-table writers): after `writePerfData`, before `writeHoldings`. All
writes share the single transaction already in use.

### `data.DataFrame.Assets()` helper (if missing)

If the DataFrame does not already expose the set of asset keys, add a
`func (d *DataFrame) Assets() []asset.Asset` that returns them in a stable
order (sorted by ticker, with `portfolioAsset` last). Used by the writer
above.

## Test plan

- **Unit (`portfolio/sqlite_test.go` or sibling)**:
  - Two-asset account (VTI, BND), 30 trading days: assert exactly
    `30 * 3` rows (VTI + BND + $CASH) with mv and quantity matching
    an in-memory recomputation.
  - Mid-period buy: asset's first row appears on purchase day with
    non-zero quantity; no rows for that asset before that date.
  - Mid-period close: row with `quantity=0, market_value=0` on the day
    after the last sale; no further rows.
  - Split: quantity doubles on ex-date, mv stable — round-trip through
    snapshot preserves the values exactly.
  - NaN-price day for one ticker (simulate stale data): row either uses
    last close or is skipped (assert whichever policy you land on, above
    default is "skip + DEBUG log").
  - $CASH: rows every day, even when balance is 0.
- **Integration**: extend an existing end-to-end test (`cli/cli_test.go` or
  `study/integration_test.go`) to assert `positions_daily` row count ≥
  `days * assets` and that `sum(mv) per date` matches
  `perf_data.PortfolioEquity` for that date within 1e-6.
- **Regression**: existing snapshot tests must keep passing unchanged.
- **Schema version**: bump to `"5"` and add a test asserting the schema
  version written to `metadata`.

## Acceptance criteria

A fresh backtest writes a snapshot such that:

1. `SELECT count(*) FROM positions_daily` > 0 for any non-trivial run.
2. For every `date` present in `positions_daily`:
   `abs(sum(market_value) where date=? group by date - perf_data.value
        where metric='PortfolioEquity' and date=?) < 1e-4`.
3. `PRAGMA table_info(positions_daily)` matches the DDL above (column
   names, types, order, PK).
4. `metadata.schema_version == '5'`.
5. No existing snapshot consumer in pvbt or pv-api regresses on a rerun of
   its test suite.

## Out of scope / explicitly deferred

- Per-ticker cost basis over time. (pv-api derives it from `transactions`
  and `tax_lots`; no new column needed.)
- Per-ticker realised vs. unrealised split. Contribution math doesn't need
  it; a future attribution endpoint might. File as a follow-up.
- Daily per-ticker returns. Computed downstream; not persisted.
- Migration of old snapshots. See §Non-goals.

## Coordination

pv-api's consumer work (`holdings-impact` endpoint) is blocked on this. The
pv-api spec assumes:

- Table name: `positions_daily`.
- Column names and types exactly as above.
- `$CASH` encoded with `ticker='$CASH'`, `figi=''`.
- Daily granularity; one row per trading day per position.

Any divergence should be agreed before merge so pv-api can match.
