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

package engine_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
)

// mockPriceProvider implements broker.PriceProvider for tests.
type mockPriceProvider struct {
	prices map[asset.Asset]float64
	date   time.Time
}

func (m *mockPriceProvider) Prices(_ context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	times := []time.Time{m.date}
	metrics := []data.Metric{data.MetricClose}
	vals := make([]float64, len(assets))
	for idx, held := range assets {
		if price, ok := m.prices[held]; ok {
			vals[idx] = price
		}
	}
	df, err := data.NewDataFrame(times, assets, metrics, data.Daily, vals)
	if err != nil {
		return nil, err
	}
	return df, nil
}

// Compile-time interface check.
var _ broker.Broker = (*engine.SimulatedBroker)(nil)

var _ = Describe("SimulatedBroker", func() {
	var (
		aapl asset.Asset
		date time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		date = time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)
	})

	Context("Submit", func() {
		It("fills a market order at the close price", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{aapl: 150.0},
				date:   date,
			}, date)

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     aapl,
				Side:      broker.Buy,
				Qty:       100,
				OrderType: broker.Market,
			})

			Expect(err).NotTo(HaveOccurred())

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.Price).To(Equal(150.0))
			Expect(fill.Qty).To(Equal(100.0))
			Expect(fill.FilledAt).To(Equal(date))
		})

		It("delivers fills through the Fills channel", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{aapl: 200.0},
				date:   date,
			}, date)

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     aapl,
				Side:      broker.Buy,
				Qty:       50,
				OrderType: broker.Market,
			})

			Expect(err).NotTo(HaveOccurred())

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.Price).To(Equal(200.0))
			Expect(fill.Qty).To(Equal(50.0))
			Expect(fill.FilledAt).To(Equal(date))
		})

		It("returns an error for an asset with no price", func() {
			unknown := asset.Asset{CompositeFigi: "FIGI-UNKNOWN", Ticker: "UNKNOWN"}
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{},
				date:   date,
			}, date)

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     unknown,
				Side:      broker.Buy,
				Qty:       100,
				OrderType: broker.Market,
			})

			Expect(err).To(HaveOccurred())
		})
	})

	Context("Orders", func() {
		It("returns empty when no deferred orders exist", func() {
			simBroker := engine.NewSimulatedBroker()

			orders, err := simBroker.Orders(context.Background())

			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})
	})

	Context("Cancel", func() {
		It("removes a deferred stop-loss order from pending", func() {
			simBroker := engine.NewSimulatedBroker()

			stopOrder := broker.Order{
				ID:        "order-sl-1",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Market,
				GroupRole: broker.RoleStopLoss,
			}
			err := simBroker.Submit(context.Background(), stopOrder)
			Expect(err).NotTo(HaveOccurred())

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(1))

			err = simBroker.Cancel(context.Background(), "order-sl-1")
			Expect(err).NotTo(HaveOccurred())

			orders, err = simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("returns an error for an unknown order ID", func() {
			simBroker := engine.NewSimulatedBroker()

			err := simBroker.Cancel(context.Background(), "nonexistent-order")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-order"))
		})
	})

	Context("Connect and Close", func() {
		It("succeeds without error", func() {
			simBroker := engine.NewSimulatedBroker()
			Expect(simBroker.Connect(context.Background())).To(Succeed())
			Expect(simBroker.Close()).To(Succeed())
		})
	})

	Context("Broker lifecycle", func() {
		It("calls Close on the broker when engine.Close() is called", func() {
			mock := &mockLifecycleBroker{}
			eng := engine.New(nil, engine.WithBroker(mock))

			Expect(mock.closeCalled).To(BeFalse())
			Expect(eng.Close()).To(Succeed())
			Expect(mock.closeCalled).To(BeTrue())
		})
	})
})

// mockLifecycleBroker is a minimal broker.Broker that tracks Connect/Close calls.
type mockLifecycleBroker struct {
	connectCalled bool
	closeCalled   bool
}

func (mock *mockLifecycleBroker) Connect(_ context.Context) error {
	mock.connectCalled = true
	return nil
}

func (mock *mockLifecycleBroker) Close() error {
	mock.closeCalled = true
	return nil
}

func (mock *mockLifecycleBroker) Submit(_ context.Context, _ broker.Order) error        { return nil }
func (mock *mockLifecycleBroker) Fills() <-chan broker.Fill                              { return nil }
func (mock *mockLifecycleBroker) Cancel(_ context.Context, _ string) error               { return nil }
func (mock *mockLifecycleBroker) Replace(_ context.Context, _ string, _ broker.Order) error {
	return nil
}
func (mock *mockLifecycleBroker) Orders(_ context.Context) ([]broker.Order, error)      { return nil, nil }
func (mock *mockLifecycleBroker) Positions(_ context.Context) ([]broker.Position, error) { return nil, nil }
func (mock *mockLifecycleBroker) Balance(_ context.Context) (broker.Balance, error)     { return broker.Balance{}, nil }
