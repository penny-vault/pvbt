// Momentum Rotation is a simple strategy that rotates into the asset
// with the highest trailing return over a configurable lookback period.
// If no asset has positive momentum, it moves to a risk-off asset.
package main

import (
	"context"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/cli"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/penny-vault/pvbt/universe"
	"github.com/rs/zerolog"
)

type MomentumRotation struct {
	RiskOn   universe.Universe `pvbt:"risk-on"  desc:"Assets to rotate between" default:"SPY,EFA,EEM"`
	RiskOff  universe.Universe `pvbt:"risk-off" desc:"Safe-haven asset"         default:"SHY"`
	Lookback int               `pvbt:"lookback" desc:"Momentum lookback months"  default:"6"`
}

func (s *MomentumRotation) Name() string { return "momentum-rotation" }

func (s *MomentumRotation) Setup(e *engine.Engine) {
	tc, err := tradecron.New("@monthend", tradecron.MarketHours{Open: 930, Close: 1600})
	if err != nil {
		panic(err)
	}
	e.Schedule(tc)
	e.SetBenchmark(e.Asset("SPY"))
	e.RiskFreeAsset(e.Asset("SHV"))
}

func (s *MomentumRotation) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
	log := zerolog.Ctx(ctx)
	log.Debug().Msg("Compute called")

	// Fetch adjusted close prices for the lookback period.
	df, err := s.RiskOn.Window(ctx, portfolio.Months(s.Lookback), data.MetricClose)
	if err != nil {
		log.Error().Err(err).Msg("Window fetch failed")
		return
	}
	log.Debug().Int("len", df.Len()).
		Int("assets", len(df.AssetList())).
		Int("metrics", len(df.MetricList())).
		Msg("Window result")
	if df.Len() < 2 {
		log.Debug().Int("len", df.Len()).Msg("insufficient data")
		return
	}

	// Compute total return over the full window: (last / first) - 1.
	returns := df.Pct(df.Len() - 1)
	log.Debug().Int("returns_len", returns.Len()).
		Int("returns_assets", len(returns.AssetList())).
		Int("returns_metrics", len(returns.MetricList())).
		Msg("Pct result")

	// Find the asset with the highest return in the last row.
	lastRow := returns.Last()
	log.Debug().Int("lastRow_len", lastRow.Len()).
		Int("lastRow_assets", len(lastRow.AssetList())).
		Msg("Last result")
	assets := lastRow.AssetList()
	log.Debug().Int("assets", len(assets)).Msg("AssetList")
	if len(assets) == 0 {
		return
	}

	bestAsset := assets[0]
	bestReturn := math.Inf(-1)

	for _, a := range assets {
		v := lastRow.Value(a, data.MetricClose)
		log.Debug().Str("asset", a.Ticker).Float64("return", v).Msg("momentum")
		if !math.IsNaN(v) && v > bestReturn {
			bestReturn = v
			bestAsset = a
		}
	}

	log.Debug().Str("best", bestAsset.Ticker).Float64("return", bestReturn).Msg("selected")

	// If the best asset has positive momentum, go all-in; otherwise risk-off.
	var target asset.Asset
	if bestReturn > 0 {
		target = bestAsset
	} else {
		riskOffAssets := s.RiskOff.Assets(e.CurrentDate())
		if len(riskOffAssets) == 0 {
			return
		}
		target = riskOffAssets[0]
	}

	log.Debug().Str("target", target.Ticker).Msg("rebalancing to")
	if err := p.RebalanceTo(ctx, portfolio.Allocation{
		Date:    e.CurrentDate(),
		Members: map[asset.Asset]float64{target: 1.0},
	}); err != nil {
		log.Error().Err(err).Msg("rebalance failed")
	}
}

func (s *MomentumRotation) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode:   "momrot",
		Description: "Rotates into the asset with the highest trailing return.",
		Version:     "0.1.0",
	}
}

func main() {
	cli.Run(&MomentumRotation{})
}
