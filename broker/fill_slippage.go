package broker

import (
	"context"

	"github.com/penny-vault/pvbt/data"
)

// SlippageOption configures how a slippage adjuster computes the price adjustment.
type SlippageOption func(price float64) float64

// Percent returns a SlippageOption that applies a percentage slippage.
// For example, Percent(0.01) applies 1% slippage.
func Percent(pct float64) SlippageOption {
	return func(price float64) float64 {
		return price * pct
	}
}

// Fixed returns a SlippageOption that applies a fixed dollar amount of slippage.
func Fixed(amount float64) SlippageOption {
	return func(_ float64) float64 {
		return amount
	}
}

// slippageAdjuster applies slippage to the fill price based on order side.
type slippageAdjuster struct {
	option SlippageOption
}

// Slippage returns an Adjuster that applies slippage to the fill price.
// Buys are filled at a higher price; sells are filled at a lower price.
func Slippage(opt SlippageOption) Adjuster {
	return &slippageAdjuster{option: opt}
}

func (sa *slippageAdjuster) Adjust(_ context.Context, order Order, _ *data.DataFrame, current FillResult) (FillResult, error) {
	delta := sa.option(current.Price)

	adjusted := current.Price
	if order.Side == Buy {
		adjusted += delta
	} else {
		adjusted -= delta
	}

	return FillResult{
		Price:    adjusted,
		Quantity: current.Quantity,
		Partial:  current.Partial,
	}, nil
}
