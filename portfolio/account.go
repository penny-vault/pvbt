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
func (a *Account) Order(ast asset.Asset, side Side, qty float64, mods ...OrderModifier) {
	order := broker.Order{
		Asset:       ast,
		Qty:         qty,
		OrderType:   broker.Market,
		TimeInForce: broker.Day,
	}

	// Map portfolio side to broker side.
	switch side {
	case Buy:
		order.Side = broker.Buy
	case Sell:
		order.Side = broker.Sell
	}

	// Apply modifiers.
	var hasLimit, hasStop bool
	for _, mod := range mods {
		switch m := mod.(type) {
		case limitModifier:
			order.LimitPrice = m.price
			hasLimit = true
		case stopModifier:
			order.StopPrice = m.price
			hasStop = true
		case dayOrderModifier:
			order.TimeInForce = broker.Day
		case goodTilCancelModifier:
			order.TimeInForce = broker.GTC
		case fillOrKillModifier:
			order.TimeInForce = broker.FOK
		case immediateOrCancelModifier:
			order.TimeInForce = broker.IOC
		case onTheOpenModifier:
			order.TimeInForce = broker.OnOpen
		case onTheCloseModifier:
			order.TimeInForce = broker.OnClose
		case goodTilDateModifier:
			order.TimeInForce = broker.GTD
			order.GTDDate = m.date
		}
	}

	// Determine order type from price modifiers.
	if hasLimit && hasStop {
		order.OrderType = broker.StopLimit
	} else if hasLimit {
		order.OrderType = broker.Limit
	} else if hasStop {
		order.OrderType = broker.Stop
	}

	// Submit to broker.
	fill, err := a.broker.Submit(order)
	if err != nil {
		return
	}

	// Record the fill as a transaction.
	var txType TransactionType
	var amount float64
	switch side {
	case Buy:
		txType = BuyTransaction
		amount = -(fill.Price * fill.Qty)
	case Sell:
		txType = SellTransaction
		amount = fill.Price * fill.Qty
	}

	a.Record(Transaction{
		Date:   fill.FilledAt,
		Asset:  ast,
		Type:   txType,
		Qty:    fill.Qty,
		Price:  fill.Price,
		Amount: amount,
	})
}

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

// Record appends a transaction to the log and updates cash, holdings,
// and tax lots accordingly.
func (a *Account) Record(tx Transaction) {
	a.transactions = append(a.transactions, tx)
	a.cash += tx.Amount

	switch tx.Type {
	case BuyTransaction:
		a.holdings[tx.Asset] += tx.Qty
		a.taxLots[tx.Asset] = append(a.taxLots[tx.Asset], taxLot{
			Date:  tx.Date,
			Qty:   tx.Qty,
			Price: tx.Price,
		})
	case SellTransaction:
		a.holdings[tx.Asset] -= tx.Qty
		remaining := tx.Qty
		lots := a.taxLots[tx.Asset]
		i := 0
		for i < len(lots) && remaining > 0 {
			if lots[i].Qty <= remaining {
				remaining -= lots[i].Qty
				i++
			} else {
				lots[i].Qty -= remaining
				remaining = 0
			}
		}
		a.taxLots[tx.Asset] = lots[i:]
		if a.holdings[tx.Asset] == 0 {
			delete(a.holdings, tx.Asset)
			delete(a.taxLots, tx.Asset)
		}
	}
}
// UpdatePrices stores the latest price DataFrame, computes the total
// portfolio value, and appends it to the equity curve. It also tracks
// benchmark and risk-free price series when those assets are configured.
func (a *Account) UpdatePrices(df *data.DataFrame) {
	a.prices = df

	// Compute total portfolio value: cash + marked holdings.
	total := a.cash
	for ast, qty := range a.holdings {
		v := df.Value(ast, data.MetricClose)
		if !math.IsNaN(v) {
			total += qty * v
		}
	}

	a.equityCurve = append(a.equityCurve, total)
	a.equityTimes = append(a.equityTimes, df.End())

	// Track benchmark price series (AdjClose).
	if a.benchmark != (asset.Asset{}) {
		v := df.Value(a.benchmark, data.AdjClose)
		if !math.IsNaN(v) {
			a.benchmarkPrices = append(a.benchmarkPrices, v)
		}
	}

	// Track risk-free price series (AdjClose).
	if a.riskFree != (asset.Asset{}) {
		v := df.Value(a.riskFree, data.AdjClose)
		if !math.IsNaN(v) {
			a.riskFreePrices = append(a.riskFreePrices, v)
		}
	}
}

// EquityCurve returns the equity curve slice.
func (a *Account) EquityCurve() []float64 { return a.equityCurve }

// EquityTimes returns the equity times slice.
func (a *Account) EquityTimes() []time.Time { return a.equityTimes }

// BenchmarkPrices returns the benchmark prices slice.
func (a *Account) BenchmarkPrices() []float64 { return a.benchmarkPrices }

// RiskFreePrices returns the risk-free prices slice.
func (a *Account) RiskFreePrices() []float64 { return a.riskFreePrices }
func (a *Account) SetBroker(b broker.Broker)      { a.broker = b }
