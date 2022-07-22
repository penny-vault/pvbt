# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Additional metrics in strategy list API; including: max draw down, downside risk, std. deviation, and more
- OpenAPI 3.0 documentation
- Keller's Protective Asset Allocation strategy (PAA)
- OpenTelemetry tracing
- Seeking Alpha SEEK algorithm
- MDEP algorithm

### Changed
- /v1/ api now returns the current time in it's message
- Upgraded all libraries
- Switch to faster JSON serializer/deserializer: goccy/go-json, in some cases
  it is up to 2x faster
- Portfolio metrics are now computed using daily values (was formerly monthly)
- Default data provider is now PVDB not Tiingo (to support strategies that invest in a large
  number of securities)

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

[0.3.2]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.2
[0.3.1]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.1
[0.3.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.3.0
[0.2.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.2.0
[0.1.1]: https://github.com/penny-vault/pv-api/releases/tag/v0.1.1
[0.1.0]: https://github.com/penny-vault/pv-api/releases/tag/v0.1.0
