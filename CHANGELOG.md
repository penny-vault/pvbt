# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.8.2] - 2026-05-02

### Fixed

- Backtests no longer liquidate every held position when the end date runs one or two trading days past the data feed.

## [0.8.1] - 2026-05-01

### Changed

- Strategies that reference economic indicators must use the `FRED:` namespace prefix (e.g. `FRED:DGS3MO`); FRED data is now sourced from the dedicated `economic_indicators` table in pvdb rather than the end-of-day asset table. The risk-free rate identifier reported by `describe` and stored in snapshots is now `FRED:DGS3MO`.

## [0.8.0] - 2026-04-24

### Added

- Snapshots now include a `positions_daily` table recording each ticker's end-of-day market value and quantity, so consumers can compute per-ticker contribution without replaying prices. `$CASH` participates as a position with an empty `figi`, and rows are emitted every trading day including zero-balance days.

### Changed

- The snapshot schema version is now `5`. Snapshots produced by earlier releases can no longer be read; re-run the backtest to regenerate.

## [0.7.7] - 2026-04-23

### Added

- The `backtest` command now accepts a `--json` flag that writes all output as JSON Lines to stdout. Each line is a self-contained object with a `type` field (`status`, `progress`, or `log`), making it straightforward to pipe backtest runs into other tools or CI pipelines without screen-scraping the interactive UI.

## [0.7.6] - 2026-04-21

### Added

- Snapshots now record a monotonic batch id on every transaction and annotation and include a new `batches` table, so tools can reconstruct the portfolio's holdings after each batch.
- The backtest risk summary now reports three Ulcer Index variants alongside the point-in-time value: average, 90th-percentile, and median of the rolling series over the full window.
- When the interactive progress bar is active, backtest logs are written to a plain-text file beside the output database (`<output-stem>.log`) instead of being buffered and dumped to stderr. The log path is printed to stderr at startup.

### Changed

- The snapshot schema version is now `4`. Snapshots produced by earlier releases can no longer be read; re-run the backtest to regenerate.

### Fixed

- The Ulcer Index now uses the double-rolling formula matching TradingView's definition: each bar's drawdown is measured against its own 14-day rolling high, then the index is the RMS of those drawdowns over the last 14 bars. The previous implementation anchored the peak to the first bar of the outer window, systematically underreporting the index for long series.

## [0.7.5] - 2026-04-18

### Fixed

- Snapshot replay no longer returns an empty screen when a `FetchFundamentalsByDateKey` call targets a reporting period that falls on a trading day (e.g. `2024-12-31`). The recorder's upsert now propagates `date_key` and `report_period` into existing rows instead of leaving them NULL on conflict.

## [0.7.4] - 2026-04-18

### Fixed

- Snapshot capture no longer fails with "no provider supports FundamentalsByDateKeyProvider" for strategies that call `Engine.FetchFundamentalsByDateKey`. The recorder now delegates the call to the wrapped provider and stores the result so replay sees the same values.

## [0.7.3] - 2026-04-18

### Added

- Strategies can cap which filings are considered available to `Engine.FetchFundamentalsByDateKey` by passing `engine.WithAsOfDate`. This lets a screen use an earlier "formation date" (e.g. March 31 fundamentals) even when the strategy rebalances later in the year.

### Fixed

- The engine-package documentation now lists `WithBenchmarkTicker`, `WithFillModel`, `WithMiddlewareConfig`, and `WithProgressCallback`; describes `SetFundamentalDimension`; and shows the correct `MarginCallHandler` signature and discovery mechanism.

## [0.7.2] - 2026-04-18

### Added

- Strategies can query a specific fundamentals reporting period with `Engine.FetchFundamentalsByDateKey`, and can read the period behind any fundamental value via the new `FundamentalsDateKey` and `FundamentalsReportPeriod` metrics. Values are encoded as Unix seconds; round-trip with `time.Unix(int64(v), 0)`.
- Snapshots capture the reporting-period metadata and configured dimension instead of NULL/`"ARQ"` placeholders, so replays match live queries.

## [0.7.1] - 2026-04-15

### Fixed

- Long backtests no longer crash near the end with an index-out-of-range panic when risk-free rate data is unavailable for recent dates.

## [0.7.0] - 2026-04-14

### Changed

- Long-horizon backtests run significantly faster. Point-in-time data lookups now binary-search the cache directly instead of rebuilding a union time axis, and the data provider fetches only the database columns each strategy requests.

### Added

- All strategy binaries accept `--cpu-profile <path>` to write a Go CPU profile for the duration of the command.
- `pvbt backtest` now renders an interactive progress bar by default when run from a terminal, showing the current and final simulation dates, percent complete, ETA, and a running count of performance measurements evaluated. Pass `--no-progress` to disable it (e.g. for CI logs).
- Strategy authors can register a progress observer on the engine via `WithProgressCallback`, which fires after each backtest step with a `ProgressEvent` containing the step index, total steps, current and bounding dates, and cumulative measurement count.
- Strategies can use `universe.USTradable` as a daily-refreshed investable universe of liquid US stocks. Membership is sourced from pv-data and filters by market cap, dollar volume, price floor, and data completeness, mirroring the criteria of Quantopian's QTradableStocksUS. This is the recommended default universe for broad US equity strategies.
- The `asset.Asset` type carries metadata from the data provider: name, asset type, exchange, sector, industry, SIC code, CIK, and listing dates. Strategies can filter by these fields directly (e.g. exclude financial-sector stocks or limit to common stock).
- Strategies can configure the fundamental data dimension (ARQ, MRQ, ARY, MRY, ART, MRT) via `SetFundamentalDimension` in `Setup`. AR dimensions use SEC filing dates for point-in-time correctness; MR dimensions include restatements and are indexed to the fiscal period. Defaults to ARQ.
- Strategy authors can mark a parameter field as test-only with `testonly:"true"`. Test-only parameters are hidden from `pvbt describe`, the TUI, CLI flags, presets, and study sweeps, and `engine.ApplyParams` rejects any attempt to set them. Tests assign test-only fields directly on the strategy struct.

### Fixed

- `--preset` silently skipped strategy fields like `RiskOn` when they had no `pvbt` tag. It now applies them.
- The strategy guide and overview examples did not compile — they called `Universe.At(ctx, date, metrics...)`, which was removed in 0.6.0.
- The strategy guide wrongly documented `Mean`, `Sum`, `Variance`, and `Std` as reducing across assets. They reduce across time and preserve the asset axis.
- Fundamental metrics (revenue, working capital, etc.) are now forward-filled onto the daily time grid. Previously, `FetchAt` returned NaN when the simulation date did not exactly match a filing date, and `Fetch` returned sparse data with NaN gaps between quarterly filings.
- `FetchAt` for fundamental metrics now returns forward-filled values when the requested date falls on a weekend or holiday. Previously the response was empty, causing strategies that rebalance on a calendar date (e.g. March 31 for Q1 data) to silently skip the step in years where that date was not a trading day.

## [0.6.0] - 2026-04-06

### Added

- Schedules support `@daily`, `@quarterbegin`, and `@quarterend` directives for daily and quarterly trading schedules.
- Strategies can use `universe.SP500` and `universe.Nasdaq100` to trade against historical index membership sourced from pv-data. Index weight data is available via `Constituents()` on the index universe.
- Users can now trade live through Webull accounts.
- Users can now trade live through E*TRADE (Morgan Stanley) accounts.
- Users can now trade through TradeStation using OAuth 2.0 authentication, with support for all order types, all time-in-force durations, and native OCO/bracket order groups.
- Mean reversion signals: Z-Score, Hurst exponent (R/S and DFA variants), and pairs trading signals (PairsResidual, PairsRatio) for identifying stretched conditions and pair relationships (#27)

### Changed

- **Breaking:** `IndexProvider.IndexMembers` now returns `([]asset.Asset, []IndexConstituent, error)` instead of `([]asset.Asset, error)`. Implementors must return both the asset list and the constituent list with weights.
- **Breaking:** `Universe.Prefetch` has been removed. Data providers now pre-fetch internally.
- **Breaking:** `Universe.At` no longer accepts a date parameter; it always uses the current simulation date. Update strategy code from `u.At(ctx, date, metrics...)` to `u.At(ctx, metrics...)`.
- **Breaking:** `universe.SP500` and `universe.Nasdaq100` now use pv-data canonical names (`"sp500"`, `"ndx100"`) instead of `"SP500"` and `"NASDAQ100"`.
- **Breaking:** `Portfolio.Holdings` now returns `map[asset.Asset]float64` instead of taking a callback. Update strategy code from `port.Holdings(func(a asset.Asset, qty float64) { ... })` to `for a, qty := range port.Holdings() { ... }`.
- **Breaking:** `Portfolio` interface now includes a `View(start, end time.Time) Portfolio` method that returns a date-restricted view. Custom `Portfolio` implementations must add this method.
- **Breaking:** Optimization objectives are now specified with `portfolio.Rankable` values (e.g. `portfolio.Sharpe`) instead of the deleted `study.Metric` enum.
- Optimization reports show real equity curves for the top 10 parameter combinations.

### Fixed

- Database NULL values for metrics like EV/EBIT now appear as NaN instead of 0 in DataFrames.
- KFold cross-validation in-sample scores now correctly exclude the test fold, producing accurate overfitting diagnostics.
- Parameter sweeps with fractional step sizes no longer skip the final value due to floating-point drift.
- Backtests with rated universes run up to 17x faster because universe membership queries no longer scan the full ratings history on every step.
- Broker price data is now fetched once per batch instead of once per order, and the MarketImpact fill adjuster correctly receives volume data.
- Housekeeping data fetches (dividends, splits, margin prices, equity recording) are now batched into a single query per step instead of three separate queries.
- Snapshots no longer produce empty ratings tables; strategies using rated universes now run correctly against snapshot data.

## [0.5.0] - 2026-03-25

### Added

- Strategies can use five new built-in signals: RSI, MACD, Bollinger Bands, moving average crossover, and ATR.
- DataFrames support exponential moving averages via `Rolling(n).EMA()`.
- Strategy authors can configure fill models on the simulated broker for more realistic backtesting (VWAP, spread-aware, market impact, slippage), composable via `WithFillModel`.
- Users can configure risk management rules and tax optimization through a TOML config file (`pvbt.toml`) and `--risk-profile`/`--tax` CLI flags, without modifying strategy code.
- The `pvbt config` command displays the resolved middleware configuration after merging config file, profile defaults, and CLI flag overrides.
- Users can now trade through Tradier with support for market, limit, stop, and stop-limit orders, OCO and bracket groups, and real-time fill streaming.
- Users can now trade through Interactive Brokers using either OAuth or the Client Portal Gateway for authentication.
- The new `study optimize` command searches for the best strategy parameters using grid, random, or Bayesian search with out-of-sample validation. Validation schemes include simple train/test splits, k-fold cross-validation, walk-forward analysis, and scenario-based leave-N-out using the shared historical scenario library.
- Strategy parameter flags accept range syntax (`--lookback=3:24:1`) to define sweep ranges directly from the command line.
- Named historical scenarios are now available to all study types, not just stress tests.
- The `pvbt library` TUI shows strategy descriptions and GitHub README content rendered with styled markdown, and supports searching strategies by name or description.
- Strategies can be uninstalled from the library TUI with inline confirmation.
- Backtests automatically liquidate positions in delisted assets at the last known price instead of silently holding them with stale data.
- Strategies can identify overbought/oversold conditions with new Stochastic Oscillator (fast and slow), Williams %R, CCI, Keltner Channel, and Donchian Channel signals and adapt position sizing and stops to current market volatility.
- Volume signals (OBV, VWMA, Accumulation/Distribution, Chaikin Money Flow, and Money Flow Index) confirm price moves and detect accumulation/distribution patterns.

### Changed

- **Breaking:** Transaction type constants (`BuyTransaction`, `SellTransaction`, etc.) moved from the `portfolio` package to `asset`.
- The `pvbt discover`, `pvbt list`, and `pvbt remove` commands are replaced by `pvbt library`, with `list` and `remove` as subcommands.
- `broker.IsTransient` is renamed to `broker.IsRetryableError` and a new `broker.ErrRateLimited` sentinel error is available for all brokers.

### Fixed

- `DataFrame.Assets()` deduplicates when the same asset is passed more than once.
- TWRR now correctly eliminates the effect of deposits and withdrawals instead of counting them as investment returns.

## [0.4.0] - 2026-03-22

### Added

- Live trading now supports two new brokers: Alpaca and Charles Schwab.
- The new `study` command runs a strategy across multiple configurations and produces a combined analysis. Parameter sweeps vary lookback periods, universe composition, presets, or any other strategy parameter.
- The first built-in study is `study stress-test`, which evaluates a strategy against 17 named historical crises from the 1973 oil embargo through the 2023 regional banking crisis.
- The new `study monte-carlo` study tests whether a strategy's performance is skill or luck by comparing it against thousands of randomized alternatives.

### Changed

- Backtests run approximately 9x faster and use 14x less memory.
- **Breaking:** `NewDataFrame` accepts per-column slices instead of a single combined slice.

### Fixed

- Benchmark-relative metrics no longer crash when portfolio and benchmark return series contain NaN values at different positions.

## [0.3.0] - 2026-03-21

### Added

- The engine validates historical data coverage before a backtest begins, rejecting or adjusting runs when warmup data is insufficient.
- New portfolio weighting methods: inverse volatility, market capitalization, and risk parity.
- Strategies can compute Pearson correlation between asset pairs to inform diversification and pair-trading decisions.
- Weighting functions automatically fetch the additional data they need (e.g., market cap).
- The `describe` command displays strategy name, schedule, parameters, and presets; `--json` produces machine-readable output.
- The `--preset` flag selects a named parameter preset; the `--benchmark` flag sets the benchmark from the command line.
- Portfolio middleware intercepts orders between strategy and broker for risk management, slippage modeling, and tax optimization without changing strategy code.
- Built-in risk middleware enforces position size caps, drawdown circuit breakers, and position count limits. Pre-built profiles (`risk.Conservative`, `risk.Moderate`, `risk.Aggressive`) bundle common configurations.
- Tax optimization middleware harvests losses, tracks wash sales with IRS-compliant basis adjustment, and swaps to correlated substitutes. Lot selection is configurable via `WithDefaultLotSelection` or per-order via `WithLotSelection`.
- New tax drag metric shows how much of a strategy's return is lost to trading-related taxes.
- Strategy authors can compose multiple strategies into a single portfolio-of-strategies that allocates capital across them with configurable weights.
- Per-trade MFE and MAE tracking shows how much further each position could have run and how deep it went against you.
- Live trading via tastytrade with WebSocket fill streaming and sandbox mode for paper trading.
- Bracket and OCO order groups let strategies submit linked exit orders that activate or cancel automatically.
- Bracket and OCO orders fill within the same bar during backtests when high/low data shows the trigger price was reached.
- Short selling with proper cost basis tracking, margin accounting (Reg T defaults), borrow fees, and dividend obligations.
- Margin call handling via the optional `MarginCallHandler` interface, with automatic proportional liquidation as the default.
- Stock splits adjust position quantities and tax lot prices automatically.
- `RebalanceTo` accepts negative weights for short allocations; `GrossExposureLimit` and `NetExposureLimit` middleware bound overall leverage.
- Per-side trade metrics: `ShortWinRate`, `LongWinRate`, `ShortProfitFactor`, `LongProfitFactor`.

### Changed

- **Breaking:** `engine.DescribeStrategy` takes a `Strategy` instead of an `*Engine`.
- **Breaking:** Strategies declare their schedule in `Describe()` instead of calling `eng.Schedule()` during `Setup`.
- **Breaking:** Benchmark is set via the `--benchmark` flag instead of `eng.SetBenchmark()`.
- **Breaking:** `Strategy.Compute` receives a `*portfolio.Batch`; orders and annotations go to the batch instead of the portfolio directly.
- **Breaking:** `Portfolio` is read-only; `RebalanceTo`, `Order`, and `Annotate` move to `Batch`.
- **Breaking:** `PortfolioSnapshot` gains `TradeDetails()`, `Excursions()`, and `ShortLots()` methods.
- **Breaking:** `TradeDetail` and `TradeMetrics` gain new fields; positional struct literals will not compile.
- **Breaking:** `Record()` now creates short tax lots when a sell exceeds the long position.
- **Breaking:** Engine housekeeping order changed to fills-then-splits-then-dividends.
- **Breaking:** The simulated broker rejects short sells that violate initial margin requirements.
- **Breaking:** `universe.DataSource` becomes a type alias for `data.DataSource`.
- **Breaking:** `Annotation.Timestamp` changes from `int64` to `time.Time`.
- **Breaking:** Fills are delivered through a buffered channel (`Fills()`) instead of returned from `Submit`.

## [0.2.0] - 2026-03-17

### Added

- Rich terminal backtest report with performance chart, returns tables, risk metrics, drawdowns, and trade log.
- Ticker symbols provided via CLI flags are uppercased automatically.
- Benchmark targeting via the `.Benchmark()` query builder method.
- Portable SQLite snapshots capture backtest data access for reproducible offline testing.
- Index-based universes let strategies filter by index membership.
- New metrics: ForwardPE, PEG, PriceToCashFlow, Beta, and `RiskAdjustedPct(n)`.
- DataFrames from engine Fetch and FetchAt include the cumulative risk-free rate for computing excess returns.

### Changed

- DGS3MO (3-month Treasury yield) is now the system risk-free rate for all performance metrics.
- Annualization factor computed from actual observation frequency instead of hardcoding 252 or 12.
- Daily equity recorded on every trading day regardless of strategy schedule.
- Jensen's alpha computed from mean periodic excess returns instead of total cumulative returns.
- Treynor ratio annualized using CAGR instead of total returns.
- Withdrawal rates computed from actual return path instead of Monte Carlo bootstrap.

### Removed

- `engine.RiskFreeAsset()` -- the engine now uses DGS3MO automatically.

### Fixed

- Backtests skip non-trading days like Good Friday using market holidays from the database.
- `--start`, `--end`, and other CLI flags are honored on backtest, snapshot, and explore subcommands.
- `@monthend`, `@weekend`, and `@close` schedules fire at the actual close time on early-close days.
- `Months(N)` lookback snaps to month boundaries so monthly downsample yields exactly N rows.
- CLI flag overrides for strategy universe fields are applied instead of silently using defaults.

## [0.1.0] - 2026-03-14

### Added

- Backtest and live-trade strategies using the same code for both modes.
- Interactive terminal UI for exploring backtest results.
- 30+ built-in performance metrics covering risk-adjusted returns, drawdowns, capture ratios, trade round-trips, tax impact, and withdrawal sustainability.
- Built-in signals (momentum, volatility, earnings yield), asset selectors, weighting schemes, and universe filters.
- Preview upcoming trades before the next scheduled trade date.
- Annotate portfolio decisions with justifications for audit trails.
- Save and reload complete backtest results between sessions.

[unreleased]: https://github.com/penny-vault/pvbt/compare/v0.8.2...HEAD
[0.8.2]: https://github.com/penny-vault/pvbt/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/penny-vault/pvbt/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/penny-vault/pvbt/compare/v0.7.7...v0.8.0
[0.7.7]: https://github.com/penny-vault/pvbt/compare/v0.7.6...v0.7.7
[0.7.6]: https://github.com/penny-vault/pvbt/compare/v0.7.5...v0.7.6
[0.7.5]: https://github.com/penny-vault/pvbt/compare/v0.7.4...v0.7.5
[0.7.4]: https://github.com/penny-vault/pvbt/compare/v0.7.3...v0.7.4
[0.7.3]: https://github.com/penny-vault/pvbt/compare/v0.7.2...v0.7.3
[0.7.2]: https://github.com/penny-vault/pvbt/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/penny-vault/pvbt/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/penny-vault/pvbt/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/penny-vault/pvbt/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/penny-vault/pvbt/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/penny-vault/pvbt/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/penny-vault/pvbt/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/penny-vault/pvbt/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/penny-vault/pvbt/releases/tag/v0.1.0
