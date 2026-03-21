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
	"github.com/penny-vault/pvbt/portfolio"
)

// mockPortfolio implements portfolio.Portfolio for margin tests.
type mockPortfolio struct {
	positions        map[asset.Asset]float64
	equity           float64
	shortMarketValue float64
}

func (m *mockPortfolio) Position(a asset.Asset) float64      { return m.positions[a] }
func (m *mockPortfolio) Equity() float64                     { return m.equity }
func (m *mockPortfolio) ShortMarketValue() float64           { return m.shortMarketValue }
func (m *mockPortfolio) Cash() float64                       { return 0 }
func (m *mockPortfolio) Value() float64                      { return 0 }
func (m *mockPortfolio) PositionValue(_ asset.Asset) float64 { return 0 }
func (m *mockPortfolio) Holdings(_ func(asset.Asset, float64)) {}
func (m *mockPortfolio) Transactions() []portfolio.Transaction { return nil }
func (m *mockPortfolio) Prices() *data.DataFrame             { return nil }
func (m *mockPortfolio) PerfData() *data.DataFrame           { return nil }
func (m *mockPortfolio) PerformanceMetric(_ portfolio.PerformanceMetric) portfolio.PerformanceMetricQuery {
	return portfolio.PerformanceMetricQuery{}
}
func (m *mockPortfolio) Summary() (portfolio.Summary, error)               { return portfolio.Summary{}, nil }
func (m *mockPortfolio) RiskMetrics() (portfolio.RiskMetrics, error)        { return portfolio.RiskMetrics{}, nil }
func (m *mockPortfolio) TaxMetrics() (portfolio.TaxMetrics, error)          { return portfolio.TaxMetrics{}, nil }
func (m *mockPortfolio) TradeMetrics() (portfolio.TradeMetrics, error)      { return portfolio.TradeMetrics{}, nil }
func (m *mockPortfolio) WithdrawalMetrics() (portfolio.WithdrawalMetrics, error) { return portfolio.WithdrawalMetrics{}, nil }
func (m *mockPortfolio) SetMetadata(_, _ string)                           {}
func (m *mockPortfolio) GetMetadata(_ string) string                       { return "" }
func (m *mockPortfolio) Annotations() []portfolio.Annotation               { return nil }
func (m *mockPortfolio) TradeDetails() []portfolio.TradeDetail              { return nil }
func (m *mockPortfolio) LongMarketValue() float64                          { return 0 }
func (m *mockPortfolio) MarginRatio() float64                              { return 0 }
func (m *mockPortfolio) MarginDeficiency() float64                         { return 0 }
func (m *mockPortfolio) BuyingPower() float64                              { return 0 }

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
	columns := make([][]float64, len(assets))
	for idx := range assets {
		columns[idx] = []float64{vals[idx]}
	}
	df, err := data.NewDataFrame(times, assets, metrics, data.Daily, columns)
	if err != nil {
		return nil, err
	}
	return df, nil
}

// mockHLPriceProvider implements broker.PriceProvider with high, low, and close data.
type mockHLPriceProvider struct {
	high  map[asset.Asset]float64
	low   map[asset.Asset]float64
	close map[asset.Asset]float64
	date  time.Time
}

func (m *mockHLPriceProvider) Prices(_ context.Context, assets ...asset.Asset) (*data.DataFrame, error) {
	times := []time.Time{m.date}
	metrics := []data.Metric{data.MetricClose, data.MetricHigh, data.MetricLow}
	// Layout: for T=1, A=len(assets), M=3:
	// vals[(a*3+0)*1+0] = close[a], vals[(a*3+1)*1+0] = high[a], vals[(a*3+2)*1+0] = low[a]
	vals := make([]float64, len(assets)*3)
	for idx, a := range assets {
		vals[idx*3+0] = m.close[a]
		vals[idx*3+1] = m.high[a]
		vals[idx*3+2] = m.low[a]
	}
	numCols := len(assets) * len(metrics)
	df, err := data.NewDataFrame(times, assets, metrics, data.Daily, data.SlabToColumns(vals, numCols, 1))
	if err != nil {
		return nil, err
	}
	return df, nil
}

// Compile-time interface checks.
var _ broker.Broker = (*engine.SimulatedBroker)(nil)
var _ broker.GroupSubmitter = (*engine.SimulatedBroker)(nil)

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

		It("rejects a short order when it would violate initial margin", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{aapl: 100.0},
				date:   date,
			}, date)

			// Portfolio with low equity relative to proposed short value.
			// Equity = 100, existing short value = 0, selling 10 shares @ 100 = 1000 short value.
			// Margin ratio = 100/1000 = 0.10, below default 0.50 threshold.
			simBroker.SetPortfolio(&mockPortfolio{
				positions:        map[asset.Asset]float64{},
				equity:           100,
				shortMarketValue: 0,
			})

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       10,
				OrderType: broker.Market,
			})

			Expect(err).NotTo(HaveOccurred())
			Consistently(simBroker.Fills()).ShouldNot(Receive())
		})

		It("fills a short order when margin is sufficient", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{aapl: 100.0},
				date:   date,
			}, date)

			// Equity = 10000, selling 10 shares @ 100 = 1000 short value.
			// Margin ratio = 10000/1000 = 10.0, well above 0.50 threshold.
			simBroker.SetPortfolio(&mockPortfolio{
				positions:        map[asset.Asset]float64{},
				equity:           10000,
				shortMarketValue: 0,
			})

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       10,
				OrderType: broker.Market,
			})

			Expect(err).NotTo(HaveOccurred())

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.Qty).To(Equal(10.0))
			Expect(fill.Price).To(Equal(100.0))
		})

		It("does not margin-check sell orders that close long positions", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockPriceProvider{
				prices: map[asset.Asset]float64{aapl: 100.0},
				date:   date,
			}, date)

			// Portfolio holds 20 shares long, selling 15 closes part of the long.
			// currentPos(20) - qty(15) = 5 >= 0, so no margin check.
			simBroker.SetPortfolio(&mockPortfolio{
				positions:        map[asset.Asset]float64{aapl: 20},
				equity:           50, // Very low equity -- would fail margin if checked.
				shortMarketValue: 0,
			})

			err := simBroker.Submit(context.Background(), broker.Order{
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       15,
				OrderType: broker.Market,
			})

			Expect(err).NotTo(HaveOccurred())

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.Qty).To(Equal(15.0))
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

	Context("SubmitGroup", func() {
		It("stores OCO legs as deferred orders", func() {
			simBroker := engine.NewSimulatedBroker()

			stopOrder := broker.Order{
				ID:        "grp-sl-1",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "group-1",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "grp-tp-1",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 160.0,
				GroupID:    "group-1",
				GroupRole:  broker.RoleTakeProfit,
			}

			err := simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)
			Expect(err).NotTo(HaveOccurred())

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(2))
		})
	})

	Context("EvaluatePending", func() {
		It("triggers stop loss when low <= stop price for long (Sell side)", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockHLPriceProvider{
				high:  map[asset.Asset]float64{aapl: 155.0},
				low:   map[asset.Asset]float64{aapl: 138.0},
				close: map[asset.Asset]float64{aapl: 145.0},
				date:  date,
			}, date)

			stopOrder := broker.Order{
				ID:        "sl-1",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "grp-a",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "tp-1",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 165.0,
				GroupID:    "grp-a",
				GroupRole:  broker.RoleTakeProfit,
			}

			Expect(simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)).To(Succeed())
			simBroker.EvaluatePending()

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("sl-1"))
			Expect(fill.Price).To(Equal(140.0))
			Expect(fill.Qty).To(Equal(100.0))

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("triggers take profit when high >= limit price for long (Sell side)", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockHLPriceProvider{
				high:  map[asset.Asset]float64{aapl: 168.0},
				low:   map[asset.Asset]float64{aapl: 148.0},
				close: map[asset.Asset]float64{aapl: 155.0},
				date:  date,
			}, date)

			stopOrder := broker.Order{
				ID:        "sl-2",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "grp-b",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "tp-2",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 165.0,
				GroupID:    "grp-b",
				GroupRole:  broker.RoleTakeProfit,
			}

			Expect(simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)).To(Succeed())
			simBroker.EvaluatePending()

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("tp-2"))
			Expect(fill.Price).To(Equal(165.0))
			Expect(fill.Qty).To(Equal(100.0))

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("stop loss wins when both trigger on same bar", func() {
			simBroker := engine.NewSimulatedBroker()
			// Both high >= TP limit (165) and low <= SL stop (140) on same bar.
			simBroker.SetPriceProvider(&mockHLPriceProvider{
				high:  map[asset.Asset]float64{aapl: 170.0},
				low:   map[asset.Asset]float64{aapl: 135.0},
				close: map[asset.Asset]float64{aapl: 150.0},
				date:  date,
			}, date)

			stopOrder := broker.Order{
				ID:        "sl-3",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "grp-c",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "tp-3",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 165.0,
				GroupID:    "grp-c",
				GroupRole:  broker.RoleTakeProfit,
			}

			Expect(simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)).To(Succeed())
			simBroker.EvaluatePending()

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("sl-3"))
			Expect(fill.Price).To(Equal(140.0))

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
		})

		It("does nothing when neither stop loss nor take profit triggers", func() {
			simBroker := engine.NewSimulatedBroker()
			simBroker.SetPriceProvider(&mockHLPriceProvider{
				high:  map[asset.Asset]float64{aapl: 155.0},
				low:   map[asset.Asset]float64{aapl: 145.0},
				close: map[asset.Asset]float64{aapl: 150.0},
				date:  date,
			}, date)

			stopOrder := broker.Order{
				ID:        "sl-4",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "grp-d",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "tp-4",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 165.0,
				GroupID:    "grp-d",
				GroupRole:  broker.RoleTakeProfit,
			}

			Expect(simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)).To(Succeed())
			simBroker.EvaluatePending()

			Expect(simBroker.Fills()).NotTo(Receive())

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(HaveLen(2))
		})

		It("falls back to close price when high/low unavailable", func() {
			simBroker := engine.NewSimulatedBroker()
			// High/low are 0 (unavailable); close price is at or below stop price.
			simBroker.SetPriceProvider(&mockHLPriceProvider{
				high:  map[asset.Asset]float64{aapl: 0},
				low:   map[asset.Asset]float64{aapl: 0},
				close: map[asset.Asset]float64{aapl: 138.0},
				date:  date,
			}, date)

			stopOrder := broker.Order{
				ID:        "sl-5",
				Asset:     aapl,
				Side:      broker.Sell,
				Qty:       100,
				OrderType: broker.Stop,
				StopPrice: 140.0,
				GroupID:   "grp-e",
				GroupRole: broker.RoleStopLoss,
			}
			tpOrder := broker.Order{
				ID:         "tp-5",
				Asset:      aapl,
				Side:       broker.Sell,
				Qty:        100,
				OrderType:  broker.Limit,
				LimitPrice: 165.0,
				GroupID:    "grp-e",
				GroupRole:  broker.RoleTakeProfit,
			}

			Expect(simBroker.SubmitGroup(context.Background(), []broker.Order{stopOrder, tpOrder}, broker.GroupOCO)).To(Succeed())
			simBroker.EvaluatePending()

			var fill broker.Fill
			Eventually(simBroker.Fills()).Should(Receive(&fill))
			Expect(fill.OrderID).To(Equal("sl-5"))
			Expect(fill.Price).To(Equal(140.0))

			orders, err := simBroker.Orders(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(orders).To(BeEmpty())
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
