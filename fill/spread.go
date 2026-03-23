package fill

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// SpreadOption configures the SpreadAware adjuster.
type SpreadOption func(*spreadAdjuster)

// SpreadBPS sets a basis-point fallback for estimating the half-spread
// when real bid/ask data is not available in the bar.
func SpreadBPS(bps int) SpreadOption {
	return func(sa *spreadAdjuster) {
		sa.bps = bps
	}
}

// spreadAdjuster adjusts the fill price to account for the bid-ask spread.
type spreadAdjuster struct {
	bps int // fallback basis points; 0 means no fallback configured
}

// SpreadAware returns an Adjuster that accounts for bid-ask spread.
// If the bar contains real Bid and Ask values, buys fill at the ask and
// sells fill at the bid. Otherwise it falls back to the configured BPS
// estimate applied to current.Price. If neither source is available it
// returns an error.
func SpreadAware(opts ...SpreadOption) Adjuster {
	sa := &spreadAdjuster{}
	for _, opt := range opts {
		opt(sa)
	}

	return sa
}

func (sa *spreadAdjuster) Adjust(_ context.Context, order broker.Order, bar *data.DataFrame, current FillResult) (FillResult, error) {
	bidVal := bar.Value(order.Asset, data.Bid)
	askVal := bar.Value(order.Asset, data.Ask)

	hasBidAsk := !math.IsNaN(bidVal) && !math.IsNaN(askVal) && bidVal != 0 && askVal != 0

	if hasBidAsk {
		price := askVal
		if order.Side == broker.Sell {
			price = bidVal
		}

		return FillResult{
			Price:    price,
			Quantity: current.Quantity,
			Partial:  current.Partial,
		}, nil
	}

	if sa.bps > 0 {
		halfSpread := current.Price * float64(sa.bps) / 10000.0
		price := current.Price + halfSpread

		if order.Side == broker.Sell {
			price = current.Price - halfSpread
		}

		return FillResult{
			Price:    price,
			Quantity: current.Quantity,
			Partial:  current.Partial,
		}, nil
	}

	return FillResult{}, fmt.Errorf("spread adjuster: no bid/ask data and no BPS fallback configured")
}
