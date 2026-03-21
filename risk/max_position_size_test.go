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

var _ = Describe("MaxPositionSize", func() {
	var (
		spy asset.Asset
		ts  time.Time
		ctx context.Context
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		ts = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		ctx = context.Background()
	})

	// buildAccount creates an Account with the given cash and a single-asset
	// price DataFrame, then records a buy transaction so the asset is held.
	buildAccount := func(cash float64, ast asset.Asset, price float64, qty float64) *portfolio.Account {
		acct := portfolio.New(portfolio.WithCash(cash+price*qty, time.Time{}))

		df, err := data.NewDataFrame(
			[]time.Time{ts},
			[]asset.Asset{ast},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{price}},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		if qty > 0 {
			acct.Record(portfolio.Transaction{
				Date:   ts,
				Asset:  ast,
				Type:   portfolio.BuyTransaction,
				Qty:    qty,
				Price:  price,
				Amount: -(price * qty),
			})
		}

		return acct
	}

	// buildShortAccount creates an Account with the given cash and a short
	// position. The sell transaction creates a short lot (no existing long).
	buildShortAccount := func(cash float64, ast asset.Asset, price float64, shortQty float64) *portfolio.Account {
		// Cash is initial cash plus proceeds from short sale.
		acct := portfolio.New(portfolio.WithCash(cash+price*shortQty, time.Time{}))

		acct.Record(portfolio.Transaction{
			Date:   ts,
			Asset:  ast,
			Type:   portfolio.SellTransaction,
			Qty:    shortQty,
			Price:  price,
			Amount: 0, // cash already accounted for
		})

		df, err := data.NewDataFrame(
			[]time.Time{ts},
			[]asset.Asset{ast},
			[]data.Metric{data.MetricClose},
			data.Daily,
			[][]float64{{price}},
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		return acct
	}

	Describe("Process", func() {
		It("injects a sell order to cap a position that exceeds the limit", func() {
			// 80 shares of SPY at $100 = $8000 position.
			// Cash = $2000, total = $10000. SPY weight = 0.80.
			// Limit = 0.50 => excess = 0.30 * 10000 = $3000 must be sold.
			acct := buildAccount(2_000, spy, 100, 80)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(spy))
			Expect(sells[0].Amount).To(BeNumerically("~", 3_000.0, 1e-6))
			Expect(sells[0].OrderType).To(Equal(broker.Market))
			Expect(sells[0].TimeInForce).To(Equal(broker.Day))
		})

		It("projected weight is at or below limit after the injected sell", func() {
			// 80 shares SPY @ $100 = $8000 (80% of $10000). Limit = 0.60.
			acct := buildAccount(2_000, spy, 100, 80)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.60)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			// After running the middleware the batch now contains the sell order.
			// Recompute projected weights.
			weights := batch.ProjectedWeights()
			Expect(weights[spy]).To(BeNumerically("<=", 0.60+1e-9))
		})

		It("does not modify a batch when all positions are within the limit", func() {
			// 40 shares SPY @ $100 = $4000 (40% of $10000). Limit = 0.50.
			acct := buildAccount(6_000, spy, 100, 40)
			batch := portfolio.NewBatch(ts, acct)

			originalOrderCount := len(batch.Orders)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(HaveLen(originalOrderCount))
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-size"))
		})

		It("annotates the batch when capping a position", func() {
			// 80 shares SPY @ $100 = $8000 (80%). Limit = 0.50.
			acct := buildAccount(2_000, spy, 100, 80)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Annotations).To(HaveKey("risk:max-position-size"))
			Expect(batch.Annotations["risk:max-position-size"]).To(ContainSubstring("SPY"))
			Expect(batch.Annotations["risk:max-position-size"]).To(ContainSubstring("50.0%"))
		})

		It("handles an empty batch gracefully", func() {
			acct := portfolio.New(portfolio.WithCash(0, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-size"))
		})

		It("handles a cash-only portfolio with no holdings", func() {
			// Account has $10000 cash and no positions.
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.30)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})

		It("does not modify a position exactly at the limit", func() {
			// 50 shares SPY @ $100 = $5000 (50% of $10000). Limit = 0.50.
			acct := buildAccount(5_000, spy, 100, 50)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:max-position-size"))
		})

		It("injects a buy order to cap a short position that exceeds the limit", func() {
			// Short 80 shares SPY @ $100 = -$8000 position.
			// Cash = $10000 (initial) + $8000 (short proceeds) = $18000.
			// Equity = cash + LMV - SMV = 18000 + 0 - 8000 = 10000.
			// Short weight = -8000 / 10000 = -0.80, abs = 0.80.
			// Limit = 0.50 => excess = 0.30 * 10000 = $3000 must be covered.
			acct := buildShortAccount(10_000, spy, 100, 80)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).To(HaveLen(1))
			Expect(buys[0].Asset).To(Equal(spy))
			Expect(buys[0].Amount).To(BeNumerically("~", 3_000.0, 1e-6))
			Expect(buys[0].OrderType).To(Equal(broker.Market))
		})

		It("does not restrict a sell that closes a long position", func() {
			// 40 shares SPY @ $100 = $4000 (40% of $10000). Limit = 0.50.
			// A sell of 20 shares reduces position to 20% -- should not be blocked.
			acct := buildAccount(6_000, spy, 100, 40)
			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Sell,
				Amount:      2_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			// The original sell should remain, and no additional orders should be added.
			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Amount).To(BeNumerically("~", 2_000.0, 1e-6))
		})

		It("annotates the batch when capping a short position", func() {
			acct := buildShortAccount(10_000, spy, 100, 80)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Annotations).To(HaveKey("risk:max-position-size"))
			Expect(batch.Annotations["risk:max-position-size"]).To(ContainSubstring("short"))
			Expect(batch.Annotations["risk:max-position-size"]).To(ContainSubstring("SPY"))
		})

		It("does not modify a short position within the limit", func() {
			// Short 30 shares SPY @ $100 = -$3000. Cash = $13000.
			// Equity = 13000 - 3000 = 10000. Weight = -30%, abs = 30%.
			acct := buildShortAccount(10_000, spy, 100, 30)
			batch := portfolio.NewBatch(ts, acct)

			mw := risk.MaxPositionSize(0.50)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
		})
	})
})

// ordersWithSide filters a slice of broker orders by side.
func ordersWithSide(orders []broker.Order, side broker.Side) []broker.Order {
	var result []broker.Order

	for _, ord := range orders {
		if ord.Side == side {
			result = append(result, ord)
		}
	}

	return result
}
