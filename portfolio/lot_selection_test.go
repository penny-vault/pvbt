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
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("LotSelection", func() {
	It("has four lot selection methods", func() {
		Expect(portfolio.LotFIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotLIFO).To(BeNumerically(">=", 0))
		Expect(portfolio.LotHighestCost).To(BeNumerically(">=", 0))
		Expect(portfolio.LotSpecificID).To(BeNumerically(">=", 0))
	})

	It("defaults to FIFO", func() {
		Expect(portfolio.LotFIFO).To(Equal(portfolio.LotSelection(0)))
	})
})

var _ = Describe("WithLotSelection modifier", func() {
	It("sets LotSelection on broker.Order via batch.Order()", func() {
		spy := asset.Asset{CompositeFigi: "SPY001", Ticker: "SPY"}
		timestamp := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

		acct := portfolio.New(portfolio.WithCash(100_000, time.Time{}))

		df := buildDF(timestamp, []asset.Asset{spy}, []float64{150.0}, []float64{150.0})
		acct.UpdatePrices(df)

		// Record a buy so we have something to sell.
		acct.Record(portfolio.Transaction{
			Date:   timestamp,
			Asset:  spy,
			Type:   asset.BuyTransaction,
			Qty:    10,
			Price:  150.0,
			Amount: -1500.0,
		})

		batch := portfolio.NewBatch(timestamp, acct)
		err := batch.Order(context.Background(), spy, portfolio.Sell, 5,
			portfolio.WithLotSelection(portfolio.LotHighestCost))
		Expect(err).ToNot(HaveOccurred())
		Expect(batch.Orders).To(HaveLen(1))
		Expect(batch.Orders[0].LotSelection).To(Equal(int(portfolio.LotHighestCost)))
	})
})
