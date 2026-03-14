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
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog/log"
)

var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
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
	perfData        *data.DataFrame
	benchmark       asset.Asset
	riskFree        asset.Asset
	taxLots            map[asset.Asset][]TaxLot
	metadata           map[string]string
	metrics            []MetricRow
	registeredMetrics  []PerformanceMetric
}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	a := &Account{
		holdings: make(map[asset.Asset]float64),
		taxLots:  make(map[asset.Asset][]TaxLot),
		metadata: make(map[string]string),
	}
	for _, opt := range opts {
		opt(a)
	}

	// Default: register all metrics if none were explicitly specified.
	if len(a.registeredMetrics) == 0 {
		WithAllMetrics()(a)
	}

	return a
}

// WithBroker returns an Option that sets the broker used for order
// execution. A broker is always required.
func WithBroker(b broker.Broker) Option {
	return func(a *Account) {
		a.SetBroker(b)
	}
}

// WithCash returns an Option that sets the initial cash balance and
// records a DepositTransaction on the given date.
func WithCash(amount float64, date time.Time) Option {
	return func(a *Account) {
		a.cash = amount
		a.transactions = append(a.transactions, Transaction{
			Date:   date,
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

func (a *Account) RebalanceTo(ctx context.Context, allocs ...Allocation) error {
	for _, alloc := range allocs {
		totalValue := a.Value()

		type pendingOrder struct {
			asset  asset.Asset
			side   Side
			amount float64 // dollar amount
			qty    float64 // share count (for full liquidations)
		}

		var sells []pendingOrder

		// Sell all holdings not in the target allocation.
		for ast, qty := range a.holdings {
			if _, ok := alloc.Members[ast]; !ok && qty > 0 {
				sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
			}
		}

		// Compute sells for overweight positions.
		for ast, weight := range alloc.Members {
			targetDollars := weight * totalValue
			currentDollars := a.PositionValue(ast)
			diff := targetDollars - currentDollars

			if diff < 0 {
				sells = append(sells, pendingOrder{asset: ast, side: Sell, amount: -diff})
			}
		}

		// Process sells first to free up cash.
		for _, o := range sells {
			order := broker.Order{
				Asset:       o.asset,
				Side:        broker.Sell,
				Qty:         o.qty,
				Amount:      o.amount,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			if err := a.submitAndRecord(ctx, o.asset, Sell, order); err != nil {
				return fmt.Errorf("RebalanceTo: sell %s: %w", o.asset.Ticker, err)
			}
		}

		// Recompute target values after sells so buys use actual
		// available cash rather than the pre-sell portfolio value.
		postSellValue := a.Value()

		var buys []pendingOrder
		for ast, weight := range alloc.Members {
			targetDollars := weight * postSellValue
			currentDollars := a.PositionValue(ast)
			diff := targetDollars - currentDollars

			if diff > 0 {
				buys = append(buys, pendingOrder{asset: ast, side: Buy, amount: diff})
			}
		}

		for _, o := range buys {
			order := broker.Order{
				Asset:       o.asset,
				Side:        broker.Buy,
				Amount:      o.amount,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			if err := a.submitAndRecord(ctx, o.asset, Buy, order); err != nil {
				return fmt.Errorf("RebalanceTo: buy %s: %w", o.asset.Ticker, err)
			}
		}
	}
	return nil
}
func (a *Account) Order(ctx context.Context, ast asset.Asset, side Side, qty float64, mods ...OrderModifier) error {
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

	return a.submitAndRecord(ctx, ast, side, order)
}

// submitAndRecord sends an order to the broker and records each fill
// as a transaction. Used by both Order and RebalanceTo.
func (a *Account) submitAndRecord(ctx context.Context, ast asset.Asset, side Side, order broker.Order) error {
	fills, err := a.broker.Submit(ctx, order)
	if err != nil {
		return fmt.Errorf("order %s (qty=%.2f, amount=%.2f): %w", ast.Ticker, order.Qty, order.Amount, err)
	}

	for _, fill := range fills {
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
	return nil
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

func (a *Account) Summary() (Summary, error) {
	var errs []error
	s := Summary{}
	var err error

	s.TWRR, err = a.PerformanceMetric(TWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.MWRR, err = a.PerformanceMetric(MWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.Sharpe, err = a.PerformanceMetric(Sharpe).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.Sortino, err = a.PerformanceMetric(Sortino).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.Calmar, err = a.PerformanceMetric(Calmar).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.MaxDrawdown, err = a.PerformanceMetric(MaxDrawdown).Value()
	if err != nil {
		errs = append(errs, err)
	}

	s.StdDev, err = a.PerformanceMetric(StdDev).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return s, errors.Join(errs...)
}

func (a *Account) RiskMetrics() (RiskMetrics, error) {
	var errs []error
	r := RiskMetrics{}
	var err error

	r.Beta, err = a.PerformanceMetric(Beta).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.Alpha, err = a.PerformanceMetric(Alpha).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.TrackingError, err = a.PerformanceMetric(TrackingError).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.DownsideDeviation, err = a.PerformanceMetric(DownsideDeviation).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.InformationRatio, err = a.PerformanceMetric(InformationRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.Treynor, err = a.PerformanceMetric(Treynor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.UlcerIndex, err = a.PerformanceMetric(UlcerIndex).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.ExcessKurtosis, err = a.PerformanceMetric(ExcessKurtosis).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.Skewness, err = a.PerformanceMetric(Skewness).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.RSquared, err = a.PerformanceMetric(RSquared).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.ValueAtRisk, err = a.PerformanceMetric(ValueAtRisk).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.UpsideCaptureRatio, err = a.PerformanceMetric(UpsideCaptureRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	r.DownsideCaptureRatio, err = a.PerformanceMetric(DownsideCaptureRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return r, errors.Join(errs...)
}

func (a *Account) TaxMetrics() (TaxMetrics, error) {
	var errs []error
	t := TaxMetrics{}
	var err error

	t.LTCG, err = a.PerformanceMetric(LTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.STCG, err = a.PerformanceMetric(STCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.UnrealizedLTCG, err = a.PerformanceMetric(UnrealizedLTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.UnrealizedSTCG, err = a.PerformanceMetric(UnrealizedSTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.QualifiedDividends, err = a.PerformanceMetric(QualifiedDividendsMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.NonQualifiedIncome, err = a.PerformanceMetric(NonQualifiedIncomeMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.TaxCostRatio, err = a.PerformanceMetric(TaxCostRatioMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return t, errors.Join(errs...)
}

func (a *Account) TradeMetrics() (TradeMetrics, error) {
	var errs []error
	t := TradeMetrics{}
	var err error

	t.WinRate, err = a.PerformanceMetric(WinRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.AverageWin, err = a.PerformanceMetric(AverageWin).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.AverageLoss, err = a.PerformanceMetric(AverageLoss).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.ProfitFactor, err = a.PerformanceMetric(ProfitFactor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.AverageHoldingPeriod, err = a.PerformanceMetric(AverageHoldingPeriod).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.Turnover, err = a.PerformanceMetric(Turnover).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.NPositivePeriods, err = a.PerformanceMetric(NPositivePeriods).Value()
	if err != nil {
		errs = append(errs, err)
	}

	t.GainLossRatio, err = a.PerformanceMetric(TradeGainLossRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return t, errors.Join(errs...)
}

func (a *Account) WithdrawalMetrics() (WithdrawalMetrics, error) {
	var errs []error
	w := WithdrawalMetrics{}
	var err error

	w.SafeWithdrawalRate, err = a.PerformanceMetric(SafeWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	w.PerpetualWithdrawalRate, err = a.PerformanceMetric(PerpetualWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	w.DynamicWithdrawalRate, err = a.PerformanceMetric(DynamicWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return w, errors.Join(errs...)
}

// --- PortfolioManager interface ---

// Record appends a transaction to the log and updates cash, holdings,
// and tax lots accordingly.
func (a *Account) Record(tx Transaction) {
	if tx.Type == DividendTransaction {
		tx.Qualified = a.isDividendQualified(tx.Asset, tx.Date)
	}

	a.transactions = append(a.transactions, tx)
	a.cash += tx.Amount

	switch tx.Type {
	case BuyTransaction:
		a.holdings[tx.Asset] += tx.Qty
		a.taxLots[tx.Asset] = append(a.taxLots[tx.Asset], TaxLot{
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
// isDividendQualified checks whether the earliest FIFO tax lot for the
// asset was held for more than 60 days before the dividend date. This
// is a simplified version of the IRS 121-day window rule, appropriate
// for backtesting purposes.
func (a *Account) isDividendQualified(ast asset.Asset, divDate time.Time) bool {
	lots := a.taxLots[ast]
	if len(lots) == 0 {
		return false
	}

	holdingDays := divDate.Sub(lots[0].Date).Hours() / 24
	return holdingDays > 60
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

	// Track benchmark price series. Prefer AdjClose, fall back to Close.
	if a.benchmark != (asset.Asset{}) {
		v := df.Value(a.benchmark, data.AdjClose)
		if math.IsNaN(v) || v == 0 {
			v = df.Value(a.benchmark, data.MetricClose)
		}
		a.benchmarkPrices = append(a.benchmarkPrices, v)
	}

	// Track risk-free price series. Prefer AdjClose, fall back to Close.
	if a.riskFree != (asset.Asset{}) {
		v := df.Value(a.riskFree, data.AdjClose)
		if math.IsNaN(v) || v == 0 {
			v = df.Value(a.riskFree, data.MetricClose)
		}
		a.riskFreePrices = append(a.riskFreePrices, v)
	}

	// Build perfData in parallel with old fields.
	var benchVal, rfVal float64
	if a.benchmark != (asset.Asset{}) {
		benchVal = a.benchmarkPrices[len(a.benchmarkPrices)-1]
	}
	if a.riskFree != (asset.Asset{}) {
		rfVal = a.riskFreePrices[len(a.riskFreePrices)-1]
	}

	if a.perfData == nil {
		t := []time.Time{df.End()}
		assets := []asset.Asset{portfolioAsset}
		metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
		row, err := data.NewDataFrame(t, assets, metrics, []float64{total, benchVal, rfVal})
		if err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
			return
		}
		a.perfData = row
	} else {
		if err := a.perfData.AppendRow(df.End(), []float64{total, benchVal, rfVal}); err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
			return
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

// PerfData returns the accumulated performance DataFrame, or nil if no
// prices have been recorded yet.
func (a *Account) PerfData() *data.DataFrame { return a.perfData }

// TaxLots returns the current tax lot positions keyed by asset.
func (a *Account) TaxLots() map[asset.Asset][]TaxLot { return a.taxLots }

// Prices returns the most recent price DataFrame, or nil if no prices
// have been recorded yet.
func (a *Account) Prices() *data.DataFrame { return a.prices }

func (a *Account) SetBroker(b broker.Broker) { a.broker = b }

// HasBroker returns true if a broker has been set on the account.
func (a *Account) HasBroker() bool { return a.broker != nil }

// SetBenchmark sets the benchmark asset after construction.
func (a *Account) SetBenchmark(b asset.Asset) { a.benchmark = b }

// SetRiskFree sets the risk-free asset after construction.
func (a *Account) SetRiskFree(rf asset.Asset) { a.riskFree = rf }
