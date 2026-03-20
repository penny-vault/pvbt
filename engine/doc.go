// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package engine is the main entry point for pvbt, a backtesting engine
// library. It handles infrastructure -- data fetching, order management,
// fill simulation, and performance metrics -- so you can focus on strategy
// logic.
//
// To write a new strategy you bring two things:
//
//   - A Go struct that implements the [Strategy] interface
//   - A main function that creates the engine and runs the backtest
//
// Parameters are defined as exported struct fields with struct tags. No
// external configuration files are needed.
//
// # Strategy Interface
//
// The [Strategy] interface requires three methods:
//
//   - Name returns a short identifier for the strategy (e.g. "adm").
//   - Setup runs once after the engine populates strategy fields from their
//     default struct tags. It sets the trading schedule, benchmark, risk-free
//     asset, and performs any other one-time initialization.
//   - Compute runs at each scheduled step. It receives a context, the engine
//     (for data fetching), and the portfolio (current holdings). The strategy
//     computes signals, selects assets, and tells the portfolio to rebalance.
//
// # Example
//
// The following example implements Accelerating Dual Momentum (ADM). It
// computes 1-, 3-, and 6-month momentum on a set of risk-on assets, averages
// the scores, and invests in the highest-scoring asset if it is above zero.
// Otherwise it moves to a risk-off asset.
//
//	package main
//
//	import (
//		"context"
//		"time"
//
//		"github.com/penny-vault/pvbt/engine"
//		"github.com/penny-vault/pvbt/portfolio"
//		"github.com/penny-vault/pvbt/signal"
//		"github.com/penny-vault/pvbt/tradecron"
//		"github.com/penny-vault/pvbt/universe"
//		"github.com/rs/zerolog/log"
//	)
//
//	type ADM struct {
//		RiskOn  universe.Universe `pvbt:"riskOn"  desc:"ETFs to invest in" default:"VOO,SCZ"`
//		RiskOff universe.Universe `pvbt:"riskOff" desc:"Out-of-market asset" default:"TLT"`
//	}
//
//	func (s *ADM) Name() string { return "adm" }
//
//	func (s *ADM) Setup(e *engine.Engine) {
//		tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
//		e.Schedule(tc)
//		e.SetBenchmark(e.Asset("VFINX"))
//		// The engine automatically uses DGS3MO (3-Month Treasury yield) as the
//		// risk-free rate for all performance metrics.
//	}
//
//	func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
//		mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
//		mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
//		mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))
//
//		// Average the three momentum scores across all risk-on assets.
//		momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
//		if err := momentum.Err(); err != nil {
//			log.Error().Err(err).Msg("signal computation failed")
//			return err
//		}
//
//		// Pick the risk-on asset with the highest positive momentum.
//		// If none are positive, fall back to the risk-off asset (TLT).
//		riskOffDF, err := s.RiskOff.At(ctx, eng.CurrentDate(), data.MetricClose)
//		if err != nil {
//			log.Error().Err(err).Msg("risk-off data fetch failed")
//			return err
//		}
//		portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)
//
//		// Build an equal-weight plan and rebalance into it.
//		plan, err := portfolio.EqualWeight(momentum)
//		if err != nil {
//			log.Error().Err(err).Msg("EqualWeight failed")
//			return err
//		}
//		portfolio.RebalanceTo(ctx, plan...)
//		return nil
//	}
//
//	func main() {
//		eng := engine.New(&ADM{},
//			engine.WithInitialDeposit(10_000),
//			engine.WithDataProvider(provider),
//			engine.WithAssetProvider(provider),
//		)
//		defer eng.Close()
//
//		ctx := context.Background()
//		start := time.Date(2005, time.January, 1, 0, 0, 0, 0, time.UTC)
//		end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
//
//		p, err := eng.Backtest(ctx, start, end)
//	}
//
// # Execution Model
//
// A backtest proceeds in four phases.
//
// Phase 1: Initialization. The engine loads the asset registry from the
// AssetProvider, then uses reflection to populate exported strategy fields
// from their default struct tags. It builds a routing table mapping each
// data metric to its provider. Then it calls Setup, where the strategy sets
// the schedule, benchmark, risk-free asset, and does any other one-time
// initialization. The engine creates a portfolio account from the initial
// deposit (or snapshot, or pre-configured account), attaches a simulated
// broker, and initializes the per-column data cache.
//
// Phase 2: Date enumeration. The engine walks the tradecron schedule from
// start to end, collecting every trading date. For ADM with @monthend, that
// is roughly 240 dates from 2005 to 2024.
//
// Phase 3: Step loop. At each date the engine:
//
//  1. Fetches housekeeping data (close, adjusted close, dividends) for held
//     assets, the benchmark, and the risk-free asset.
//  2. Records dividend income for held positions.
//  3. Updates the simulated broker with the current price provider and date
//     so orders can fill.
//  4. Calls Compute. The strategy fetches data, computes signals, and tells
//     the portfolio to rebalance.
//  5. Fetches post-Compute prices for all held assets (including newly
//     acquired positions) and updates the equity curve.
//  6. Computes all registered performance metrics across standard windows
//     (5yr, 3yr, 1yr, YTD, MTD, WTD, and since-inception).
//  7. Evicts stale cache entries.
//
// Phase 4: Return. After the final step, the portfolio contains the full
// transaction log and can compute performance metrics. It provides access to
// the equity curve, every trade via Transactions, individual metrics via
// PerformanceMetric, and convenient bundles like Summary and RiskMetrics.
//
// # Previewing Upcoming Trades
//
// [Engine.PredictedPortfolio] previews what trades a strategy would make on
// the next scheduled trade date using currently available data. This is useful
// for strategies that trade infrequently (e.g., monthly) where users want to
// see what trades are coming before the actual trade date.
//
// The method clones the current portfolio, advances the engine's date to the
// next scheduled trade date, fills any data gaps by copying the last available
// prices forward day-by-day, and runs the strategy's Compute against the
// shadow copy. The strategy is completely unaware it is a
// prediction run.
//
// Call it after a backtest or during live operation:
//
//	predicted, err := eng.PredictedPortfolio(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, tx := range predicted.Transactions() {
//	    fmt.Printf("%s %s %.0f shares\n", tx.Type, tx.Asset.Ticker, tx.Qty)
//	}
//
// The returned portfolio includes annotations, justifications, and all
// other portfolio state produced by the prediction run.
//
// # Configuration
//
// Strategy parameters are defined as exported struct fields with struct tags.
// The engine populates them via reflection before calling Setup.
//
// Four tags control how a field is exposed:
//
//   - pvbt: CLI flag name. Defaults to the lowercase field name if omitted.
//   - desc: Description for help text.
//   - default: Default value, parsed from a string representation.
//   - suggest: Named presets, pipe-delimited as name=value pairs.
//
// Supported field types:
//
//   - float64: decimal number (e.g. default:"0.05")
//   - int: integer (e.g. default:"12")
//   - string: plain text (e.g. default:"momentum")
//   - bool: true or false (e.g. default:"true")
//   - time.Duration: Go duration string (e.g. default:"720h")
//   - [asset.Asset]: ticker symbol (e.g. default:"SPY"), resolved via Engine.Asset
//   - [universe.Universe]: comma-separated tickers (e.g. default:"VOO,SCZ"),
//     resolved and wrapped in a StaticUniverse via Engine.Universe
//
// Hydration runs before Setup. The engine reflects over the strategy struct
// and processes each exported field with a default tag. If the field is
// already non-zero (set by the caller or CLI flags), it is not overwritten.
// Otherwise the default tag value is parsed into the field's type. For
// [asset.Asset] fields the ticker is resolved via Engine.Asset. For
// [universe.Universe] fields the comma-separated tickers are resolved and
// wrapped in a StaticUniverse.
//
// The CLI uses the pvbt and desc tags to register cobra flags automatically.
// When a user passes --riskOn "SPY,QQQ", the field is populated before
// hydration runs, so the default tag is skipped.
//
// # Metadata
//
// Strategies can optionally implement the [Descriptor] interface to provide
// additional metadata such as a shortcode, description, source URL, version,
// schedule, and benchmark via the [StrategyDescription] struct. When Schedule
// and Benchmark are declared in Describe(), the engine reads them during
// initialization and the strategy does not need to call [Engine.Schedule] or
// [Engine.SetBenchmark] in Setup. The imperative methods still work and
// override values from Describe().
//
// [DescribeStrategy] takes a [Strategy] (not an engine) and produces a
// [StrategyInfo] struct that serializes to JSON. It collects name and
// description from the strategy, schedule and benchmark from Describe(),
// parameters from struct tags, and suggestions grouped by preset name.
// It does not require an engine or Setup to have run.
//
// # Logging
//
// Strategies use zerolog for structured logging. The logger is carried on
// the context passed to Compute. Use zerolog.Ctx(ctx) to retrieve it:
//
//	func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
//		log := zerolog.Ctx(ctx)
//		log.Info().Str("strategy", s.Name()).Msg("computing")
//		return nil
//	}
//
// The engine attaches a pre-configured logger to the context before calling
// Compute. Strategies should use zerolog.Ctx(ctx) rather than creating their
// own logger. This ensures log output is consistent and correctly scoped to
// the current computation step.
//
// # Design Principles
//
// Two principles shaped the API.
//
// Strategies should read like their plain-English descriptions. Accelerating
// Dual Momentum computes 1-, 3-, and 6-month momentum on a set of risk-on
// assets, averages the scores, and invests in the highest-scoring asset if
// it is above zero -- otherwise it moves to a risk-off asset. The code says
// exactly that, in roughly the same number of words.
//
// The same code should work in a backtest and in production. A strategy that
// runs against 20 years of historical data should deploy to a live trading
// system without modification. The API never exposes whether you are in a
// simulation or operating in real time.
package engine
