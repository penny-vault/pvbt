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

package portfolio

import (
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// taxLot tracks the purchase date, quantity, and price of a position for
// tax gain/loss calculations.
type taxLot struct {
	Date  time.Time
	Qty   float64
	Price float64
}

// Option configures an Account during construction.
type Option func(*Account)

// Account is the concrete type that implements both Portfolio and
// PortfolioManager. The user creates an Account with New, passes it to
// the engine, and inspects it after the run. The engine holds it as
// *Account (giving access to both interfaces): it passes it as Portfolio
// to strategy Compute calls, and calls Record/SetBroker directly.
type Account struct {
	cash            float64
	holdings        map[asset.Asset]float64
	transactions    []Transaction
	broker          broker.Broker
	prices          *data.DataFrame
	equityCurve     []float64
	equityTimes     []time.Time
	benchmarkPrices []float64
	riskFreePrices  []float64
	benchmark       asset.Asset
	riskFree        asset.Asset
	taxLots         map[asset.Asset][]taxLot
}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	a := &Account{
		holdings: make(map[asset.Asset]float64),
		taxLots:  make(map[asset.Asset][]taxLot),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// WithBroker returns an Option that attaches a broker to the account
// at construction time.
func WithBroker(b broker.Broker) Option {
	return func(a *Account) {
		a.SetBroker(b)
	}
}

// WithCash returns an Option that sets the initial cash balance and
// records a DepositTransaction.
func WithCash(amount float64) Option {
	return func(a *Account) {
		a.cash = amount
		a.transactions = append(a.transactions, Transaction{
			Type:   DepositTransaction,
			Amount: amount,
		})
	}
}

// WithBenchmark returns an Option that stores the benchmark asset.
func WithBenchmark(b asset.Asset) Option {
	return func(a *Account) {
		a.benchmark = b
	}
}

// WithRiskFree returns an Option that stores the risk-free asset.
func WithRiskFree(rf asset.Asset) Option {
	return func(a *Account) {
		a.riskFree = rf
	}
}

// Benchmark returns the benchmark asset.
func (a *Account) Benchmark() asset.Asset {
	return a.benchmark
}

// RiskFree returns the risk-free asset.
func (a *Account) RiskFree() asset.Asset {
	return a.riskFree
}

// --- Portfolio interface ---

func (a *Account) RebalanceTo(alloc ...Allocation)                                     {}
func (a *Account) Order(ast asset.Asset, side Side, qty float64, mods ...OrderModifier) {}

// Cash returns the current cash balance.
func (a *Account) Cash() float64 {
	return a.cash
}

// Value returns the total portfolio value: cash plus all holdings marked
// to current prices. If no prices have been set yet, returns cash only.
func (a *Account) Value() float64 {
	total := a.cash
	if a.prices != nil {
		for ast, qty := range a.holdings {
			v := a.prices.Value(ast, data.MetricClose)
			if !math.IsNaN(v) {
				total += qty * v
			}
		}
	}
	return total
}

// Position returns the quantity held of a specific asset.
func (a *Account) Position(ast asset.Asset) float64 {
	return a.holdings[ast]
}

// PositionValue returns the current market value of the position in a
// specific asset (quantity * current price), or 0 if no prices or no position.
func (a *Account) PositionValue(ast asset.Asset) float64 {
	qty := a.holdings[ast]
	if qty == 0 || a.prices == nil {
		return 0
	}
	v := a.prices.Value(ast, data.MetricClose)
	if math.IsNaN(v) {
		return 0
	}
	return qty * v
}

// Holdings iterates over all current positions, calling fn with each
// asset and its held quantity.
func (a *Account) Holdings(fn func(asset.Asset, float64)) {
	for ast, qty := range a.holdings {
		fn(ast, qty)
	}
}

// Transactions returns the full transaction log in chronological order.
func (a *Account) Transactions() []Transaction {
	return a.transactions
}

func (a *Account) PerformanceMetric(m PerformanceMetric) PerformanceMetricQuery {
	return PerformanceMetricQuery{account: a, metric: m}
}

func (a *Account) Summary() Summary {
	return Summary{
		TWRR:        a.PerformanceMetric(TWRR).Value(),
		MWRR:        a.PerformanceMetric(MWRR).Value(),
		Sharpe:      a.PerformanceMetric(Sharpe).Value(),
		Sortino:     a.PerformanceMetric(Sortino).Value(),
		Calmar:      a.PerformanceMetric(Calmar).Value(),
		MaxDrawdown: a.PerformanceMetric(MaxDrawdown).Value(),
		StdDev:      a.PerformanceMetric(StdDev).Value(),
	}
}

func (a *Account) RiskMetrics() RiskMetrics {
	return RiskMetrics{
		Beta:                 a.PerformanceMetric(Beta).Value(),
		Alpha:                a.PerformanceMetric(Alpha).Value(),
		TrackingError:        a.PerformanceMetric(TrackingError).Value(),
		DownsideDeviation:    a.PerformanceMetric(DownsideDeviation).Value(),
		InformationRatio:     a.PerformanceMetric(InformationRatio).Value(),
		Treynor:              a.PerformanceMetric(Treynor).Value(),
		UlcerIndex:           a.PerformanceMetric(UlcerIndex).Value(),
		ExcessKurtosis:       a.PerformanceMetric(ExcessKurtosis).Value(),
		Skewness:             a.PerformanceMetric(Skewness).Value(),
		RSquared:             a.PerformanceMetric(RSquared).Value(),
		ValueAtRisk:          a.PerformanceMetric(ValueAtRisk).Value(),
		UpsideCaptureRatio:   a.PerformanceMetric(UpsideCaptureRatio).Value(),
		DownsideCaptureRatio: a.PerformanceMetric(DownsideCaptureRatio).Value(),
	}
}

func (a *Account) TaxMetrics() TaxMetrics {
	return TaxMetrics{}
}

func (a *Account) TradeMetrics() TradeMetrics {
	return TradeMetrics{}
}

func (a *Account) WithdrawalMetrics() WithdrawalMetrics {
	return WithdrawalMetrics{
		SafeWithdrawalRate:      a.PerformanceMetric(SafeWithdrawalRate).Value(),
		PerpetualWithdrawalRate: a.PerformanceMetric(PerpetualWithdrawalRate).Value(),
		DynamicWithdrawalRate:   a.PerformanceMetric(DynamicWithdrawalRate).Value(),
	}
}

// --- PortfolioManager interface ---

func (a *Account) Record(tx Transaction)         {}
func (a *Account) UpdatePrices(df *data.DataFrame) {}
func (a *Account) SetBroker(b broker.Broker)      { a.broker = b }
