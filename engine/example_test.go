package engine_test

import (
	"context"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
)

// MyStrategy demonstrates the Strategy interface. A strategy needs
// three methods: Name, Setup, and Compute.
type MyStrategy struct {
	Stocks universe.Universe `pvbt:"stocks" desc:"equity universe" default:"SPY,QQQ"`
}

func (s *MyStrategy) Name() string { return "example" }

func (s *MyStrategy) Setup(e *engine.Engine) {
	tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
	e.Schedule(tc)
}

func (s *MyStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {
	// Strategy logic goes here: compute signals, select assets,
	// build a portfolio plan, and rebalance.
}

// This example shows how to define a strategy and create an engine.
// In practice, you would register data and asset providers via
// WithDataProvider and WithAssetProvider.
func Example() {
	eng := engine.New(&MyStrategy{},
		engine.WithInitialDeposit(10_000),
	)
	defer eng.Close()

	_ = eng // call eng.Backtest(ctx, start, end) with real providers
}
