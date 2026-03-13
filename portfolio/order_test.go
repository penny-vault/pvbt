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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/portfolio"
)

// mockBroker records submitted orders and returns pre-configured fills.
type mockBroker struct {
	submitted    []broker.Order
	fills        [][]broker.Fill              // one []Fill per Submit call, consumed in order
	fillsByAsset map[asset.Asset][]broker.Fill // look up fills by asset (for map-iteration-safe tests)
	defaultFill  *broker.Fill                 // when set, return a fill at this price/time with order qty
	submitErr    error                        // when set, Submit returns this error immediately
	callIdx      int
}

func (m *mockBroker) Connect(context.Context) error { return nil }
func (m *mockBroker) Close() error                   { return nil }
func (m *mockBroker) Cancel(_ context.Context, _ string) error {
	return nil
}
func (m *mockBroker) Replace(_ context.Context, _ string, _ broker.Order) ([]broker.Fill, error) {
	return nil, nil
}
func (m *mockBroker) Orders(_ context.Context) ([]broker.Order, error)       { return nil, nil }
func (m *mockBroker) Positions(_ context.Context) ([]broker.Position, error) { return nil, nil }
func (m *mockBroker) Balance(_ context.Context) (broker.Balance, error) {
	return broker.Balance{}, nil
}

func (m *mockBroker) Submit(_ context.Context, order broker.Order) ([]broker.Fill, error) {
	m.submitted = append(m.submitted, order)
	if m.submitErr != nil {
		return nil, m.submitErr
	}
	if fills, ok := m.fillsByAsset[order.Asset]; ok {
		return fills, nil
	}
	if m.callIdx < len(m.fills) {
		f := m.fills[m.callIdx]
		m.callIdx++
		return f, nil
	}
	if m.defaultFill != nil {
		return []broker.Fill{{
			Price:    m.defaultFill.Price,
			Qty:      order.Qty,
			FilledAt: m.defaultFill.FilledAt,
		}}, nil
	}
	Fail(fmt.Sprintf("mockBroker: unexpected Submit call #%d for %s with no configured fill", m.callIdx, order.Asset.Ticker))
	return nil, nil
}

var _ broker.Broker = (*mockBroker)(nil)

var _ = Describe("Order", func() {
	var (
		testAsset asset.Asset
		mb        *mockBroker
		acct      *portfolio.Account
	)

	BeforeEach(func() {
		testAsset = asset.Asset{CompositeFigi: "TEST001", Ticker: "TEST"}
		mb = &mockBroker{
			defaultFill: &broker.Fill{Price: 100.0, FilledAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)},
		}
		acct = portfolio.New(portfolio.WithCash(10_000, time.Time{}), portfolio.WithBroker(mb))
	})

	It("places a market buy order via broker", func() {
		acct.Order(context.Background(), testAsset, portfolio.Buy, 10)

		Expect(mb.submitted).To(HaveLen(1))
		ord := mb.submitted[0]
		Expect(ord.Side).To(Equal(broker.Buy))
		Expect(ord.Qty).To(Equal(10.0))
		Expect(ord.OrderType).To(Equal(broker.Market))
		Expect(ord.TimeInForce).To(Equal(broker.Day))
		Expect(acct.Cash()).To(Equal(9_000.0))
		Expect(acct.Position(testAsset)).To(Equal(10.0))
	})

	It("places a limit order", func() {
		acct.Order(context.Background(), testAsset, portfolio.Buy, 5, portfolio.Limit(50.0))

		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].OrderType).To(Equal(broker.Limit))
		Expect(mb.submitted[0].LimitPrice).To(Equal(50.0))
	})

	It("places a stop order", func() {
		acct.Order(context.Background(), testAsset, portfolio.Buy, 5, portfolio.Stop(45.0))

		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].OrderType).To(Equal(broker.Stop))
		Expect(mb.submitted[0].StopPrice).To(Equal(45.0))
	})

	It("places a stop-limit order", func() {
		acct.Order(context.Background(), testAsset, portfolio.Buy, 5, portfolio.Limit(50.0), portfolio.Stop(45.0))

		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].OrderType).To(Equal(broker.StopLimit))
		Expect(mb.submitted[0].LimitPrice).To(Equal(50.0))
		Expect(mb.submitted[0].StopPrice).To(Equal(45.0))
	})

	It("handles GoodTilCancel modifier", func() {
		acct.Order(context.Background(), testAsset, portfolio.Buy, 5, portfolio.GoodTilCancel)

		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].TimeInForce).To(Equal(broker.GTC))
	})

	It("handles GoodTilDate modifier", func() {
		gtdDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
		acct.Order(context.Background(), testAsset, portfolio.Buy, 5, portfolio.GoodTilDate(gtdDate))

		Expect(mb.submitted).To(HaveLen(1))
		Expect(mb.submitted[0].TimeInForce).To(Equal(broker.GTD))
		Expect(mb.submitted[0].GTDDate).To(Equal(gtdDate))
	})

	It("places a sell order", func() {
		fillTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		mb.fills = [][]broker.Fill{
			{{Price: 100.0, Qty: 10, FilledAt: fillTime}},
			{{Price: 105.0, Qty: 10, FilledAt: fillTime}},
		}

		acct.Order(context.Background(), testAsset, portfolio.Buy, 10)
		acct.Order(context.Background(), testAsset, portfolio.Sell, 10)

		Expect(mb.submitted).To(HaveLen(2))
		Expect(mb.submitted[1].Side).To(Equal(broker.Sell))
		Expect(acct.Cash()).To(Equal(10_050.0)) // 10000 - 1000 + 1050
		Expect(acct.Position(testAsset)).To(Equal(0.0))
	})

	It("handles multiple fills for a single order", func() {
		fillTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		mb.fills = [][]broker.Fill{
			{
				{Price: 300.0, Qty: 6, FilledAt: fillTime},
				{Price: 299.0, Qty: 4, FilledAt: fillTime},
			},
		}

		acct.Order(context.Background(), testAsset, portfolio.Buy, 10)

		Expect(acct.Position(testAsset)).To(Equal(10.0))
		// cash: 10_000 - (6*300 + 4*299) = 10_000 - 2996 = 7_004
		Expect(acct.Cash()).To(Equal(7_004.0))
		// should produce 2 BuyTransactions (one per fill) + 1 initial deposit
		txns := acct.Transactions()
		buyCount := 0
		for _, tx := range txns {
			if tx.Type == portfolio.BuyTransaction {
				buyCount++
			}
		}
		Expect(buyCount).To(Equal(2))
	})

	It("leaves cash and position unchanged when broker returns an error", func() {
		mb.submitErr = fmt.Errorf("connection refused")
		acct.Order(context.Background(), testAsset, portfolio.Buy, 10)

		Expect(acct.Cash()).To(Equal(10_000.0))
		Expect(acct.Position(testAsset)).To(Equal(0.0))
		// The order was still submitted to the broker.
		Expect(mb.submitted).To(HaveLen(1))
		// Only the initial deposit transaction should exist.
		txns := acct.Transactions()
		Expect(txns).To(HaveLen(1))
		Expect(txns[0].Type).To(Equal(portfolio.DepositTransaction))
	})

	Context("edge cases", func() {
		It("submits an order with zero quantity", func() {
			acct.Order(context.Background(), testAsset, portfolio.Buy, 0)

			// The order is forwarded to the broker with qty=0.
			Expect(mb.submitted).To(HaveLen(1))
			Expect(mb.submitted[0].Qty).To(Equal(0.0))

			// A fill with qty=0 produces a transaction with zero amount,
			// so cash and position are unchanged.
			Expect(acct.Cash()).To(Equal(10_000.0))
			Expect(acct.Position(testAsset)).To(Equal(0.0))
		})

		It("submits an order with negative quantity", func() {
			acct.Order(context.Background(), testAsset, portfolio.Buy, -5)

			// The broker receives the negative qty as-is.
			Expect(mb.submitted).To(HaveLen(1))
			Expect(mb.submitted[0].Qty).To(Equal(-5.0))

			// The mock returns a fill with qty = order.Qty = -5.
			// amount = -(price * qty) = -(100 * -5) = +500, so cash increases.
			// holdings += -5, giving a negative position.
			Expect(acct.Cash()).To(Equal(10_500.0))
			Expect(acct.Position(testAsset)).To(Equal(-5.0))
		})

		It("sells more shares than currently held", func() {
			fillTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
			mb.fills = [][]broker.Fill{
				{{Price: 100.0, Qty: 5, FilledAt: fillTime}},
				{{Price: 110.0, Qty: 10, FilledAt: fillTime}},
			}

			// Buy 5 shares, then sell 10 (more than held).
			acct.Order(context.Background(), testAsset, portfolio.Buy, 5)
			acct.Order(context.Background(), testAsset, portfolio.Sell, 10)

			Expect(mb.submitted).To(HaveLen(2))

			// After buy: cash = 10000 - 500 = 9500, position = 5
			// After sell: cash = 9500 + 1100 = 10600, position = 5 - 10 = -5
			// The account does not prevent overselling; it results in a
			// negative (short) position.
			Expect(acct.Cash()).To(Equal(10_600.0))
			Expect(acct.Position(testAsset)).To(Equal(-5.0))
		})
	})

	DescribeTable("time-in-force modifiers",
		func(mod portfolio.OrderModifier, expected broker.TimeInForce) {
			acct.Order(context.Background(), testAsset, portfolio.Buy, 1, mod)
			Expect(mb.submitted).To(HaveLen(1))
			Expect(mb.submitted[0].TimeInForce).To(Equal(expected))
		},
		Entry("DayOrder", portfolio.DayOrder, broker.Day),
		Entry("GoodTilCancel", portfolio.GoodTilCancel, broker.GTC),
		Entry("FillOrKill", portfolio.FillOrKill, broker.FOK),
		Entry("ImmediateOrCancel", portfolio.ImmediateOrCancel, broker.IOC),
		Entry("OnTheOpen", portfolio.OnTheOpen, broker.OnOpen),
		Entry("OnTheClose", portfolio.OnTheClose, broker.OnClose),
	)
})
