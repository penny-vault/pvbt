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

package engine_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// mockAssetProvider implements data.AssetProvider for tests.
type mockAssetProvider struct {
	assets []asset.Asset
}

func (m *mockAssetProvider) Assets(_ context.Context) ([]asset.Asset, error) {
	return m.assets, nil
}

func (m *mockAssetProvider) LookupAsset(_ context.Context, ticker string) (asset.Asset, error) {
	for _, a := range m.assets {
		if a.Ticker == ticker {
			return a, nil
		}
	}
	return asset.Asset{}, fmt.Errorf("not found: %s", ticker)
}

// backtestStrategy is a simple equal-weight strategy used in integration tests.
type backtestStrategy struct {
	assets []asset.Asset
}

func (s *backtestStrategy) Name() string { return "backtestStrategy" }

func (s *backtestStrategy) Setup(e *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("backtestStrategy.Setup: tradecron.New: %v", err))
	}
	e.Schedule(tc)
}

func (s *backtestStrategy) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
	if len(s.assets) == 0 {
		return
	}
	// Fetch current prices for the strategy's assets.
	priceDF, err := e.FetchAt(ctx, s.assets, e.CurrentDate(), []data.Metric{data.MetricClose})
	if err != nil || priceDF == nil {
		return
	}

	weight := 1.0 / float64(len(s.assets))
	totalValue := p.Cash()
	// Include current holdings value via positions.
	p.Holdings(func(a asset.Asset, qty float64) {
		v := priceDF.ValueAt(a, data.MetricClose, e.CurrentDate())
		if !math.IsNaN(v) {
			totalValue += qty * v
		}
	})

	// Sell assets not in target.
	p.Holdings(func(a asset.Asset, qty float64) {
		inTarget := false
		for _, ta := range s.assets {
			if ta == a {
				inTarget = true
				break
			}
		}
		if !inTarget && qty > 0 {
			p.Order(a, portfolio.Sell, qty)
		}
	})

	// Buy/adjust target assets.
	for _, a := range s.assets {
		v := priceDF.ValueAt(a, data.MetricClose, e.CurrentDate())
		if math.IsNaN(v) || v <= 0 {
			continue
		}
		targetShares := math.Floor(weight * totalValue / v)
		currentShares := p.Position(a)
		diff := targetShares - currentShares
		if diff > 0 {
			p.Order(a, portfolio.Buy, diff)
		} else if diff < 0 {
			p.Order(a, portfolio.Sell, -diff)
		}
	}
}

// noScheduleStrategy omits calling e.Schedule in Setup.
type noScheduleStrategy struct{}

func (s *noScheduleStrategy) Name() string                                                  { return "noSchedule" }
func (s *noScheduleStrategy) Setup(_ *engine.Engine)                                        {}
func (s *noScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {
}

// makeDailyTestData creates a DataFrame with daily prices for the given assets
// and metrics, covering nDays starting at start. Timestamps are set to 16:00
// UTC to match tradecron "0 16 * * 1-5" schedule dates. Values are sequential
// floats starting from 100.0, incrementing by 1.0 each row.
func makeDailyTestData(t *testing.T, start time.Time, nDays int, assets []asset.Asset, metrics []data.Metric) *data.DataFrame {
	t.Helper()
	times := make([]time.Time, nDays)
	for i := range times {
		d := start.AddDate(0, 0, i)
		times[i] = time.Date(d.Year(), d.Month(), d.Day(), 16, 0, 0, 0, time.UTC)
	}
	vals := make([]float64, nDays*len(assets)*len(metrics))
	for i := range vals {
		vals[i] = 100.0 + float64(i)
	}
	df, err := data.NewDataFrame(times, assets, metrics, vals)
	if err != nil {
		t.Fatalf("makeDailyTestData: %v", err)
	}
	return df
}

// TestBacktestEndToEnd verifies a complete backtest run with two assets over
// approximately one month.
func TestBacktestEndToEnd(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	msft := asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
	assets := []asset.Asset{aapl, msft}

	// Build 400 days of data so the provider covers the full date range.
	dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	df := makeDailyTestData(t, dataStart, 400, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	assetProvider := &mockAssetProvider{assets: assets}
	strategy := &backtestStrategy{assets: assets}

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(assetProvider),
	)

	acct := portfolio.New(portfolio.WithCash(100_000.0))

	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := eng.Backtest(ctx, acct, start, end)
	if err != nil {
		t.Fatalf("Backtest returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Backtest returned nil account")
	}

	// There should be buy transactions after the first rebalance.
	txns := result.Transactions()
	hasBuy := false
	for _, tx := range txns {
		if tx.Type == portfolio.BuyTransaction {
			hasBuy = true
			break
		}
	}
	if !hasBuy {
		t.Error("expected buy transactions after rebalance, found none")
	}

	// Equity curve should have entries.
	curve := result.EquityCurve()
	if len(curve) == 0 {
		t.Error("expected equity curve entries, got none")
	}
}

// TestBacktestNoSchedule verifies that Backtest returns an error when the
// strategy does not set a schedule during Setup.
func TestBacktestNoSchedule(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	assetProvider := &mockAssetProvider{assets: []asset.Asset{aapl}}
	strategy := &noScheduleStrategy{}

	eng := engine.New(strategy, engine.WithAssetProvider(assetProvider))
	acct := portfolio.New(portfolio.WithCash(100_000.0))

	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

	_, err := eng.Backtest(context.Background(), acct, start, end)
	if err == nil {
		t.Fatal("expected error for missing schedule, got nil")
	}
}

// TestBacktestNoAssetProvider verifies that Backtest returns an error when
// no asset provider is configured.
func TestBacktestNoAssetProvider(t *testing.T) {
	strategy := &noScheduleStrategy{}
	eng := engine.New(strategy) // no WithAssetProvider

	acct := portfolio.New(portfolio.WithCash(100_000.0))

	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

	_, err := eng.Backtest(context.Background(), acct, start, end)
	if err == nil {
		t.Fatal("expected error for missing asset provider, got nil")
	}
}
