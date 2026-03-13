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
Metrics() *data.DataFrame
```

`SetMetadata`/`GetMetadata` provide a generic key-value store for run-level information (run ID, strategy name, start/end dates, parameters, etc.). Both keys and values are strings. Backed by a `map[string]string` on `*Account`, initialized in `New()`.

`Metrics()` returns a DataFrame containing computed performance metrics. The engine populates this at each step during the backtest.

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

The Account stores registered metrics as `[]PerformanceMetric`. The engine iterates this list at each step.

### Engine Changes

#### `WithAccount` Option

New engine option that accepts a pre-configured `*Account`:

```go
engine.WithAccount(acct)
```

This replaces `WithInitialDeposit`, `WithPortfolioSnapshot`, and `WithBroker` on the engine. The user creates and fully configures the Account, then hands it to the engine.

`createAccount()` becomes: if an account was provided via `WithAccount`, use it; otherwise create a default one with all metrics and the initial deposit. The engine still provides a default SimulatedBroker if the account has no broker set.

#### Metric Computation

At each step, after `UpdatePrices`, the engine:

1. Iterates the account's registered metrics.
2. Computes each metric across all standard windows: **5yr, 3yr, 1yr, ytd, mtd, wtd, since inception**.
3. Appends the results to the account's metrics DataFrame.

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

CREATE TABLE equity_curve (
    date  TEXT NOT NULL,
    value REAL NOT NULL
);

CREATE TABLE transactions (
    date      TEXT NOT NULL,
    type      TEXT NOT NULL,  -- "buy", "sell", "dividend", "fee", "deposit", "withdrawal"
    ticker    TEXT,
    figi      TEXT,
    quantity  REAL,
    price     REAL,
    amount    REAL,
    qualified INTEGER         -- 0/1, only meaningful for dividends
);

CREATE TABLE holdings (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    quantity     REAL NOT NULL,
    avg_cost     REAL NOT NULL,
    market_value REAL NOT NULL
);

CREATE TABLE tax_lots (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    date         TEXT NOT NULL,
    quantity     REAL NOT NULL,
    price        REAL NOT NULL
);

CREATE TABLE price_series (
    series TEXT NOT NULL,     -- "benchmark" or "risk_free"
    idx    INTEGER NOT NULL,
    value  REAL NOT NULL
);

CREATE TABLE metrics (
    date   TEXT NOT NULL,
    name   TEXT NOT NULL,     -- "sharpe", "beta", "twrr", etc.
    window TEXT NOT NULL,     -- "5yr", "3yr", "1yr", "ytd", "mtd", "wtd", "since_inception"
    value  REAL
);
```

### CLI Changes

`backtest.go` becomes:

1. Create and configure `*Account` with desired metrics, cash, broker.
2. Pass to engine via `engine.WithAccount(acct)`.
3. After backtest, set metadata: `acct.SetMetadata("run_id", fullID)`, etc.
4. Call `acct.ToSQLite(path)`.

Remove `cli/output.go`, `cli/output_jsonl.go`, `cli/output_parquet.go`.

The `--output-transactions`, `--output-holdings`, `--output-metrics` flags are no longer needed -- `ToSQLite` always writes everything.

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
| Metric storage | DataFrame on portfolio, populated by engine at each step | Engine orchestrates computation, portfolio holds results |
| Metric registration | Options on `*Account` with group conveniences, default all | User controls cost, sensible default |
| Metric windows | 5yr, 3yr, 1yr, ytd, mtd, wtd, since inception | Standard analysis windows |
| Account ownership | `WithAccount` engine option | Portfolio owns its config, engine just uses it |
