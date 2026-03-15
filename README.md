# pvbt

pvbt is Penny Vault's backtesting engine. It lets you write investment strategies that read like their plain-English descriptions, then run them against 20 years of history or deploy them to production -- same code, no changes.

## Highlights

- **Strategy code reads like prose.** Express ideas in terms of metrics, DataFrames, and portfolio operations -- not loops, arrays, and index math.
- **Backtest and production use the same code.** The API never exposes whether you're in a simulation or trading live.
- **Optimized data loading.** The engine discovers data requirements and batches requests across providers automatically.
- **DataFrame-centric.** All time-series operations go through DataFrame -- column-major storage, gonum-compatible, SIMD-friendly.
- **Rich performance measurement.** Sharpe, Sortino, Calmar, Alpha, Beta, drawdowns, tax impact, withdrawal rates, and dozens more.
- **Survivorship-bias-free universes.** Index-based universes like the S&P 500 resolve historically, so you never accidentally trade a stock that didn't exist yet.

## Quick Example

Accelerating Dual Momentum:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
)

type ADM struct {
	RiskOn  universe.Universe `pvbt:"riskOn"  desc:"equity universe" default:"SPY,GLD,VWO"`
	RiskOff universe.Universe `pvbt:"riskOff" desc:"safe-haven"      default:"TLT"`
}

func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(eng *engine.Engine) {
	tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
	eng.Schedule(tc)
	eng.RiskFreeAsset(eng.Asset("DGS3MO"))
	eng.SetBenchmark(eng.Asset("SPY"))
}

func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, p portfolio.Portfolio) error {
	mom := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3), data.MetricClose)
	if err := mom.Err(); err != nil {
		return nil
	}

	riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
	if err != nil {
		return nil
	}

	portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(mom)
	plan, err := portfolio.EqualWeight(mom)
	if err != nil {
		return nil
	}
	p.RebalanceTo(ctx, plan...)
	return nil
}

func main() {
	eng := engine.New(&ADM{},
		engine.WithInitialDeposit(10_000),
	)
	defer eng.Close()

	ctx := context.Background()
	start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

	result, err := eng.Backtest(ctx, start, end)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Final value: $%.2f\n", result.Value())
}
```

The engine handles data loading, order execution, commission/slippage, and performance measurement.

## Installation

```sh
go get github.com/penny-vault/pvbt
```

Requires Go 1.25.6 or later.

## How It Works

A strategy implements three methods:

| Method | Purpose |
|--------|---------|
| `Name()` | Returns the strategy's short identifier |
| `Setup(eng *Engine)` | Optional initialization after fields are populated |
| `Compute(ctx context.Context, eng *Engine, p portfolio.Portfolio) error` | Runs at each scheduled step to make allocation decisions |

The engine runs in three phases:

1. **Setup** -- populates strategy fields from TOML, registers universes, and builds a data loading plan.
2. **Computation** -- steps through time according to the schedule, calling `Compute` at each step.
3. **Results** -- the returned portfolio provides access to the transaction log and performance metrics over its full history.

## Key Concepts

**Metrics** are externally-sourced measurements -- Price, Volume, MarketCap, EarningsPerShare, Unemployment, etc. Data providers supply metrics; the engine routes requests to the right provider.

**DataFrames** are the primary type for working with time-series data. They store values indexed by time, asset, and metric with operations for filtering, arithmetic, transforms, rolling windows, and more. Columns are contiguous `[]float64` slices, directly compatible with gonum.

**Signals** are computations derived from metrics -- momentum, risk-adjusted returns, moving average crossovers. They receive a DataFrame and return computed values.

**Universes** define the investable space -- from explicit ticker lists to historically-accurate index membership.

**Portfolios** turn allocation decisions into trades. Use `RebalanceTo` for declarative allocation or `Order` for individual trades. Risk controls are configured by the operator, not the strategy author. Attach a broker for live execution or leave it off for backtesting -- the strategy code is the same either way.

**Scheduling** uses tradecron, a cron dialect that understands trading calendars, market holidays, and trading hours.

**Configuration** is defined in a TOML file alongside the strategy code, making strategies user-configurable without touching Go.

**Predicted Portfolio** shows what a strategy would buy on the next trade date. Call `eng.PredictedPortfolio(ctx)` after a backtest or during live operation to preview upcoming trades without affecting the real portfolio.

## Documentation

Detailed documentation for each concept:

- [Overview](docs/overview.md) -- full walkthrough of the example strategy
- [Universes](docs/universes.md) -- asset groups, index tracking, historical membership
- [Data](docs/data.md) -- metrics, data providers, DataFrames, signals
- [Portfolio](docs/portfolio.md) -- construction, order types, performance measurement
- [Broker](docs/broker.md) -- broker interface for live trading
- [Scheduling](docs/scheduling.md) -- tradecron syntax and schedule configuration
- [Configuration](docs/configuration.md) -- TOML file format and strategy parameterization

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
