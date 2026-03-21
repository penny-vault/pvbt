# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The engine validates historical data coverage before a backtest begins. Strategies declare how many trading days of warmup data they need; in strict mode the engine rejects the run if any asset falls short, and in permissive mode it shifts the start date forward until all assets have enough history.
- Strategies can weight portfolios by inverse volatility, market capitalization, or risk parity (both a fast single-pass approximation and a full iterative solver) in addition to the existing equal-weight and signal-based methods
- DataFrames expose a `Correlation` method for computing Pearson correlation between asset pairs, complementing the existing `Covariance` and `Std` methods
- DataFrames carry a reference to the engine that created them so weighting functions can fetch additional data (e.g., market cap or extended price history) on demand
- Show strategy name, schedule, parameters, and presets in a readable table when running `describe`; pass `--json` to get machine-readable output
- Select a named parameter preset with `--preset` on backtest, live, and snapshot (e.g. `--preset Classic`); explicit flags still override preset values
- Set the benchmark from the command line with `--benchmark` on backtest, live, and snapshot (e.g. `--benchmark SPY`)
- A portfolio middleware system intercepts and modifies orders between strategy execution and the broker, enabling risk management, slippage modeling, and other post-allocation processing without changing strategy code
- Built-in risk middleware enforces position size caps (`risk.MaxPositionSize`), drawdown circuit breakers (`risk.DrawdownCircuitBreaker`), position count limits (`risk.MaxPositionCount`), and inverse-volatility position scaling (`risk.VolatilityScaler`); pre-built profiles (`risk.Conservative`, `risk.Moderate`, `risk.Aggressive`) bundle common configurations
- The engine supports meta-strategies that allocate across child strategies declared as struct fields with `weight` tags; children run on their own schedules automatically, and `ChildAllocations()` expands their holdings into a flat set of underlying asset weights (including a `$CASH` sentinel for uninvested cash) ready to pass directly to `RebalanceTo`

### Changed

- **Breaking:** `engine.DescribeStrategy` now takes a `Strategy` instead of `*Engine`
- **Breaking:** Declare schedule in `Describe()` instead of calling `eng.Schedule()` in Setup
- Benchmark is now a runner concern set via `--benchmark` flag or suggested by strategies in `Describe()`; strategies should no longer call `eng.SetBenchmark()` directly
- **Breaking:** `Strategy.Compute` receives a `*portfolio.Batch` parameter; strategies write orders and annotations to the batch instead of calling methods on the portfolio directly
- **Breaking:** The `Portfolio` interface is now read-only; `RebalanceTo`, `Order`, and `Annotate` move to the `Batch` type, preventing strategies from bypassing middleware
- **Breaking:** `Annotation.Timestamp` changes from `int64` (Unix seconds) to `time.Time`; the `data.Annotator` interface and all call sites are updated accordingly
- **Breaking:** The broker delivers fills through a buffered channel (`Fills()`) instead of returning them from `Submit` and `Replace`, enabling non-blocking order execution for both backtesting and live trading

## [0.2.0] - 2026-03-17

### Added

- Display rich backtest report with performance chart, recent returns, annualized returns, annual/monthly returns, risk metrics, drawdowns, and trade log
- Show recent returns (1D, 1W, 1M, WTD, MTD, YTD) and annualized returns (1Y, 3Y, 5Y, 10Y, Since Inception) in the terminal report
- Upper-case ticker symbols provided via CLI flags
- Target a benchmark for performance metrics via the `.Benchmark()` query builder method
- Compute MonthlyReturns, AnnualReturns, and DrawdownDetails from an account
- Render terminal report using lipgloss and ntcharts
- Capture backtest data access into portable SQLite snapshots for reproducible offline testing
- Support index-based universes so strategies can filter by index membership
- Add ForwardPE, PEG, PriceToCashFlow, and Beta metrics
- Add `RiskAdjustedPct(n)` to DataFrame for computing percent change minus the risk-free return over the same period
- Attach cumulative risk-free rate series to DataFrames returned by engine Fetch and FetchAt

### Changed

- Use DGS3MO (3-month Treasury yield) as the system risk-free rate for all
  performance metrics; the rate is no longer strategy-configurable
- Compute annualization factor from actual observation frequency instead of
  hardcoding 252 or 12
- Record daily equity on every trading day regardless of strategy schedule
- Compute Jensen's alpha from mean periodic excess returns instead of total
  cumulative returns
- Annualize Treynor ratio using CAGR instead of total returns
- Compute withdrawal rates (Safe, Perpetual, Dynamic) from the actual return
  path instead of Monte Carlo bootstrap simulation

### Removed

- `engine.RiskFreeAsset()` -- the engine now uses DGS3MO automatically

### Fixed

- Load market holidays from the database so backtests skip non-trading days like Good Friday
- Honor --start, --end, and other CLI flags on the backtest, snapshot, and explore subcommands
- Fire @monthend, @weekend, and @close schedules on early-close days at the actual close time instead of skipping the day
- Snap Months(N) lookback to month boundaries so monthly downsample always yields exactly N rows
- Apply CLI flag overrides for strategy universe fields instead of silently using defaults

## [0.1.0] - 2026-03-14

### Added

- Backtest and live-trade strategies against historical or real-time market
  data using the same strategy code for both modes
- Explore backtest results in an interactive terminal UI with equity curves,
  performance metrics, and trade logs
- Evaluate strategy performance with 30+ built-in metrics covering
  risk-adjusted returns, drawdowns, capture ratios, trade round-trips, tax
  impact, and withdrawal sustainability
- Compose strategies from built-in signals (momentum, volatility, earnings
  yield), asset selectors, weighting schemes, and rating-based universe filters
- Preview upcoming trades before the next scheduled trade date
- Annotate portfolio decisions with justifications for audit trails
- Save and reload complete backtest results between sessions

[unreleased]: https://github.com/penny-vault/pvbt/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/penny-vault/pvbt/releases/tag/v0.1.0
