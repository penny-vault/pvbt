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

package risk

import (
	"context"
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

type volatilityScaler struct {
	dataSource DataSource
	lookback   int // trading days
}

// VolatilityScaler returns a middleware that scales position sizes inversely
// to trailing realized volatility. lookback is in trading days.
func VolatilityScaler(dataSource DataSource, lookback int) portfolio.Middleware {
	return &volatilityScaler{dataSource: dataSource, lookback: lookback}
}

func (vs *volatilityScaler) Process(ctx context.Context, batch *portfolio.Batch) error {
	projectedWeights := batch.ProjectedWeights()
	if len(projectedWeights) == 0 {
		return nil
	}

	totalValue := batch.ProjectedValue()
	if totalValue == 0 {
		return nil
	}

	// Collect the assets we need vol data for.
	assets := make([]asset.Asset, 0, len(projectedWeights))
	for ast := range projectedWeights {
		assets = append(assets, ast)
	}

	// Fetch close prices for the lookback period.
	lookbackPeriod := data.Days(vs.lookback)

	priceFrame, err := vs.dataSource.Fetch(ctx, assets, lookbackPeriod, []data.Metric{data.MetricClose})
	if err != nil {
		return fmt.Errorf("volatility scaler: fetch prices: %w", err)
	}

	// Compute annualized vol for each asset, split into with-vol and without-vol groups.
	type volEntry struct {
		asset          asset.Asset
		vol            float64
		originalWeight float64
	}

	var (
		withVol          []volEntry
		withoutVolWeight float64
	)

	for ast, weight := range projectedWeights {
		vol := computeAnnualizedVol(priceFrame, ast)
		if math.IsNaN(vol) || vol <= 0 {
			withoutVolWeight += math.Abs(weight)
			continue
		}

		withVol = append(withVol, volEntry{
			asset:          ast,
			vol:            vol,
			originalWeight: math.Abs(weight),
		})
	}

	if len(withVol) == 0 {
		return nil
	}

	// Compute the total original weight of assets with vol data.
	var groupOriginalWeight float64
	for _, entry := range withVol {
		groupOriginalWeight += entry.originalWeight
	}

	// Compute inverse-vol weights scaled to preserve the group's total weight.
	var invVolSum float64
	for _, entry := range withVol {
		invVolSum += 1.0 / entry.vol
	}

	// Build annotation details and inject sell orders for overweight positions.
	var annotationDetails string

	modified := false

	for _, entry := range withVol {
		invVolWeight := (1.0 / entry.vol) / invVolSum
		targetWeight := invVolWeight * groupOriginalWeight

		if targetWeight >= entry.originalWeight {
			// Target is equal or larger -- do not inject buys.
			continue
		}

		// Position is overweight relative to inverse-vol target; inject sell.
		excessWeight := entry.originalWeight - targetWeight
		excessDollars := excessWeight * totalValue

		batch.Orders = append(batch.Orders, broker.Order{
			Asset:       entry.asset,
			Side:        broker.Sell,
			Amount:      excessDollars,
			OrderType:   broker.Market,
			TimeInForce: broker.Day,
		})

		annotationDetails += fmt.Sprintf("%s: vol=%.1f%% weight %.1f%%->%.1f%%; ",
			entry.asset.Ticker, entry.vol*100, entry.originalWeight*100, targetWeight*100)

		modified = true
	}

	_ = withoutVolWeight

	if modified {
		batch.Annotate("risk:volatility-scaler", annotationDetails)
	}

	return nil
}

// computeAnnualizedVol computes annualized realized volatility from daily
// close prices: stddev(daily log returns) * sqrt(252).
// Returns NaN if insufficient data (need at least 2 prices).
func computeAnnualizedVol(priceFrame *data.DataFrame, ast asset.Asset) float64 {
	if priceFrame == nil {
		return math.NaN()
	}

	prices := priceFrame.Column(ast, data.MetricClose)
	if len(prices) < 2 {
		return math.NaN()
	}

	// Compute daily log returns.
	returns := make([]float64, len(prices)-1)

	for idx := 1; idx < len(prices); idx++ {
		if prices[idx-1] <= 0 || prices[idx] <= 0 {
			return math.NaN()
		}

		returns[idx-1] = math.Log(prices[idx] / prices[idx-1])
	}

	// Compute standard deviation of returns.
	meanReturn := 0.0

	for _, ret := range returns {
		meanReturn += ret
	}

	meanReturn /= float64(len(returns))

	variance := 0.0

	for _, ret := range returns {
		diff := ret - meanReturn
		variance += diff * diff
	}

	variance /= float64(len(returns) - 1) // sample variance

	stdDev := math.Sqrt(variance)

	// Annualize.
	return stdDev * math.Sqrt(252.0)
}
