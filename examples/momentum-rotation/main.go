// Momentum Rotation is a simple strategy that rotates into the asset
// with the highest trailing return over a configurable lookback period.
// If no asset has positive momentum, it moves to a risk-off asset.
package main

import (
	"context"

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

func (s *MomentumRotation) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("@monthend", tradecron.MarketHours{Open: 930, Close: 1600})
	if err != nil {
		panic(err)
	}

	eng.Schedule(tc)
	eng.SetBenchmark(eng.Asset("SPY"))
	eng.RiskFreeAsset(eng.Asset("SHV"))
}

func (s *MomentumRotation) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio) error {
	log := zerolog.Ctx(ctx)

	// Fetch close prices for the lookback period.
	df, err := s.RiskOn.Window(ctx, portfolio.Months(s.Lookback), data.MetricClose)
	if err != nil {
		log.Error().Err(err).Msg("Window fetch failed")
		return nil
	}

	if df.Len() < 2 {
		return nil
	}

	// Compute total return over the full window, take the last row.
	momentum := df.Pct(df.Len() - 1).Last()

	// Build a fallback DataFrame for risk-off assets at the current date.
	riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
	if err != nil {
		log.Error().Err(err).Msg("risk-off data fetch failed")
		return nil
	}

	// Select the asset with the highest positive return; fall back to risk-off.
	portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)

	plan, err := portfolio.EqualWeight(momentum)
	if err != nil {
		log.Error().Err(err).Msg("EqualWeight failed")
		return nil
	}

	if err := port.RebalanceTo(ctx, plan...); err != nil {
		log.Error().Err(err).Msg("rebalance failed")
	}

	return nil
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
