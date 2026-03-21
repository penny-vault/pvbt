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

package data

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// ExampleData returns a BatchProvider and AssetProvider pre-populated with
// synthetic daily data for three assets (SPY, TLT, GLD) from January
// through June 2024. The data is deterministic and suitable for use in
// testable examples and documentation.
//
// The returned providers supply [MetricClose], [AdjClose], [Dividend],
// [MetricHigh], [MetricLow], and [SplitFactor] metrics. Dividend values
// are zero (no dividends in the synthetic data). High and Low are derived
// from Close by adding/subtracting 1% of the base price. SplitFactor is
// always 1.0 (no splits).
func ExampleData() (*TestProvider, AssetProvider) {
	spy := asset.Asset{CompositeFigi: "BBG000BDTBL9", Ticker: "SPY"}
	tlt := asset.Asset{CompositeFigi: "BBG000BKJMY2", Ticker: "TLT"}
	gld := asset.Asset{CompositeFigi: "BBG000CRF6Q8", Ticker: "GLD"}

	assets := []asset.Asset{spy, tlt, gld}
	metrics := []Metric{MetricClose, AdjClose, Dividend, MetricHigh, MetricLow, SplitFactor}

	// Generate trading days (skip weekends). Timestamps use US Eastern
	// at 16:00 (market close) to match tradecron schedule dates.
	nyc := eodLocation

	start := time.Date(2024, time.January, 2, 16, 0, 0, 0, nyc)
	end := time.Date(2024, time.June, 28, 16, 0, 0, 0, nyc)

	var times []time.Time

	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		wd := date.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			continue
		}

		times = append(times, date)
	}

	numTimes := len(times)
	numAssets := len(assets)
	numMetrics := len(metrics)

	cols := make([][]float64, numAssets*numMetrics)
	for i := range cols {
		cols[i] = make([]float64, numTimes)
	}

	// Deterministic price curves using simple sine-modulated trends.
	//   SPY: starts 450, trends up ~8% annualized
	//   TLT: starts 100, trends down ~3% annualized
	//   GLD: starts 180, trends up ~5% annualized
	basePrice := []float64{450.0, 100.0, 180.0}
	dailyDrift := []float64{0.08 / 252, -0.03 / 252, 0.05 / 252}
	amplitude := []float64{5.0, 2.0, 3.0}

	for aIdx := range numAssets {
		for timeIdx := range numTimes {
			wave := amplitude[aIdx] * math.Sin(2*math.Pi*float64(timeIdx)/63)
			price := basePrice[aIdx]*(1+dailyDrift[aIdx]*float64(timeIdx)) + wave
			price = math.Round(price*100) / 100

			cols[aIdx*numMetrics+0][timeIdx] = price                                              // MetricClose
			cols[aIdx*numMetrics+1][timeIdx] = price                                              // AdjClose
			cols[aIdx*numMetrics+2][timeIdx] = 0                                                  // Dividend
			cols[aIdx*numMetrics+3][timeIdx] = math.Round((price+basePrice[aIdx]*0.01)*100) / 100 // MetricHigh
			cols[aIdx*numMetrics+4][timeIdx] = math.Round((price-basePrice[aIdx]*0.01)*100) / 100 // MetricLow
			cols[aIdx*numMetrics+5][timeIdx] = 1.0                                                // SplitFactor
		}
	}

	frame, err := NewDataFrame(times, assets, metrics, Daily, cols)
	if err != nil {
		panic(fmt.Sprintf("ExampleData: %v", err))
	}

	provider := NewTestProvider(metrics, frame)
	assetProv := &exampleAssetProvider{assets: assets}

	return provider, assetProv
}

// exampleAssetProvider is a simple in-memory AssetProvider for use in
// examples and tests.
type exampleAssetProvider struct {
	assets []asset.Asset
}

func (p *exampleAssetProvider) Assets(_ context.Context) ([]asset.Asset, error) {
	return p.assets, nil
}

func (p *exampleAssetProvider) LookupAsset(_ context.Context, ticker string) (asset.Asset, error) {
	for _, a := range p.assets {
		if a.Ticker == ticker {
			return a, nil
		}
	}

	return asset.Asset{}, fmt.Errorf("unknown ticker: %s", ticker)
}
