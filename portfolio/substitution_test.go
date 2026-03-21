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

package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Substitution mapping", func() {
	var (
		spy  asset.Asset
		ivv  asset.Asset
		qqq  asset.Asset
		qqqe asset.Asset
		ts   time.Time
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		ivv = asset.Asset{CompositeFigi: "IVV001", Ticker: "IVV"}
		qqq = asset.Asset{CompositeFigi: "QQQ001", Ticker: "QQQ"}
		qqqe = asset.Asset{CompositeFigi: "QQQE001", Ticker: "QQQE"}
		ts = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	})

	// buildPricedAccountWithAssets creates an account with prices for the given assets.
	buildPricedAccountWithAssets := func(cash float64, assets []asset.Asset, prices []float64) *portfolio.Account {
		acct := portfolio.New(portfolio.WithCash(cash, time.Time{}))
		df, err := data.NewDataFrame(
			[]time.Time{ts},
			assets,
			[]data.Metric{data.MetricClose},
			data.Daily,
			prices,
		)
		Expect(err).NotTo(HaveOccurred())
		acct.UpdatePrices(df)

		return acct
	}

	Describe("RegisterSubstitution and ActiveSubstitutions", func() {
		It("registers a substitution and returns it via ActiveSubstitutions", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			expiry := ts.AddDate(0, 1, 0)

			acct.RegisterSubstitution(spy, ivv, expiry)

			subs := acct.ActiveSubstitutions()
			Expect(subs).To(HaveLen(1))
			Expect(subs).To(HaveKey(spy))
			Expect(subs[spy].Original).To(Equal(spy))
			Expect(subs[spy].Substitute).To(Equal(ivv))
			Expect(subs[spy].Until).To(Equal(expiry))
		})

		It("returns nil when no substitutions are registered", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			Expect(acct.ActiveSubstitutions()).To(BeNil())
		})

		It("returns a copy; mutations do not affect internal state", func() {
			acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)

			subs := acct.ActiveSubstitutions()
			delete(subs, spy)

			fresh := acct.ActiveSubstitutions()
			Expect(fresh).To(HaveLen(1))
		})
	})

	Describe("Holdings returns logical view", func() {
		It("maps substitute asset back to original when substitution is active", func() {
			// Hold IVV (the substitute) with an SPY->IVV substitution active.
			acct := buildPricedAccountWithAssets(90_000, []asset.Asset{spy, ivv}, []float64{450, 445})
			buyLot(acct, ivv, ts, 445.0, 10)

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)

			// Holdings callback should receive SPY, not IVV.
			found := make(map[string]float64)
			acct.Holdings(func(ast asset.Asset, qty float64) {
				found[ast.Ticker] = qty
			})

			Expect(found).To(HaveKey("SPY"))
			Expect(found).NotTo(HaveKey("IVV"))
			Expect(found["SPY"]).To(BeNumerically("~", 10.0, 0.001))
		})

		It("returns real asset after substitution expires", func() {
			// Hold IVV but the substitution has expired.
			acct := buildPricedAccountWithAssets(90_000, []asset.Asset{spy, ivv}, []float64{450, 445})
			buyLot(acct, ivv, ts, 445.0, 10)

			// Expiry is before the price date (ts), so substitution is expired.
			expired := ts.Add(-24 * time.Hour)
			acct.RegisterSubstitution(spy, ivv, expired)

			found := make(map[string]float64)
			acct.Holdings(func(ast asset.Asset, qty float64) {
				found[ast.Ticker] = qty
			})

			Expect(found).To(HaveKey("IVV"))
			Expect(found).NotTo(HaveKey("SPY"))
			Expect(found["IVV"]).To(BeNumerically("~", 10.0, 0.001))
		})
	})

	Describe("Value is unaffected by substitution mapping", func() {
		It("reports the same value regardless of active substitutions", func() {
			acct := buildPricedAccountWithAssets(90_000, []asset.Asset{spy, ivv}, []float64{450, 445})
			buyLot(acct, ivv, ts, 445.0, 10)
			// Re-update prices so value reflects current marks.
			df, err := data.NewDataFrame(
				[]time.Time{ts},
				[]asset.Asset{spy, ivv},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[]float64{450, 445},
			)
			Expect(err).NotTo(HaveOccurred())
			acct.UpdatePrices(df)

			valueBefore := acct.Value()

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)

			// Value uses the raw holdings * prices, unaffected by mapping.
			valueAfter := acct.Value()
			Expect(valueAfter).To(BeNumerically("~", valueBefore, 0.01))
		})
	})

	Describe("ProjectedHoldings maps substitute orders", func() {
		It("records buy order for substitute under the original asset", func() {
			acct := buildPricedAccountWithAssets(100_000, []asset.Asset{spy, ivv}, []float64{450, 445})

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)

			batch := portfolio.NewBatch(ts, acct)
			// Buy IVV (the substitute).
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       ivv,
				Side:        broker.Buy,
				Qty:         20,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			projected := batch.ProjectedHoldings()
			// Should appear under SPY (the original), not IVV.
			Expect(projected).To(HaveKey(spy))
			Expect(projected).NotTo(HaveKey(ivv))
			Expect(projected[spy]).To(BeNumerically("~", 20.0, 0.001))
		})
	})

	Describe("ProjectedWeights maps substitute assets", func() {
		It("returns weights keyed by the original asset", func() {
			acct := buildPricedAccountWithAssets(100_000, []asset.Asset{spy, ivv}, []float64{450, 445})

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)

			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       ivv,
				Side:        broker.Buy,
				Qty:         20,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			weights := batch.ProjectedWeights()
			Expect(weights).To(HaveKey(spy))
			Expect(weights).NotTo(HaveKey(ivv))
			Expect(weights[spy]).To(BeNumerically(">", 0.0))
		})
	})

	Describe("Multiple substitutions", func() {
		It("maps multiple substitutes to their logical originals", func() {
			acct := buildPricedAccountWithAssets(80_000,
				[]asset.Asset{spy, ivv, qqq, qqqe},
				[]float64{450, 445, 380, 375},
			)
			buyLot(acct, ivv, ts, 445.0, 10)
			buyLot(acct, qqqe, ts, 375.0, 5)

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)
			acct.RegisterSubstitution(qqq, qqqe, expiry)

			found := make(map[string]float64)
			acct.Holdings(func(ast asset.Asset, qty float64) {
				found[ast.Ticker] = qty
			})

			Expect(found).To(HaveKey("SPY"))
			Expect(found).To(HaveKey("QQQ"))
			Expect(found).NotTo(HaveKey("IVV"))
			Expect(found).NotTo(HaveKey("QQQE"))
			Expect(found["SPY"]).To(BeNumerically("~", 10.0, 0.001))
			Expect(found["QQQ"]).To(BeNumerically("~", 5.0, 0.001))
		})

		It("maps multiple substitutes in ProjectedHoldings", func() {
			acct := buildPricedAccountWithAssets(100_000,
				[]asset.Asset{spy, ivv, qqq, qqqe},
				[]float64{450, 445, 380, 375},
			)

			expiry := ts.AddDate(0, 1, 0)
			acct.RegisterSubstitution(spy, ivv, expiry)
			acct.RegisterSubstitution(qqq, qqqe, expiry)

			batch := portfolio.NewBatch(ts, acct)
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       ivv,
				Side:        broker.Buy,
				Qty:         10,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})
			batch.Orders = append(batch.Orders, broker.Order{
				Asset:       qqqe,
				Side:        broker.Buy,
				Qty:         5,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			})

			projected := batch.ProjectedHoldings()
			Expect(projected).To(HaveKey(spy))
			Expect(projected).To(HaveKey(qqq))
			Expect(projected).NotTo(HaveKey(ivv))
			Expect(projected).NotTo(HaveKey(qqqe))
			Expect(projected[spy]).To(BeNumerically("~", 10.0, 0.001))
			Expect(projected[qqq]).To(BeNumerically("~", 5.0, 0.001))
		})
	})
})
