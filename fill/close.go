package fill

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// closeFill fills orders at the bar's close price.
type closeFill struct{}

// Close returns a BaseModel that fills at the close price.
func Close() BaseModel {
	return &closeFill{}
}

func (cf *closeFill) Fill(_ context.Context, order broker.Order, bar *data.DataFrame) (FillResult, error) {
	price := bar.Value(order.Asset, data.MetricClose)
	if math.IsNaN(price) || price == 0 {
		return FillResult{}, fmt.Errorf("close fill: no close price for %s", order.Asset.Ticker)
	}

	return FillResult{
		Price:    price,
		Quantity: order.Qty,
	}, nil
}
