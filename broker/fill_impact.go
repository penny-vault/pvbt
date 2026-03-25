package broker

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/data"
)

// ImpactPreset bundles coefficient and volume threshold for market impact modeling.
type ImpactPreset struct {
	Coefficient      float64
	PartialThreshold float64 // fraction of daily volume above which fill is partial
}

var (
	// LargeCap models market impact for large-cap, liquid stocks.
	LargeCap = ImpactPreset{Coefficient: 0.1, PartialThreshold: 0.05}

	// SmallCap models market impact for small-cap stocks with moderate liquidity.
	SmallCap = ImpactPreset{Coefficient: 0.3, PartialThreshold: 0.02}

	// MicroCap models market impact for micro-cap stocks with thin liquidity.
	MicroCap = ImpactPreset{Coefficient: 0.5, PartialThreshold: 0.01}
)

// marketImpactAdjuster adjusts fill price and quantity based on modeled market impact.
type marketImpactAdjuster struct {
	preset ImpactPreset
}

// MarketImpact returns an Adjuster that models market impact as a function
// of order participation in daily volume. Large orders relative to volume
// receive worse fill prices and may be partially filled.
func MarketImpact(preset ImpactPreset) Adjuster {
	return &marketImpactAdjuster{preset: preset}
}

func (mi *marketImpactAdjuster) Adjust(_ context.Context, order Order, bar *data.DataFrame, current FillResult) (FillResult, error) {
	volume := bar.Value(order.Asset, data.Volume)
	if math.IsNaN(volume) || volume <= 0 {
		return FillResult{}, fmt.Errorf("market impact adjuster: volume data is unavailable or zero for %s", order.Asset.Ticker)
	}

	qty := current.Quantity
	partial := current.Partial

	participation := qty / volume
	if participation > mi.preset.PartialThreshold {
		qty = math.Floor(volume * mi.preset.PartialThreshold)
		partial = true
		participation = qty / volume
	}

	impact := mi.preset.Coefficient * math.Sqrt(participation)

	adjustedPrice := current.Price * (1.0 + impact)
	if order.Side == Sell {
		adjustedPrice = current.Price * (1.0 - impact)
	}

	return FillResult{
		Price:    adjustedPrice,
		Quantity: qty,
		Partial:  partial,
	}, nil
}
