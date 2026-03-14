package portfolio_test

import (
	"context"
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// This example shows the declarative rebalancing workflow:
// select assets, weight them, and rebalance the portfolio.
func Example_rebalance() {
	// In a real strategy, p comes from the Compute method argument
	// and df comes from a signal computation on a universe.
	var p portfolio.Portfolio
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

	// Rebalance: the portfolio diffs current holdings against the
	// target and generates the necessary trades.
	if err := p.RebalanceTo(ctx, plan...); err != nil {
		fmt.Println(err)
	}
}

// This example shows imperative order placement with modifiers.
func Example_orderModifiers() {
	var p portfolio.Portfolio
	ctx := context.Background()
	spy := asset.Asset{Ticker: "SPY"}

	// Market order (default, no modifiers).
	p.Order(ctx, spy, portfolio.Buy, 100)

	// Limit order: buy at $450 or lower.
	p.Order(ctx, spy, portfolio.Buy, 100, portfolio.Limit(450.00))

	// Stop-limit order: trigger at $440, fill at $435 or better.
	p.Order(ctx, spy, portfolio.Sell, 50,
		portfolio.Stop(440.00),
		portfolio.Limit(435.00),
	)

	// Good-til-cancelled limit order.
	p.Order(ctx, spy, portfolio.Buy, 200,
		portfolio.Limit(420.00),
		portfolio.GoodTilCancel,
	)
}
