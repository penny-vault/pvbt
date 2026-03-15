package engine_test

import (
	"context"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/signal"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
)

// BuyAndHold is a minimal strategy that buys SPY on the first step
// and holds it. It uses imperative ordering rather than the
// select-weight-rebalance pipeline.
type BuyAndHold struct {
	bought bool
}

func (s *BuyAndHold) Name() string { return "buy-and-hold" }

func (s *BuyAndHold) Setup(e *engine.Engine) {
	tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
	e.Schedule(tc)
	e.SetBenchmark(e.Asset("SPY"))
}

func (s *BuyAndHold) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) error {
	if s.bought {
		return nil
	}
	spy := e.Asset("SPY")
	p.Order(ctx, spy, portfolio.Buy, 20)
	s.bought = true
	return nil
}

// This example runs a buy-and-hold backtest with synthetic data from
// [data.ExampleData].
func Example_backtest() {
	dp, ap := data.ExampleData()

	eng := engine.New(&BuyAndHold{},
		engine.WithInitialDeposit(10_000),
		engine.WithDataProvider(dp),
		engine.WithAssetProvider(ap),
	)
	defer eng.Close()

	ctx := context.Background()
	start := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)

	p, err := eng.Backtest(ctx, start, end)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Final value: $%.2f\n", p.Value())
	// Output:
	// Final value: $10177.60
}

// MomentumStrategy picks the asset with the highest 3-month momentum
// each month, falling back to TLT when nothing has positive momentum.
// It demonstrates the select-weight-rebalance pipeline.
type MomentumStrategy struct {
	RiskOn  universe.Universe `pvbt:"riskOn"  desc:"equity universe" default:"SPY,GLD"`
	RiskOff universe.Universe `pvbt:"riskOff" desc:"safe-haven"      default:"TLT"`
}

func (s *MomentumStrategy) Name() string { return "momentum" }

func (s *MomentumStrategy) Setup(e *engine.Engine) {
	tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
	e.Schedule(tc)
	e.SetBenchmark(e.Asset("SPY"))
}

func (s *MomentumStrategy) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) error {
	mom := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3), data.MetricClose)
	if err := mom.Err(); err != nil {
		return nil
	}

	riskOffDF, err := s.RiskOff.At(ctx, e.CurrentDate(), data.MetricClose)
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

// This example runs a momentum rotation strategy with synthetic data.
func Example_momentum() {
	dp, ap := data.ExampleData()

	eng := engine.New(&MomentumStrategy{},
		engine.WithInitialDeposit(10_000),
		engine.WithDataProvider(dp),
		engine.WithAssetProvider(ap),
	)
	defer eng.Close()

	ctx := context.Background()
	start := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)

	p, err := eng.Backtest(ctx, start, end)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Final value: $%.2f\n", p.Value())
	// Output:
	// Final value: $9896.98
}
