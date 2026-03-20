package portfolio_test

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// This example shows the declarative rebalancing workflow using a Batch:
// select assets, weight them, and add orders to the batch for execution.
func Example_rebalance() {
	// In a real strategy, batch comes from the engine's NewBatch call
	// and df comes from a signal computation on a universe.
	var batch *portfolio.Batch
	var df *data.DataFrame
	var riskOffDF *data.DataFrame

	ctx := context.Background()

	// Selection: pick the asset with the highest positive momentum,
	// falling back to the risk-off asset if nothing qualifies.
	portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(df)

	// Weighting: assign equal weights to selected assets.
	plan, err := portfolio.EqualWeight(df)
	if err != nil {
		fmt.Println(err)
		return
	}

	// RebalanceTo on the batch computes the orders needed to reach
	// the target allocation without executing them.
	if err := batch.RebalanceTo(ctx, plan...); err != nil {
		fmt.Println(err)
	}
}

// This example shows imperative order placement with modifiers on a Batch.
func Example_orderModifiers() {
	var batch *portfolio.Batch
	ctx := context.Background()
	spy := asset.Asset{Ticker: "SPY"}

	// Market order (default, no modifiers).
	batch.Order(ctx, spy, portfolio.Buy, 100)

	// Limit order: buy at $450 or lower.
	batch.Order(ctx, spy, portfolio.Buy, 100, portfolio.Limit(450.00))

	// Stop-limit order: trigger at $440, fill at $435 or better.
	batch.Order(ctx, spy, portfolio.Sell, 50,
		portfolio.Stop(440.00),
		portfolio.Limit(435.00),
	)

	// Good-til-cancelled limit order.
	batch.Order(ctx, spy, portfolio.Buy, 200,
		portfolio.Limit(420.00),
		portfolio.GoodTilCancel,
	)
}
