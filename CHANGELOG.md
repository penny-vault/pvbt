# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Users can now trade through E*TRADE (Morgan Stanley) using OAuth 1.0a authentication, with support for market, limit, stop, and stop-limit orders.

### Fixed

- Backtests with many holdings run significantly faster because broker price data is now fetched once per batch instead of once per order, and the MarketImpact fill adjuster correctly receives volume data.

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

[unreleased]: https://github.com/penny-vault/pvbt/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/penny-vault/pvbt/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/penny-vault/pvbt/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/penny-vault/pvbt/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/penny-vault/pvbt/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/penny-vault/pvbt/releases/tag/v0.1.0
