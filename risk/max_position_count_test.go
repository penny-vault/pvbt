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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/risk"
)

var _ = Describe("MaxPositionCount", func() {
	var (
		spy asset.Asset
		qqq asset.Asset
		iwm asset.Asset
		ts  time.Time
		ctx context.Context
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		qqq = asset.Asset{CompositeFigi: "QQQ001", Ticker: "QQQ"}
		iwm = asset.Asset{CompositeFigi: "IWM001", Ticker: "IWM"}
		ts = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		ctx = context.Background()
	})

	// buildMultiAssetAccount creates an Account holding multiple assets.
	// assets and qtys must have the same length; prices provides per-asset prices
	// in the same order.
	buildMultiAssetAccount := func(cashExtra float64, assets []asset.Asset, prices []float64, qtys []float64) *portfolio.Account {
		totalPositionValue := 0.0
		for idx, qty := range qtys {
			totalPositionValue += prices[idx] * qty
		}

		acct := portfolio.New(portfolio.WithCash(cashExtra+totalPositionValue, time.Time{}))

		priceCols := make([][]float64, len(prices))
		for idx, price := range prices {
			priceCols[idx] = []float64{price}
		}

		df, err := data.NewDataFrame(
			[]time.Time{ts},
			assets,
			[]data.Metric{data.MetricClose},
			data.Daily,
			priceCols,
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		for idx, ast := range assets {
			if qtys[idx] > 0 {
				acct.Record(portfolio.Transaction{
					Date:   ts,
					Asset:  ast,
					Type:   asset.BuyTransaction,
					Qty:    qtys[idx],
					Price:  prices[idx],
					Amount: -(prices[idx] * qtys[idx]),
				})
			}
		}

		return acct
	}

	Describe("Process", func() {
		It("drops the smallest position when count exceeds limit", func() {
			// SPY: 50 shares @ $200 = $10000 (largest)
			// QQQ: 30 shares @ $100 = $3000
			// IWM: 10 shares @ $50  = $500  (smallest)
			// Cash: $0. Limit = 2 => IWM must be dropped.
			acct := buildMultiAssetAccount(0,
				[]asset.Asset{spy, qqq, iwm},
				[]float64{200, 100, 50},
				[]float64{50, 30, 10},
			)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(2)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(iwm))
			Expect(sells[0].Qty).To(BeNumerically("~", 10.0, 1e-9))
			Expect(sells[0].OrderType).To(Equal(broker.Market))
			Expect(sells[0].TimeInForce).To(Equal(broker.Day))
		})

		It("does not modify the batch when position count is within limit", func() {
			// 2 positions, limit = 2.
			acct := buildMultiAssetAccount(0,
				[]asset.Asset{spy, qqq},
				[]float64{200, 100},
				[]float64{50, 30},
			)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(2)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-count"))
		})

		It("annotates the batch when dropping a position", func() {
			// 3 positions, limit = 2. IWM ($500) is smallest and gets dropped.
			acct := buildMultiAssetAccount(0,
				[]asset.Asset{spy, qqq, iwm},
				[]float64{200, 100, 50},
				[]float64{50, 30, 10},
			)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(2)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Annotations).To(HaveKey("risk:max-position-count"))
			annotation := batch.Annotations["risk:max-position-count"]
			Expect(annotation).To(ContainSubstring("IWM"))
			Expect(annotation).To(ContainSubstring("2"))
		})

		It("handles an empty portfolio gracefully", func() {
			acct := portfolio.New(portfolio.WithCash(0, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(5)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-count"))
		})

		It("does not modify a batch when position count equals the limit exactly", func() {
			// 3 positions, limit = 3 => no action needed.
			acct := buildMultiAssetAccount(0,
				[]asset.Asset{spy, qqq, iwm},
				[]float64{200, 100, 50},
				[]float64{50, 30, 10},
			)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(3)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-count"))
		})

		It("drops multiple positions when count exceeds limit by more than one", func() {
			// 3 positions, limit = 1 => drop IWM ($500) and QQQ ($3000); keep SPY ($10000).
			acct := buildMultiAssetAccount(0,
				[]asset.Asset{spy, qqq, iwm},
				[]float64{200, 100, 50},
				[]float64{50, 30, 10},
			)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionCount(1)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(2))

			// Both IWM and QQQ should appear in sell orders.
			soldTickers := make(map[string]bool)
			for _, sell := range sells {
				soldTickers[sell.Asset.Ticker] = true
			}
			Expect(soldTickers).To(HaveKey("IWM"))
			Expect(soldTickers).To(HaveKey("QQQ"))
			Expect(soldTickers).NotTo(HaveKey("SPY"))
		})
	})
})
