# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
