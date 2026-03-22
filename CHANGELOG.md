# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Benchmark-relative metrics (beta, r-squared, alpha, tracking error, information ratio, downside/upside capture, and active return) no longer panic when portfolio and benchmark return columns contain NaN values at different positions. Independent NaN removal could produce mismatched slice lengths, triggering a crash in the statistics library.

### Changed

- **Breaking:** The DataFrame and metric computation internals were redesigned for performance. `NewDataFrame` accepts per-column slices instead of a flat slab, and metrics now compute against a `PortfolioStats` interface rather than a concrete `*Account`. These changes improve runtime by 9x and reduce memory by 14x over v0.3.0 -- a 30-year backtest of Accelerating Dual Momentum now takes approximately 4 seconds.

## [0.3.0] - 2026-03-21

### Added

- The engine validates historical data coverage before a backtest begins. Strategies declare how many trading days of warmup data they need; in strict mode the engine rejects the run if any asset falls short, and in permissive mode it shifts the start date forward until all assets have enough history.
- Strategies can weight portfolios by inverse volatility, market capitalization, or risk parity (both a fast single-pass approximation and a full iterative solver) in addition to the existing equal-weight and signal-based methods
- DataFrames expose a `Correlation` method for computing Pearson correlation between asset pairs, complementing the existing `Covariance` and `Std` methods
- Weighting functions like market-cap weighting automatically fetch the additional data they need (e.g., market cap or extended price history) through the DataFrame's engine reference
- The `describe` command displays strategy name, schedule, parameters, and presets in a readable table; `--json` produces machine-readable output
- The `--preset` flag on backtest, live, and snapshot selects a named parameter preset (e.g., `--preset Classic`); explicit flags still override preset values
- The `--benchmark` flag on backtest, live, and snapshot sets the benchmark from the command line (e.g., `--benchmark SPY`)
- A portfolio middleware system intercepts and modifies orders between strategy execution and the broker, enabling risk management, slippage modeling, and other post-allocation processing without changing strategy code
- Built-in risk middleware enforces position size caps, drawdown circuit breakers, position count limits, and inverse-volatility position scaling. Pre-built profiles (`risk.Conservative`, `risk.Moderate`, `risk.Aggressive`) bundle common configurations.
- Tax optimization middleware harvests losses automatically, tracks wash sales with IRS-compliant basis adjustment, and swaps to correlated substitute assets to avoid wash sale windows. Accounts choose FIFO, LIFO, or highest-cost-first lot selection as a default via `WithDefaultLotSelection` or per-order via `WithLotSelection`. Strategies continue to see original asset tickers in `Holdings()` and `ProjectedWeights()` even when the middleware has swapped to a substitute, while the transaction log reflects the actual positions. The `tax.TaxEfficient` profile bundles these into a single middleware chain.
- A tax drag metric measures the percentage of pre-tax return consumed by trading-related taxes, excluding dividend taxation, complementing the existing tax cost ratio
- The engine supports meta-strategies that allocate across child strategies declared as struct fields with `weight` tags; children run on their own schedules automatically, and `ChildAllocations()` expands their holdings into a flat set of underlying asset weights ready to pass directly to `RebalanceTo`
- The engine tracks Maximum Favorable Excursion (MFE) and Maximum Adverse Excursion (MAE) for every trade, measuring how far prices moved for and against each position before exit; per-trade details are available via `TradeDetails()` and summary statistics are added to the TradeMetrics bundle
- The tastytrade broker integration implements the Broker interface for live equity trading, with WebSocket fill streaming, automatic session management, and a sandbox mode for paper trading
- Bracket and OCO (one-cancels-other) order groups let strategies submit linked exit orders. A bracket combines an entry order with a stop-loss and take-profit that activate only after the entry fills, while an OCO pair cancels the remaining leg as soon as one fills. Brokers that support native group submission implement the optional `GroupSubmitter` interface.
- The simulated broker resolves intrabar bracket and OCO fills using high/low price data, with stop-loss taking priority when both legs could trigger on the same bar
- Strategies can open and manage short positions: sell-to-open creates short tax lots with proper cost basis tracking, buy-to-cover closes them, and unrealized P&L includes short lots alongside long lots
- Margin accounting is fully modeled on the simulated broker: initial margin defaults to 50% (Reg T) and maintenance margin defaults to 30%, both configurable; `Portfolio` exposes `MarginRatio`, `MarginDeficiency`, `ShortMarketValue`, `BuyingPower`, and `Equity` methods; the broker enforces initial margin requirements when submitting short orders
- Borrow fees accrue daily on short positions at a configurable annualized rate (default 0.5%), and short positions on ex-dates automatically receive dividend obligation debits
- Strategies that implement the optional `MarginCallHandler` interface receive control when a margin deficiency occurs; strategies that do not implement it fall back to automatic proportional liquidation of positions. Emergency liquidation orders set `Batch.SkipMiddleware` to bypass the middleware chain.
- Stock splits adjust position quantities and tax lot prices for both long and short positions, and the transaction log records a `SplitTransaction` entry for each affected position
- `RebalanceTo` accepts negative weights to declare short allocations using the industry-standard sign convention; risk middleware applies position size limits symmetrically to shorts, and two new middleware — `GrossExposureLimit` and `NetExposureLimit` — bound overall leverage
- Performance metrics include `ShortWinRate`, `LongWinRate`, `ShortProfitFactor`, and `LongProfitFactor` to break down trade statistics separately for long and short sides


### Changed

- **Breaking:** `engine.DescribeStrategy` takes a `Strategy` instead of an `*Engine`, decoupling description from the running engine
- **Breaking:** Strategies declare their schedule in `Describe()` instead of calling `eng.Schedule()` during `Setup`
- **Breaking:** Benchmark moves from a strategy concern to a runner concern, set via the `--benchmark` flag or suggested by strategies in `Describe()`; `eng.SetBenchmark()` is removed
- **Breaking:** `Strategy.Compute` receives a new `*portfolio.Batch` parameter; strategies write orders and annotations to the batch instead of calling methods on the portfolio directly
- **Breaking:** The `Portfolio` interface becomes read-only; `RebalanceTo`, `Order`, and `Annotate` move to the `Batch` type, preventing strategies from bypassing middleware. The interface also gains `Prices()`, `TradeDetails()`, and six margin methods (`Equity`, `LongMarketValue`, `ShortMarketValue`, `MarginRatio`, `MarginDeficiency`, `BuyingPower`).
- **Breaking:** The `PortfolioSnapshot` interface gains `TradeDetails()`, `Excursions()`, and `ShortLots()` methods
- **Breaking:** `TradeDetail` gains a `Direction` field of type `TradeDirection`; code constructing `TradeDetail` with positional struct literals will not compile (named-field literals are unaffected, with `Direction` defaulting to `TradeLong`)
- **Breaking:** `TradeMetrics` gains four new fields (`LongWinRate`, `ShortWinRate`, `LongProfitFactor`, `ShortProfitFactor`); same positional literal concern as `TradeDetail`
- **Breaking:** `Record()` now creates short tax lots when a sell exceeds the long position, and covers short lots when a buy is recorded while short; code that relied on sells silently producing negative holdings without lot tracking will see different behavior
- **Breaking:** The engine housekeeping order changed from dividends-then-fills to fills-then-splits-then-dividends, which may produce slightly different dividend amounts when a fill and dividend occur on the same day
- **Breaking:** The simulated broker now silently rejects short sell orders that would violate the initial margin requirement (returns no fill instead of always filling)
- **Breaking:** `universe.DataSource` becomes a type alias for `data.DataSource`; the `Fetch` lookback parameter changes from `portfolio.Period` to `data.Period`
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

[unreleased]: https://github.com/penny-vault/pvbt/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/penny-vault/pvbt/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/penny-vault/pvbt/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/penny-vault/pvbt/releases/tag/v0.1.0
