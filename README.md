# pvbt

[![CI](https://github.com/penny-vault/pvbt/actions/workflows/ci.yml/badge.svg)](https://github.com/penny-vault/pvbt/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25.6-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Every quantitative strategy begins as a simple idea -- buy momentum, hedge with bonds, rebalance monthly. Then the infrastructure arrives: data pipelines, date alignment, survivorship bias, split adjustments, slippage models. Before long the idea is buried under ten thousand lines of plumbing.

pvbt inverts that ratio. You write the thirty lines that express your thesis; the engine supplies the ten thousand underneath. The same strategy code backtests against two decades of history or trades live tomorrow.

## Highlights

- **No data plumbing.** Fetch through universes; the engine discovers your requirements, routes requests to providers, and caches results. You never write a loader.
- **Survivorship-bias-free universes.** Index membership resolves historically -- the S&P 500 on January 3, 2008 returns exactly the stocks in the index that day, not today's composition.
- **60+ performance metrics, including taxes.** Sharpe, Sortino, Calmar, drawdowns, and dozens more -- plus long- and short-term capital gains, qualified dividends, and safe withdrawal rates.
- **Market-aware scheduling.** Write `@monthend` instead of manual last-trading-day logic. Tradecron knows holidays, half-days, and market hours.
- **DataFrames that compose.** Chain `df.Pct(1).Rolling(20).Mean()` with automatic error propagation. Columns are contiguous `[]float64`, directly compatible with gonum.
- **One codebase, backtest to production.** The API never exposes whether you are in a simulation or trading live.

## Quick Example

Accelerating Dual Momentum:

```go
type ADM struct {
	RiskOn  universe.Universe `pvbt:"riskOn"  desc:"equity universe" default:"SPY,GLD,VWO"`
	RiskOff universe.Universe `pvbt:"riskOff" desc:"safe-haven"      default:"TLT"`
}

func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(_ *engine.Engine) {}

func (s *ADM) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		Schedule:  "@monthend",
		Benchmark: "SPY",
	}
}

func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, p portfolio.Portfolio, batch *portfolio.Batch) error {
	mom := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3), data.MetricClose)
	if err := mom.Err(); err != nil {
		return nil
	}

	riskOffDF, err := s.RiskOff.At(ctx, data.MetricClose)
	if err != nil {
		return nil
	}

	portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(mom)
	plan, err := portfolio.EqualWeight(mom)
	if err != nil {
		return nil
	}
	batch.RebalanceTo(ctx, plan...)
	return nil
}
```

The engine handles data loading, order execution, commission/slippage, and performance measurement.

## Installation

Requires Go 1.25.6 or later.

```sh
go get github.com/penny-vault/pvbt
```

## How It Works

A strategy implements three methods:

| Method | Purpose |
|--------|---------|
| `Name()` | Returns the strategy's short identifier |
| `Setup(eng *Engine)` | Optional initialization after fields are populated |
| `Compute(ctx, eng, p, batch)` | Runs at each scheduled step to make allocation decisions |

The engine runs in three phases:

1. **Setup** -- populates strategy fields from struct tags, registers universes, and builds a data loading plan.
2. **Computation** -- steps through time according to the schedule, calling `Compute` at each step.
3. **Results** -- the returned portfolio provides access to the transaction log and performance metrics over its full history.

## Documentation

- [Strategy Author's Guide](docs/strategy-guide.md) -- complete walkthrough from first strategy to testing
- [Overview](docs/overview.md) -- introduction and design principles
- [Engine](docs/engine.md) -- configuration, strategy interface, data access, trade preview
- [Universes](docs/universes.md) -- asset groups, index tracking, historical membership
- [Data](docs/data.md) -- metrics, data providers, DataFrames, signals
- [Portfolio](docs/portfolio.md) -- construction, order types, risk middleware, performance measurement
- [Performance Metrics](docs/performance-metrics.md) -- 60+ metrics reference, custom metrics, MFE/MAE
- [Broker](docs/broker.md) -- broker interface, tastytrade, Alpaca, and Schwab integrations, order groups
- [Scheduling](docs/scheduling.md) -- tradecron syntax and schedule configuration
- [Configuration](docs/configuration.md) -- struct tags, presets, and strategy parameterization

## Performance

Good strategy research means running backtests often -- tweaking a parameter, testing a variant, checking a hunch. The engine is designed to keep that loop tight so you spend your time thinking, not waiting. A 30-year backtest of Accelerating Dual Momentum finishes in about 4 seconds on an M-series MacBook.

- **Zero-copy DataFrame views.** Windowing, filtering by metric, and selecting assets share the underlying data instead of copying it, keeping chained transformations cheap over decades of daily history.
- **Shared metric computation.** The 85+ built-in metrics share intermediate results -- when 26 metrics all need the same windowed returns, they compute them once.
- **Low allocation pressure.** Per-column storage and view-based slicing minimize GC pauses, so performance stays consistent across long backtests.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
