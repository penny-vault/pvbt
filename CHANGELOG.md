# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Display rich backtest report with performance chart, trailing/annual/monthly returns, risk metrics, drawdowns, and trade log
- Target a benchmark for performance metrics via the `.Benchmark()` query builder method
- Compute MonthlyReturns, AnnualReturns, and DrawdownDetails from an account
- Render terminal report using lipgloss and ntcharts
- Capture backtest data access into portable SQLite snapshots for reproducible offline testing
- Support index-based universes so strategies can filter by index membership
- Add ForwardPE, PEG, PriceToCashFlow, and Beta metrics

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
