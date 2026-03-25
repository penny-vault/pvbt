package broker

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// vwapFill fills orders using the volume-weighted average price.
// When a DataFetcher is available, it computes true VWAP from intraday bars.
// Otherwise it falls back to the typical price (High + Low + Close) / 3.
type vwapFill struct {
	fetcher DataFetcher
}

// VWAP returns a BaseModel that fills at the volume-weighted average price.
func VWAP() BaseModel {
	return &vwapFill{}
}

// SetDataFetcher injects a DataFetcher for intraday data access.
func (vf *vwapFill) SetDataFetcher(fetcher DataFetcher) {
	vf.fetcher = fetcher
}

func (vf *vwapFill) Fill(ctx context.Context, order Order, bar *data.DataFrame) (FillResult, error) {
	// Try true VWAP from intraday data when a fetcher is available.
	if vf.fetcher != nil {
		price, ok := vf.intradayVWAP(ctx, order, bar)
		if ok {
			return FillResult{
				Price:    price,
				Quantity: order.Qty,
			}, nil
		}
	}

	// Fall back to typical price from the daily bar.
	price, err := typicalPrice(order, bar)
	if err != nil {
		return FillResult{}, err
	}

	return FillResult{
		Price:    price,
		Quantity: order.Qty,
	}, nil
}

// intradayVWAP fetches intraday bars and computes sum(typicalPrice_i * volume_i) / sum(volume_i).
// Returns the VWAP and true on success, or 0 and false on failure.
func (vf *vwapFill) intradayVWAP(ctx context.Context, order Order, bar *data.DataFrame) (float64, bool) {
	times := bar.Times()
	if len(times) == 0 {
		return 0, false
	}

	barDate := times[len(times)-1]
	metrics := []data.Metric{data.MetricHigh, data.MetricLow, data.MetricClose, data.Volume}

	intradayDF, err := vf.fetcher.FetchAt(ctx, []asset.Asset{order.Asset}, barDate, metrics)
	if err != nil || intradayDF == nil {
		return 0, false
	}

	highCol := intradayDF.Column(order.Asset, data.MetricHigh)
	lowCol := intradayDF.Column(order.Asset, data.MetricLow)
	closeCol := intradayDF.Column(order.Asset, data.MetricClose)
	volCol := intradayDF.Column(order.Asset, data.Volume)

	if highCol == nil || lowCol == nil || closeCol == nil || volCol == nil {
		return 0, false
	}

	var sumPV, sumVol float64

	for ii := range highCol {
		tp := (highCol[ii] + lowCol[ii] + closeCol[ii]) / 3.0
		vol := volCol[ii]
		sumPV += tp * vol
		sumVol += vol
	}

	if sumVol == 0 {
		return 0, false
	}

	return sumPV / sumVol, true
}

// typicalPrice computes (High + Low + Close) / 3 from the daily bar.
func typicalPrice(order Order, bar *data.DataFrame) (float64, error) {
	high := bar.Value(order.Asset, data.MetricHigh)
	low := bar.Value(order.Asset, data.MetricLow)
	closePrice := bar.Value(order.Asset, data.MetricClose)

	if math.IsNaN(high) || math.IsNaN(low) || math.IsNaN(closePrice) {
		return 0, fmt.Errorf("vwap fill: missing OHLC data for %s", order.Asset.Ticker)
	}

	return (high + low + closePrice) / 3.0, nil
}
