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
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("DrawdownCircuitBreaker", func() {
	var (
		spy asset.Asset
		ctx context.Context
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		ctx = context.Background()
	})

	// buildEquityCurveAccount creates an Account whose equity curve follows
	// the given values. Each value corresponds to one UpdatePrices call on a
	// successive weekday. Equity changes are modelled as deposits/withdrawals
	// so the total portfolio value matches the supplied values exactly.
	buildEquityCurveAccount := func(equityValues []float64) *portfolio.Account {
		start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
		dates := make([]time.Time, 0, len(equityValues))
		day := start

		for len(dates) < len(equityValues) {
			if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
				dates = append(dates, day)
			}

			day = day.AddDate(0, 0, 1)
		}

		acct := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))

		for idx, val := range equityValues {
			if idx > 0 {
				diff := val - equityValues[idx-1]

				if diff > 0 {
					acct.Record(portfolio.Transaction{
						Date:   dates[idx],
						Type:   asset.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					acct.Record(portfolio.Transaction{
						Date:   dates[idx],
						Type:   asset.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}

			df, err := data.NewDataFrame(
				[]time.Time{dates[idx]},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{100}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)
		}

		return acct
	}

	// buildAccountWithHoldings creates an Account that holds qty shares of
	// spy at price, with the equity curve matching equityValues.
	buildAccountWithHoldings := func(equityValues []float64, price, qty float64) *portfolio.Account {
		start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		dates := make([]time.Time, 0, len(equityValues))
		day := start

		for len(dates) < len(equityValues) {
			if wd := day.Weekday(); wd != time.Saturday && wd != time.Sunday {
				dates = append(dates, day)
			}

			day = day.AddDate(0, 0, 1)
		}

		cash := equityValues[0] - price*qty
		acct := portfolio.New(portfolio.WithCash(cash, time.Time{}))
		acct.Record(portfolio.Transaction{
			Date:   dates[0],
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    qty,
			Price:  price,
			Amount: -(price * qty),
		})

		for idx, val := range equityValues {
			if idx > 0 {
				prevEquity := equityValues[idx-1]
				diff := val - prevEquity

				if diff > 0 {
					acct.Record(portfolio.Transaction{
						Date:   dates[idx],
						Type:   asset.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					acct.Record(portfolio.Transaction{
						Date:   dates[idx],
						Type:   asset.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}

			df, err := data.NewDataFrame(
				[]time.Time{dates[idx]},
				[]asset.Asset{spy},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{price}},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)
		}

		return acct
	}

	Describe("Process", func() {
		It("force-liquidates all positions when drawdown exceeds the threshold", func() {
			// Equity curve: peak at 10000, then drops to 8000 = 20% drawdown.
			// Threshold = 0.15 (15%), so the circuit breaker fires.
			// Account holds 50 shares of SPY at $100/share.
			acct := buildAccountWithHoldings(
				[]float64{10_000, 10_000, 10_000, 8_000},
				100, 50,
			)

			batch := portfolio.NewBatch(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), acct)

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(HaveLen(1))
			Expect(sells[0].Asset).To(Equal(spy))
			Expect(sells[0].Qty).To(BeNumerically("~", 50.0, 1e-9))
			Expect(sells[0].Side).To(Equal(broker.Sell))
			Expect(sells[0].OrderType).To(Equal(broker.Market))
			Expect(sells[0].TimeInForce).To(Equal(broker.Day))
		})

		It("removes buy orders from the batch when the circuit breaker fires", func() {
			// 20% drawdown, threshold 15% -- circuit breaker fires.
			acct := buildAccountWithHoldings(
				[]float64{10_000, 10_000, 10_000, 8_000},
				100, 50,
			)

			batch := portfolio.NewBatch(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), acct)
			// Pre-populate a buy order that the middleware must remove.
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       spy,
				Side:        broker.Buy,
				Amount:      1_000,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			buys := ordersWithSide(batch.Orders, broker.Buy)
			Expect(buys).To(BeEmpty())
		})

		It("annotates the batch when the circuit breaker fires", func() {
			acct := buildAccountWithHoldings(
				[]float64{10_000, 10_000, 10_000, 8_000},
				100, 50,
			)

			batch := portfolio.NewBatch(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), acct)

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Annotations).To(HaveKey("risk:drawdown-circuit-breaker"))
			Expect(batch.Annotations["risk:drawdown-circuit-breaker"]).To(ContainSubstring("threshold"))
		})

		It("does not modify the batch when drawdown is within the threshold", func() {
			// Equity curve: peak 10000, drop to 9500 = 5% drawdown.
			// Threshold = 0.15, so no action.
			acct := buildAccountWithHoldings(
				[]float64{10_000, 10_000, 10_000, 9_500},
				100, 50,
			)

			batch := portfolio.NewBatch(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), acct)

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:drawdown-circuit-breaker"))
		})

		It("takes no action early in the backtest when there is no performance data", func() {
			// Account with no UpdatePrices calls => PerfData is nil =>
			// MaxDrawdown returns 0, which is within any positive threshold.
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			batch := portfolio.NewBatch(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), acct)

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			Expect(batch.Orders).To(BeEmpty())
			Expect(batch.Annotations).NotTo(HaveKey("risk:drawdown-circuit-breaker"))
		})

		It("handles a portfolio with no holdings gracefully when drawdown exceeds threshold", func() {
			// Cash-only portfolio with 20% drawdown -- no positions to liquidate.
			acct := buildEquityCurveAccount([]float64{10_000, 10_000, 10_000, 8_000})

			batch := portfolio.NewBatch(time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), acct)

			mw := risk.DrawdownCircuitBreaker(0.15)
			Expect(mw.Process(ctx, batch)).To(Succeed())

			sells := ordersWithSide(batch.Orders, broker.Sell)
			Expect(sells).To(BeEmpty())
			// Annotation is still added even if no positions to sell.
			Expect(batch.Annotations).To(HaveKey("risk:drawdown-circuit-breaker"))
		})
	})
})
