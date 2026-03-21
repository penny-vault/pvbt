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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("$CASH filtering in RebalanceTo", func() {
	var (
		spyAsset  asset.Asset
		cashAsset asset.Asset
		tradeDate time.Time
		fillTime  time.Time
	)

	BeforeEach(func() {
		spyAsset = asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		cashAsset = asset.Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}
		tradeDate = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		fillTime = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	})

	Describe("Account.RebalanceTo", func() {
		It("skips $CASH entries and generates orders only for real assets", func() {
			df, err := data.NewDataFrame(
				[]time.Time{tradeDate},
				[]asset.Asset{spyAsset},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{400.0}},
			)
			Expect(err).NotTo(HaveOccurred())

			// SPY at 0.60 weight means floor(0.60 * 10000 / 400) = 15 shares.
			mb := newMockBroker()
			mb.fillsByAsset = map[asset.Asset][]broker.Fill{
				spyAsset: {{Price: 400.0, Qty: 15, FilledAt: fillTime}},
			}

			acct := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBroker(mb),
			)
			acct.UpdatePrices(df)

			err = acct.RebalanceTo(context.Background(), portfolio.Allocation{
				Date: tradeDate,
				Members: map[asset.Asset]float64{
					spyAsset:  0.60,
					cashAsset: 0.40,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// An order must have been submitted for SPY only.
			Expect(mb.submitted).NotTo(BeEmpty())
			for _, order := range mb.submitted {
				Expect(order.Asset.Ticker).NotTo(Equal("$CASH"))
			}

			// There must be a SPY buy transaction.
			spyTxns := filterTransactionsByAsset(acct.Transactions(), spyAsset)
			Expect(spyTxns).NotTo(BeEmpty())

			// No transaction should reference $CASH.
			cashTxns := filterTransactionsByAsset(acct.Transactions(), cashAsset)
			Expect(cashTxns).To(BeEmpty())
		})
	})

	Describe("Batch.RebalanceTo", func() {
		It("skips $CASH entries and accumulates orders only for real assets", func() {
			df, err := data.NewDataFrame(
				[]time.Time{tradeDate},
				[]asset.Asset{spyAsset},
				[]data.Metric{data.MetricClose},
				data.Daily,
				[][]float64{{400.0}},
			)
			Expect(err).NotTo(HaveOccurred())

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.UpdatePrices(df)

			batch := portfolio.NewBatch(tradeDate, acct)

			err = batch.RebalanceTo(context.Background(), portfolio.Allocation{
				Date: tradeDate,
				Members: map[asset.Asset]float64{
					spyAsset:  0.60,
					cashAsset: 0.40,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// There must be at least one order.
			Expect(batch.Orders).NotTo(BeEmpty())

			// No order should reference $CASH.
			for _, order := range batch.Orders {
				Expect(order.Asset.Ticker).NotTo(Equal("$CASH"))
			}

			// At least one order should be for SPY.
			spyOrders := filterOrdersByAsset(batch.Orders, spyAsset)
			Expect(spyOrders).NotTo(BeEmpty())
		})
	})
})

// filterTransactionsByAsset returns the subset of txns that match the given asset.
func filterTransactionsByAsset(txns []portfolio.Transaction, target asset.Asset) []portfolio.Transaction {
	var result []portfolio.Transaction
	for _, txn := range txns {
		if txn.Asset == target {
			result = append(result, txn)
		}
	}
	return result
}

// filterOrdersByAsset returns the subset of orders that match the given asset.
func filterOrdersByAsset(orders []broker.Order, target asset.Asset) []broker.Order {
	var result []broker.Order
	for _, order := range orders {
		if order.Asset == target {
			result = append(result, order)
		}
	}
	return result
}
