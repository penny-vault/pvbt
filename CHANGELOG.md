# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.0] - 2023-02-12
### Removed
- SEEK strategy was removed because it requires proprietary data that is no longer easily available

## [0.6.0] - 2023-02-12
### Added
- Calculation of tax-adjusted returns

### Fixed
- Bug in data API that would cause dividends or splits to be duplicated on multiple runs
- Bug in application of corporate actions that caused dividends and splits to be applied in an inconsistent order
  this could result in unpredictable outcomes depending on the order that was used if a stock had a split and
  a dividend on the same day.
- Sortino ratio was showing as NA when it should have been computed

## [0.5.0] - 2022-12-19
### Changed
- Completely revamped the way data is handled internally. This will enable faster development of new features.

### Fixed
- Draw downs that are still on-going were not being included in performance metrics
- The way corporate actions were added had the possibility of creating duplicate transactions

## [0.4.0] - 2022-08-11
### Added
- Additional metrics in strategy list API; including: max draw down, downside risk, std. deviation, and more
- OpenAPI 3.0 documentation
- OpenTelemetry tracing
- New strategy: Keller's Protective Asset Allocation strategy (PAA)
- New strategy: Seeking Alpha SEEK algorithm
- New strategy: Momentum Driven Earnings Prediction algorithm

### Changed
- /v1/ api now returns the current time in it's message
- Upgraded all libraries
- Switch to faster JSON serializer/deserializer: goccy/go-json, in some cases
  it is up to 2x faster
- Portfolio metrics are now computed using daily values (was formerly monthly)
- Data provider is now PVDB not Tiingo (to support strategies that invest in a large
  number of securities)

### Removed
- Tiingo data provider (all data pulled from pvdb now)
- FRED data provider (all data pulled from pvdb now)

### Fixed
- When pvapi was running for a long time (>24 hrs) risk free rate data would become
  out-dated. Set a refresh timer every 24 hours to update this data
- Periodic refresh of JSON Web Key Set incase it is invalidated

## [0.3.1] - 2021-02-28
### Fixed
- Bug in portfolio metric calc that caused incorrect status to sometimes be sent
- Unlimit portfolio name length

## [0.3.0] - 2021-02-21
### Added
- Additional logging showing performance of strategy simulation at various points
- Transactions are now part of the portfolio.Performance struct
- Added a new strategy: Keller's Defensive Asset Allocation
- Added a section for suggested parameters in strategy definition
- Added justification to performance measurements

### Changed
- Switched logging provider from New Relic to Loki
- Database migrations are now in the 'database/migrations'

## [0.2.0] - 2021-02-14
### Added
- Calculation of a suite of portfolio metrics
- Benchmark endpoint that returns performance metrics for a benchmark ticker

## [0.1.1] - 2021-02-08
### Fixed
- Use US EST timezone for date selection in notifier
- Fixed ADM computation error when inputs were not in all uppercase
- Fixed issue with calculation of YTD Return when simulation end is not in current year

## [0.1.0] - 2021-02-08
### Added
- API CRUD functions for portfolios
- API functions for listing and executing investing strategies
- Strategy framework for executing investing strategies
- Stock data retrieval interface
- Acclerating Dual Momentum strategy

[0.7.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.7.0
[0.6.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.6.0
[0.5.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.5.0
[0.4.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.4.0
[0.3.2]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.2
[0.3.1]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.1
[0.3.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.0
[0.2.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.2.0
[0.1.1]: https://github.com/penny-vault/pv-api/releases/tag/v0.1.1
[0.1.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.1.0
