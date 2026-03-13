# Portfolio Serialization Design

**Date:** 2026-03-12
**Status:** Draft

## Problem

Serialization logic lives in the CLI package (`cli/output.go`, `cli/output_jsonl.go`, `cli/output_parquet.go`). This means:

- Non-CLI consumers (tests, libraries, other tools) cannot save/load portfolio state without duplicating format logic.
- Round-trip save/restore (`PortfolioSnapshot`) cannot work through a file format.
- Output format decisions are coupled to the CLI instead of the domain.

## Goals

1. The portfolio is self-describing and serializable -- any consumer can save/load without the CLI.
2. Round-trip save/restore works through a file format, enabling engine resume from serialized state.
3. The CLI delegates to the portfolio for serialization and no longer owns format logic.

## Design

### Portfolio Interface Additions

Add to the `Portfolio` interface:

```go
SetMetadata(key, value string)
GetMetadata(key string) string
```

`SetMetadata`/`GetMetadata` provide a generic key-value store for run-level information (run ID, strategy name, start/end dates, parameters, etc.). Both keys and values are strings. Backed by a `map[string]string` on `*Account`, initialized in `New()`.

Note: while `SetMetadata` is a mutation method on the strategy-facing interface, metadata is useful for strategies to annotate their runs (e.g., regime labels, signal descriptions). This is distinct from allocation/trading operations.

### Metric Storage

`Metrics()` is a method on `*Account` only (not on the `Portfolio` interface) since strategies should not access the metrics DataFrame during `Compute`. It returns the computed metrics accumulated by the engine.

Metric results are stored as a flat slice on `*Account`:

```go
type MetricRow struct {
    Date   time.Time
    Name   string  // "sharpe", "beta", etc.
    Window string  // "5yr", "3yr", "1yr", "ytd", "mtd", "wtd", "since_inception"
    Value  float64
}
```

This maps directly to the SQLite `metrics` table without any impedance mismatch. A `*data.DataFrame` is not used here because metrics have three dimensions (date, name, window) which do not fit the DataFrame's two-dimensional `(asset, metric)` layout.

### Metric Registration on Portfolio

Metrics are registered on the Account via options:

```go
// Individual metric
portfolio.WithMetric(portfolio.Sharpe)

// Group conveniences (matching existing metric groups)
portfolio.WithSummaryMetrics()      // TWRR, MWRR, Sharpe, Sortino, Calmar, MaxDrawdown, StdDev
portfolio.WithRiskMetrics()         // Beta, Alpha, TrackingError, DownsideDeviation, etc.
portfolio.WithTradeMetrics()        // WinRate, ProfitFactor, AverageHoldingPeriod, etc.
portfolio.WithWithdrawalMetrics()   // SafeWithdrawalRate, PerpetualWithdrawalRate, etc.
portfolio.WithTaxMetrics()          // LTCG, STCG, UnrealizedLTCG, etc.
portfolio.WithAllMetrics()          // all of the above

// Default: all metrics are registered if none are specified.
```

The Account stores registered metrics as `[]PerformanceMetric`. The engine iterates this list at each step. A `RegisteredMetrics()` accessor on `*Account` exposes the list to the engine.

### Standard Windows

The standard windows are: **5yr, 3yr, 1yr, ytd, mtd, wtd, since inception**.

Calendar-relative windows (ytd, mtd, wtd) require the current step date to compute the start of the window. The existing `Period` type supports `Days(n)`, `Months(n)`, `Years(n)` but has no concept of "year-to-date." New `PeriodUnit` values are added:

```go
const (
    UnitDay PeriodUnit = iota
    UnitMonth
    UnitYear
    UnitYTD   // year-to-date: from Jan 1 of the current year
    UnitMTD   // month-to-date: from the 1st of the current month
    UnitWTD   // week-to-date: from the most recent Monday
)
```

The `windowSlice` helper in `metric_helpers.go` is updated to handle these units by computing the start date relative to the current step date rather than using a fixed lookback count.

"Since inception" is represented by passing a nil window, which already means "use the full history."

### Engine Changes

#### `WithAccount` Option

New engine option that accepts a pre-configured `*Account`:

```go
engine.WithAccount(acct)
```

This provides an alternative to the existing `WithInitialDeposit`, `WithPortfolioSnapshot`, and `WithBroker` engine options. When `WithAccount` is used, the engine uses the provided account directly. Precedence rules:

- If `WithAccount` is set, it takes priority. `WithInitialDeposit`, `WithPortfolioSnapshot`, and `WithBroker` are ignored with a warning log.
- If `WithAccount` is not set, the existing options continue to work unchanged. This preserves backward compatibility.

`createAccount()` becomes: if an account was provided via `WithAccount`, use it; otherwise create a default one with all metrics and the initial deposit. The engine still provides a default SimulatedBroker if the account has no broker set.

#### Metric Computation

At each step, after `UpdatePrices`, the engine:

1. Iterates the account's registered metrics via `RegisteredMetrics()`.
2. Computes each metric across all standard windows: **5yr, 3yr, 1yr, ytd, mtd, wtd, since inception**.
3. Appends `MetricRow` entries to the account's metrics slice.

**Performance note:** With ~50 metrics, 7 windows, and ~1260 trading days in a 5-year backtest, this produces ~440,000 metric computations. Each `Compute` call scans the equity curve up to the current step, making the overall cost O(n^2) in the number of steps. This is acceptable for the current use case but should be profiled. If it becomes a bottleneck, future optimizations include:
- Computing metrics at a lower frequency (e.g., monthly snapshots).
- Incremental/streaming metric implementations that avoid re-scanning the full history.

### Serialization Methods

On `*Account`:

```go
func (a *Account) ToSQLite(path string) error
```

Package-level constructor:

```go
func FromSQLite(path string) (*Account, error)
```

`ToSQLite` writes the full portfolio state to a SQLite database. `FromSQLite` restores a full `*Account` that satisfies `PortfolioSnapshot` -- usable for engine resume via `WithPortfolioSnapshot` or `WithAccount`.

Uses `modernc.org/sqlite` (pure Go, no CGO).

### SQLite Schema

```sql
CREATE TABLE metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);

-- Schema versioning: metadata always contains key="schema_version", value="1"

CREATE TABLE equity_curve (
    date  TEXT NOT NULL,
    value REAL NOT NULL
);

CREATE TABLE transactions (
    date      TEXT NOT NULL,
    type      TEXT NOT NULL,  -- "buy", "sell", "dividend", "fee", "deposit", "withdrawal"
    ticker    TEXT,           -- NULL for cash-only events (deposits, withdrawals, fees)
    figi      TEXT,           -- NULL for cash-only events
    quantity  REAL,
    price     REAL,
    amount    REAL,
    qualified INTEGER         -- 0=false, 1=true; only meaningful for dividends
);

CREATE TABLE holdings (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    quantity     REAL NOT NULL,
    avg_cost     REAL NOT NULL,
    market_value REAL NOT NULL
);

-- holdings captures the final portfolio state only. Historical holdings
-- can be derived by replaying the transactions table.
--
-- avg_cost is computed from tax lots: sum(lot_price * lot_qty) / total_qty
-- market_value is quantity * last known close price from the final step's DataFrame.

CREATE TABLE tax_lots (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    date         TEXT NOT NULL,
    quantity     REAL NOT NULL,
    price        REAL NOT NULL
);

CREATE TABLE price_series (
    series TEXT NOT NULL,     -- "benchmark" or "risk_free"
    date   TEXT NOT NULL,     -- aligned with equity_curve dates
    value  REAL NOT NULL
);

-- Benchmark and risk-free asset identity stored in metadata:
--   benchmark_ticker, benchmark_figi, risk_free_ticker, risk_free_figi
--
-- Price series are aligned with equity_curve: one entry per step.
-- UpdatePrices always appends to both series (using the previous value
-- when a NaN is encountered) to maintain length alignment.

CREATE TABLE metrics (
    date   TEXT NOT NULL,
    name   TEXT NOT NULL,     -- "sharpe", "beta", "twrr", etc.
    window TEXT NOT NULL,     -- "5yr", "3yr", "1yr", "ytd", "mtd", "wtd", "since_inception"
    value  REAL
);

CREATE INDEX idx_metrics_date ON metrics(date);
CREATE INDEX idx_metrics_name ON metrics(name);
CREATE INDEX idx_transactions_date ON transactions(date);
```

### CLI Changes

`backtest.go` becomes:

1. Create and configure `*Account` with desired metrics, cash, broker.
2. Pass to engine via `engine.WithAccount(acct)`.
3. After backtest, set metadata: `acct.SetMetadata("run_id", fullID)`, etc.
4. Call `acct.ToSQLite(path)`.

Remove `cli/output.go`, `cli/output_jsonl.go`, `cli/output_parquet.go`.

The `--output-transactions`, `--output-holdings`, `--output-metrics` flags are no longer needed -- `ToSQLite` always writes everything.

### File Placement

Following the one-type-per-file convention:

- `portfolio/metadata.go` -- `SetMetadata`/`GetMetadata` implementation, metadata map on Account
- `portfolio/metric_row.go` -- `MetricRow` type, `Metrics()` accessor, `RegisteredMetrics()` accessor
- `portfolio/metric_registration.go` -- `WithMetric`, `WithSummaryMetrics`, `WithAllMetrics`, etc.
- `portfolio/sqlite.go` -- `ToSQLite` and `FromSQLite`

### Usage Example

```go
acct := portfolio.New(
    portfolio.WithCash(100000),
    portfolio.WithBroker(myBroker),
    portfolio.WithSummaryMetrics(),
    portfolio.WithRiskMetrics(),
)

eng := engine.New(strategy,
    engine.WithDataProvider(provider),
    engine.WithAccount(acct),
)
defer eng.Close()

p, err := eng.Backtest(ctx, start, end)
if err != nil {
    return err
}

p.SetMetadata("run_id", fullID)
p.SetMetadata("strategy", strategy.Name())
p.SetMetadata("start", start.Format("2006-01-02"))
p.SetMetadata("end", end.Format("2006-01-02"))

if err := acct.ToSQLite("output.db"); err != nil {
    return err
}

// Later, restore:
restored, err := portfolio.FromSQLite("output.db")
eng2 := engine.New(strategy,
    engine.WithAccount(restored),
)
```

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Format | SQLite (pure Go via modernc.org/sqlite) | Natural table structure, no CGO, queryable |
| Serialization location | `portfolio` package | Portfolio owns its own representation |
| API style | `ToSQLite`/`FromSQLite` methods on `*Account` | Simple, discoverable |
| Metadata | `SetMetadata`/`GetMetadata` string key-value on `Portfolio` interface | Flexible, easy to serialize |
| Metric storage | `[]MetricRow` on `*Account`, populated by engine at each step | Direct mapping to SQLite, avoids DataFrame shape mismatch |
| Metric registration | Options on `*Account` with group conveniences, default all | User controls cost, sensible default |
| Metric windows | 5yr, 3yr, 1yr, ytd, mtd, wtd, since inception | Standard analysis windows |
| Account ownership | `WithAccount` engine option (additive, not replacing old options) | Portfolio owns its config, backward compatible |
| Benchmark/risk-free identity | Stored in metadata table | Enables round-trip restore of asset identity |
| Holdings table | Final state only, avg_cost derived from tax lots | Historical holdings derived from transactions |
| Schema versioning | `schema_version` key in metadata | Future-proofs format evolution |
