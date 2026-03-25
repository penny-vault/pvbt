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

package risk_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/portfolio"
)

// mockDataSource is a test double for risk.DataSource that returns
// predetermined price DataFrames for each asset.
type mockDataSource struct {
	// pricesByAsset maps CompositeFigi to a slice of daily close prices.
	pricesByAsset map[string][]float64
	currentDate   time.Time
}

func (m *mockDataSource) Fetch(_ context.Context, assets []asset.Asset, _ data.Period, _ []data.Metric) (*data.DataFrame, error) {
	// Determine the longest price series to set the time axis length.
	maxLen := 0
	for _, ast := range assets {
		prices, ok := m.pricesByAsset[ast.CompositeFigi]
		if ok && len(prices) > maxLen {
			maxLen = len(prices)
		}
	}

	if maxLen == 0 {
		return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
	}

	// Build time axis.
	times := make([]time.Time, maxLen)
	baseDate := m.currentDate.AddDate(0, 0, -maxLen+1)
	for idx := range times {
		times[idx] = baseDate.AddDate(0, 0, idx)
	}

	// Build per-column slices: one column per (asset, metric) pair.
	metrics := []data.Metric{data.MetricClose}
	columns := make([][]float64, len(assets)*len(metrics))

	for assetIdx, ast := range assets {
		col := make([]float64, maxLen)
		prices, ok := m.pricesByAsset[ast.CompositeFigi]

		for timeIdx := 0; timeIdx < maxLen; timeIdx++ {
			if ok && timeIdx < len(prices) {
				col[timeIdx] = prices[timeIdx]
			} else {
				col[timeIdx] = math.NaN()
			}
		}

		columns[assetIdx] = col
	}

	return data.NewDataFrame(times, assets, metrics, data.Daily, columns)
}

func (m *mockDataSource) FetchAt(_ context.Context, _ []asset.Asset, _ time.Time, _ []data.Metric) (*data.DataFrame, error) {
	return data.NewDataFrame(nil, nil, nil, data.Daily, nil)
}

func (m *mockDataSource) CurrentDate() time.Time {
	return m.currentDate
}

var _ = Describe("VolatilityScaler", func() {
	var (
		ctx context.Context
		ts  time.Time

		// highVolAsset has daily returns with large swings => high vol.
		highVolAsset asset.Asset
		// lowVolAsset has daily returns with small swings => low vol.
		lowVolAsset asset.Asset
		// noDataAsset has no price data available.
		noDataAsset asset.Asset
	)

	BeforeEach(func() {
		ctx = context.Background()
		ts = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

		highVolAsset = asset.Asset{CompositeFigi: "HIGH001", Ticker: "HIGH"}
		lowVolAsset = asset.Asset{CompositeFigi: "LOW001", Ticker: "LOW"}
		noDataAsset = asset.Asset{CompositeFigi: "NODATA001", Ticker: "NODATA"}
	})

	// buildAccountWithPositions creates an Account holding the given assets
	// at the given prices and quantities, plus the specified cash balance.
	buildAccountWithPositions := func(cash float64, positions map[asset.Asset]struct {
		price float64
		qty   float64
	}) *portfolio.Account {
		totalCash := cash
		dfAssets := make([]asset.Asset, 0, len(positions))
		dfPrices := make([]float64, 0, len(positions))

		for ast, pos := range positions {
			totalCash += pos.price * pos.qty
			dfAssets = append(dfAssets, ast)
			dfPrices = append(dfPrices, pos.price)
		}

		acct := portfolio.New(portfolio.WithCash(totalCash, time.Time{}))

		if len(dfAssets) > 0 {
			priceCols := make([][]float64, len(dfPrices))
			for idx, price := range dfPrices {
				priceCols[idx] = []float64{price}
			}

			priceDF, err := data.NewDataFrame(
				[]time.Time{ts},
				dfAssets,
				[]data.Metric{data.MetricClose},
				data.Daily,
				priceCols,
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(priceDF)
		}

		for ast, pos := range positions {
			if pos.qty > 0 {
				acct.Record(portfolio.Transaction{
					Date:   ts,
					Asset:  ast,
					Type:   asset.BuyTransaction,
					Qty:    pos.qty,
					Price:  pos.price,
					Amount: -(pos.price * pos.qty),
				})
			}
		}

		return acct
	}

	// Generate price series: start at 100, apply daily returns.
	// High-vol: alternating +5% / -5% (annualized vol ~79%).
	// Low-vol: alternating +0.5% / -0.5% (annualized vol ~7.9%).
	highVolPrices := func() []float64 {
		prices := make([]float64, 21)
		prices[0] = 100.0
		for idx := 1; idx < len(prices); idx++ {
			if idx%2 == 1 {
				prices[idx] = prices[idx-1] * 1.05
			} else {
				prices[idx] = prices[idx-1] * 0.95
			}
		}
		return prices
	}()

	lowVolPrices := func() []float64 {
		prices := make([]float64, 21)
		prices[0] = 100.0
		for idx := 1; idx < len(prices); idx++ {
			if idx%2 == 1 {
				prices[idx] = prices[idx-1] * 1.005
			} else {
				prices[idx] = prices[idx-1] * 0.995
			}
		}
		return prices
	}()

	Describe("Process", func() {
		It("assigns smaller weight to higher-vol assets and larger weight to lower-vol assets", func() {
			positions := map[asset.Asset]struct {
				price float64
				qty   float64
			}{
				highVolAsset: {price: 100, qty: 50},
				lowVolAsset:  {price: 100, qty: 50},
			}

			acct := buildAccountWithPositions(0, positions)
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{
					highVolAsset.CompositeFigi: highVolPrices,
					lowVolAsset.CompositeFigi:  lowVolPrices,
				},
				currentDate: ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			// There should be a sell order for the high-vol asset (it should
			// be scaled down) and no buy order for the low-vol asset.
			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1), "expected exactly one sell order for the high-vol asset")
			Expect(sells[0].Asset).To(Equal(highVolAsset))
			Expect(sells[0].Amount).To(BeNumerically(">", 0))
		})

		It("keeps weights unchanged when vol data is unavailable for an asset", func() {
			positions := map[asset.Asset]struct {
				price float64
				qty   float64
			}{
				noDataAsset: {price: 100, qty: 50},
			}

			acct := buildAccountWithPositions(5000, positions)
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{},
				currentDate:   ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			// No orders should be injected since vol data is unavailable.
			Expect(batch.Orders).To(BeEmpty())
		})

		It("only injects sell orders, never buy orders", func() {
			positions := map[asset.Asset]struct {
				price float64
				qty   float64
			}{
				highVolAsset: {price: 100, qty: 50},
				lowVolAsset:  {price: 100, qty: 50},
			}

			acct := buildAccountWithPositions(0, positions)
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{
					highVolAsset.CompositeFigi: highVolPrices,
					lowVolAsset.CompositeFigi:  lowVolPrices,
				},
				currentDate: ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).To(BeEmpty(), "volatility scaler should never inject buy orders")
		})

		It("annotates the batch when scaling occurs", func() {
			positions := map[asset.Asset]struct {
				price float64
				qty   float64
			}{
				highVolAsset: {price: 100, qty: 50},
				lowVolAsset:  {price: 100, qty: 50},
			}

			acct := buildAccountWithPositions(0, positions)
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{
					highVolAsset.CompositeFigi: highVolPrices,
					lowVolAsset.CompositeFigi:  lowVolPrices,
				},
				currentDate: ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Annotations).To(HaveKey("risk:volatility-scaler"))
			annotation := batch.Annotations["risk:volatility-scaler"]
			Expect(annotation).To(ContainSubstring("vol="))
		})

		It("handles an empty portfolio gracefully", func() {
			acct := portfolio.New(portfolio.WithCash(10000, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{},
				currentDate:   ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:volatility-scaler"))
		})

		It("preserves weights of assets without vol data when others have vol data", func() {
			positions := map[asset.Asset]struct {
				price float64
				qty   float64
			}{
				highVolAsset: {price: 100, qty: 40},
				lowVolAsset:  {price: 100, qty: 40},
				noDataAsset:  {price: 100, qty: 20},
			}

			acct := buildAccountWithPositions(0, positions)
			batch := portfolio.NewBatch(ts, acct)

			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{
					highVolAsset.CompositeFigi: highVolPrices,
					lowVolAsset.CompositeFigi:  lowVolPrices,
					// noDataAsset intentionally omitted.
				},
				currentDate: ts,
			}

			mw := risk.VolatilityScaler(ds, 20)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			// The noDataAsset should not have any orders.
			for _, order := range batch.Orders {
				Expect(order.Asset).NotTo(Equal(noDataAsset),
					"no orders should be injected for assets without vol data")
			}
		})
	})
})
