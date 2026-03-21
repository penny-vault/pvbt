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
	cash              float64
	holdings          map[asset.Asset]float64
	transactions      []Transaction
	broker            broker.Broker
	prices            *data.DataFrame
	perfData          *data.DataFrame
	benchmark         asset.Asset
	riskFreeValue     float64
	taxLots           map[asset.Asset][]TaxLot
	metadata          map[string]string
	metrics           []MetricRow
	registeredMetrics []PerformanceMetric
	annotations       []Annotation
	middleware        []Middleware
	pendingOrders     map[string]broker.Order
	excursions        map[asset.Asset]ExcursionRecord
	tradeDetails      []TradeDetail
}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	acct := &Account{
		holdings:      make(map[asset.Asset]float64),
		taxLots:       make(map[asset.Asset][]TaxLot),
		metadata:      make(map[string]string),
		pendingOrders: make(map[string]broker.Order),
		excursions:    make(map[asset.Asset]ExcursionRecord),
	}
	for _, opt := range opts {
		opt(acct)
	}

	// Default: register all metrics if none were explicitly specified.
	if len(acct.registeredMetrics) == 0 {
		WithAllMetrics()(acct)
	}

	return acct
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

// Benchmark returns the benchmark asset.
func (a *Account) Benchmark() asset.Asset {
	return a.benchmark
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
		for _, sellOrder := range sells {
			order := broker.Order{
				Asset:       sellOrder.asset,
				Side:        broker.Sell,
				Qty:         sellOrder.qty,
				Amount:      sellOrder.amount,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			if err := a.submitAndRecord(ctx, sellOrder.asset, Sell, order, alloc.Justification); err != nil {
				return fmt.Errorf("RebalanceTo: sell %s: %w", sellOrder.asset.Ticker, err)
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

		for _, buyOrder := range buys {
			order := broker.Order{
				Asset:       buyOrder.asset,
				Side:        broker.Buy,
				Amount:      buyOrder.amount,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			if err := a.submitAndRecord(ctx, buyOrder.asset, Buy, order, alloc.Justification); err != nil {
				return fmt.Errorf("RebalanceTo: buy %s: %w", buyOrder.asset.Ticker, err)
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
	var (
		hasLimit, hasStop bool
		justification     string
	)

	for _, mod := range mods {
		switch modifier := mod.(type) {
		case limitModifier:
			order.LimitPrice = modifier.price
			hasLimit = true
		case stopModifier:
			order.StopPrice = modifier.price
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
			order.GTDDate = modifier.date
		case justificationModifier:
			justification = modifier.reason
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

	return a.submitAndRecord(ctx, ast, side, order, justification)
}

// submitAndRecord sends an order to the broker and records each fill
// as a transaction. Used by both Order and RebalanceTo.
//
// After Submit returns, any fills already in the broker's channel are
// drained immediately (non-blocking). This is a temporary bridge until
// Task 4 introduces the full DrainFills/ExecuteBatch infrastructure.
func (a *Account) submitAndRecord(ctx context.Context, ast asset.Asset, side Side, order broker.Order, justification string) error {
	if err := a.broker.Submit(ctx, order); err != nil {
		return fmt.Errorf("order %s (qty=%.2f, amount=%.2f): %w", ast.Ticker, order.Qty, order.Amount, err)
	}

	fillCh := a.broker.Fills()

	for {
		select {
		case fill := <-fillCh:
			var (
				txType TransactionType
				amount float64
			)

			switch side {
			case Buy:
				txType = BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case Sell:
				txType = SellTransaction
				amount = fill.Price * fill.Qty
			}

			a.Record(Transaction{
				Date:          fill.FilledAt,
				Asset:         ast,
				Type:          txType,
				Qty:           fill.Qty,
				Price:         fill.Price,
				Amount:        amount,
				Justification: justification,
			})
		default:
			return nil
		}
	}
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

	summary := Summary{}

	var err error

	summary.TWRR, err = a.PerformanceMetric(TWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.MWRR, err = a.PerformanceMetric(MWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Sharpe, err = a.PerformanceMetric(Sharpe).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Sortino, err = a.PerformanceMetric(Sortino).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Calmar, err = a.PerformanceMetric(Calmar).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.MaxDrawdown, err = a.PerformanceMetric(MaxDrawdown).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.StdDev, err = a.PerformanceMetric(StdDev).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return summary, errors.Join(errs...)
}

func (a *Account) RiskMetrics() (RiskMetrics, error) {
	var errs []error

	risk := RiskMetrics{}

	var err error

	risk.Beta, err = a.PerformanceMetric(Beta).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.Alpha, err = a.PerformanceMetric(Alpha).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.TrackingError, err = a.PerformanceMetric(TrackingError).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.DownsideDeviation, err = a.PerformanceMetric(DownsideDeviation).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.InformationRatio, err = a.PerformanceMetric(InformationRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.Treynor, err = a.PerformanceMetric(Treynor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.UlcerIndex, err = a.PerformanceMetric(UlcerIndex).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.ExcessKurtosis, err = a.PerformanceMetric(ExcessKurtosis).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.Skewness, err = a.PerformanceMetric(Skewness).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.RSquared, err = a.PerformanceMetric(RSquared).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.ValueAtRisk, err = a.PerformanceMetric(ValueAtRisk).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.UpsideCaptureRatio, err = a.PerformanceMetric(UpsideCaptureRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.DownsideCaptureRatio, err = a.PerformanceMetric(DownsideCaptureRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return risk, errors.Join(errs...)
}

func (a *Account) TaxMetrics() (TaxMetrics, error) {
	var errs []error

	taxMetrics := TaxMetrics{}

	var err error

	taxMetrics.LTCG, err = a.PerformanceMetric(LTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.STCG, err = a.PerformanceMetric(STCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.UnrealizedLTCG, err = a.PerformanceMetric(UnrealizedLTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.UnrealizedSTCG, err = a.PerformanceMetric(UnrealizedSTCGMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.QualifiedDividends, err = a.PerformanceMetric(QualifiedDividendsMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.NonQualifiedIncome, err = a.PerformanceMetric(NonQualifiedIncomeMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	taxMetrics.TaxCostRatio, err = a.PerformanceMetric(TaxCostRatioMetric).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return taxMetrics, errors.Join(errs...)
}

func (a *Account) TradeMetrics() (TradeMetrics, error) {
	var errs []error

	tradeMetrics := TradeMetrics{}

	var err error

	tradeMetrics.WinRate, err = a.PerformanceMetric(WinRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.AverageWin, err = a.PerformanceMetric(AverageWin).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.AverageLoss, err = a.PerformanceMetric(AverageLoss).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.ProfitFactor, err = a.PerformanceMetric(ProfitFactor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.AverageHoldingPeriod, err = a.PerformanceMetric(AverageHoldingPeriod).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.Turnover, err = a.PerformanceMetric(Turnover).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.NPositivePeriods, err = a.PerformanceMetric(NPositivePeriods).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.GainLossRatio, err = a.PerformanceMetric(TradeGainLossRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.AverageMFE, err = a.PerformanceMetric(AverageMFE).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.AverageMAE, err = a.PerformanceMetric(AverageMAE).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return tradeMetrics, errors.Join(errs...)
}

func (a *Account) WithdrawalMetrics() (WithdrawalMetrics, error) {
	var errs []error

	withdrawal := WithdrawalMetrics{}

	var err error

	withdrawal.SafeWithdrawalRate, err = a.PerformanceMetric(SafeWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	withdrawal.PerpetualWithdrawalRate, err = a.PerformanceMetric(PerpetualWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	withdrawal.DynamicWithdrawalRate, err = a.PerformanceMetric(DynamicWithdrawalRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return withdrawal, errors.Join(errs...)
}

// --- PortfolioManager interface ---

// Record appends a transaction to the log and updates cash, holdings,
// and tax lots accordingly.
func (a *Account) Record(txn Transaction) {
	if txn.Type == DividendTransaction {
		txn.Qualified = a.isDividendQualified(txn.Asset, txn.Date)
	}

	a.transactions = append(a.transactions, txn)
	a.cash += txn.Amount

	switch txn.Type {
	case BuyTransaction:
		a.holdings[txn.Asset] += txn.Qty
		a.taxLots[txn.Asset] = append(a.taxLots[txn.Asset], TaxLot{
			Date:  txn.Date,
			Qty:   txn.Qty,
			Price: txn.Price,
		})

		if _, exists := a.excursions[txn.Asset]; !exists {
			a.excursions[txn.Asset] = ExcursionRecord{
				EntryPrice: txn.Price,
				HighPrice:  txn.Price,
				LowPrice:   txn.Price,
			}
		}
	case SellTransaction:
		a.holdings[txn.Asset] -= txn.Qty

		// Produce TradeDetail entries from excursion data.
		if excursion, hasExcursion := a.excursions[txn.Asset]; hasExcursion {
			mfe := (excursion.HighPrice - excursion.EntryPrice) / excursion.EntryPrice
			mae := (excursion.LowPrice - excursion.EntryPrice) / excursion.EntryPrice

			// Match against tax lots FIFO to get entry dates and per-lot qty.
			tdRemaining := txn.Qty
			tdLots := a.taxLots[txn.Asset]
			for tdLotIdx := 0; tdLotIdx < len(tdLots) && tdRemaining > 0; tdLotIdx++ {
				matched := tdLots[tdLotIdx].Qty
				if matched > tdRemaining {
					matched = tdRemaining
				}

				a.tradeDetails = append(a.tradeDetails, TradeDetail{
					Asset:      txn.Asset,
					EntryDate:  tdLots[tdLotIdx].Date,
					ExitDate:   txn.Date,
					EntryPrice: tdLots[tdLotIdx].Price,
					ExitPrice:  txn.Price,
					Qty:        matched,
					PnL:        (txn.Price - tdLots[tdLotIdx].Price) * matched,
					HoldDays:   txn.Date.Sub(tdLots[tdLotIdx].Date).Hours() / 24.0,
					MFE:        mfe,
					MAE:        mae,
				})

				tdRemaining -= matched
			}
		}

		remaining := txn.Qty
		lots := a.taxLots[txn.Asset]

		lotIdx := 0
		for lotIdx < len(lots) && remaining > 0 {
			if lots[lotIdx].Qty <= remaining {
				remaining -= lots[lotIdx].Qty
				lotIdx++
			} else {
				lots[lotIdx].Qty -= remaining
				remaining = 0
			}
		}

		a.taxLots[txn.Asset] = lots[lotIdx:]
		if a.holdings[txn.Asset] == 0 {
			delete(a.holdings, txn.Asset)
			delete(a.taxLots, txn.Asset)
			delete(a.excursions, txn.Asset)
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
// portfolio value, and appends it to the perfData DataFrame. It also tracks
// benchmark and risk-free price series when those assets are configured.
func (a *Account) UpdatePrices(priceData *data.DataFrame) {
	if priceData.Len() == 0 {
		return
	}

	a.prices = priceData

	total := a.cash
	for ast, qty := range a.holdings {
		v := priceData.Value(ast, data.MetricClose)
		if !math.IsNaN(v) {
			total += qty * v
		}
	}

	var benchVal, rfVal float64

	if a.benchmark != (asset.Asset{}) {
		v := priceData.Value(a.benchmark, data.AdjClose)
		if math.IsNaN(v) || v == 0 {
			v = priceData.Value(a.benchmark, data.MetricClose)
		}

		benchVal = v
	}

	rfVal = a.riskFreeValue

	if a.perfData == nil {
		timestamps := []time.Time{priceData.End()}
		assets := []asset.Asset{portfolioAsset}
		metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}

		row, err := data.NewDataFrame(timestamps, assets, metrics, data.Daily, []float64{total, benchVal, rfVal})
		if err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to create perfData")
			return
		}

		a.perfData = row
	} else {
		if err := a.perfData.AppendRow(priceData.End(), []float64{total, benchVal, rfVal}); err != nil {
			log.Error().Err(err).Msg("UpdatePrices: failed to append to perfData")
			return
		}
	}
}

// PerfData returns the accumulated performance DataFrame, or nil if no
// prices have been recorded yet.
func (a *Account) PerfData() *data.DataFrame { return a.perfData }

// TaxLots returns the current tax lot positions keyed by asset.
func (a *Account) TaxLots() map[asset.Asset][]TaxLot { return a.taxLots }

// Excursions returns the current excursion records keyed by asset.
func (a *Account) Excursions() map[asset.Asset]ExcursionRecord { return a.excursions }

// TradeDetails returns all completed round-trip trades with per-trade
// MFE and MAE excursion data.
func (a *Account) TradeDetails() []TradeDetail { return a.tradeDetails }

// Prices returns the most recent price DataFrame, or nil if no prices
// have been recorded yet.
func (a *Account) Prices() *data.DataFrame { return a.prices }

func (a *Account) SetBroker(b broker.Broker) { a.broker = b }

// HasBroker returns true if a broker has been set on the account.
func (a *Account) HasBroker() bool { return a.broker != nil }

// SetBenchmark sets the benchmark asset after construction.
func (a *Account) SetBenchmark(b asset.Asset) { a.benchmark = b }

// SetRiskFreeValue sets the risk-free cumulative value for the next
// UpdatePrices call. The engine calls this with a yield-derived
// cumulative series; test code can pass price-like values directly.
func (a *Account) SetRiskFreeValue(v float64) {
	a.riskFreeValue = v
}

// Annotate records a key-value annotation for the given timestamp.
// If an entry with the same timestamp and key already exists, its
// value is overwritten (last-write-wins).
func (a *Account) Annotate(timestamp time.Time, key, value string) {
	for idx := range a.annotations {
		if a.annotations[idx].Timestamp.Equal(timestamp) && a.annotations[idx].Key == key {
			a.annotations[idx].Value = value
			return
		}
	}

	a.annotations = append(a.annotations, Annotation{
		Timestamp: timestamp,
		Key:       key,
		Value:     value,
	})
}

// Annotations returns the full annotation log in the order entries
// were recorded.
func (a *Account) Annotations() []Annotation {
	return a.annotations
}

// Use appends one or more middleware to the processing chain.
func (a *Account) Use(middleware ...Middleware) {
	a.middleware = append(a.middleware, middleware...)
}

// NewBatch creates an empty Batch for the given timestamp, bound to this
// account for position and price queries.
func (a *Account) NewBatch(timestamp time.Time) *Batch {
	return NewBatch(timestamp, a)
}

// ExecuteBatch runs the middleware chain, records annotations, assigns
// order IDs, submits orders to the broker, and drains immediate fills.
func (a *Account) ExecuteBatch(ctx context.Context, batch *Batch) error {
	// 1. Run middleware chain.
	for _, mw := range a.middleware {
		if err := mw.Process(ctx, batch); err != nil {
			return err
		}
	}

	// 2. Record annotations.
	for key, value := range batch.Annotations {
		a.Annotate(batch.Timestamp, key, value)
	}

	// 3. Only submit if there are orders and a broker is set.
	if len(batch.Orders) > 0 && a.broker == nil {
		return fmt.Errorf("execute batch: no broker set")
	}

	// 4. Assign IDs, track, and submit orders.
	for idx := range batch.Orders {
		order := &batch.Orders[idx]
		if order.ID == "" {
			order.ID = fmt.Sprintf("batch-%d-%d", batch.Timestamp.UnixNano(), idx)
		}

		a.pendingOrders[order.ID] = *order

		if err := a.broker.Submit(ctx, *order); err != nil {
			return fmt.Errorf("execute batch: submit %s: %w", order.Asset.Ticker, err)
		}
	}

	// 5. Drain immediate fills.
	a.drainFillsFromChannel()

	return nil
}

// DrainFills drains any pending fills from the broker's fill channel
// and records them as transactions.
func (a *Account) DrainFills(_ context.Context) error {
	a.drainFillsFromChannel()
	return nil
}

// drainFillsFromChannel reads all available fills from the broker's
// fill channel (non-blocking) and records each as a transaction.
func (a *Account) drainFillsFromChannel() {
	if a.broker == nil {
		return
	}

	fillCh := a.broker.Fills()

	for {
		select {
		case fill := <-fillCh:
			order, ok := a.pendingOrders[fill.OrderID]
			if !ok {
				log.Warn().Str("orderID", fill.OrderID).Msg("received fill for unknown order")
				continue
			}

			var (
				txType TransactionType
				amount float64
			)

			switch order.Side {
			case broker.Buy:
				txType = BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case broker.Sell:
				txType = SellTransaction
				amount = fill.Price * fill.Qty
			}

			a.Record(Transaction{
				Date:          fill.FilledAt,
				Asset:         order.Asset,
				Type:          txType,
				Qty:           fill.Qty,
				Price:         fill.Price,
				Amount:        amount,
				Justification: order.Justification,
			})

			delete(a.pendingOrders, fill.OrderID)
		default:
			return
		}
	}
}

// CancelOpenOrders cancels all open or submitted orders and removes
// them from the pending-orders tracker.
func (a *Account) CancelOpenOrders(ctx context.Context) error {
	if a.broker == nil {
		return nil
	}

	orders, err := a.broker.Orders(ctx)
	if err != nil {
		return fmt.Errorf("cancel open orders: %w", err)
	}

	for _, order := range orders {
		if order.Status == broker.OrderOpen || order.Status == broker.OrderSubmitted {
			if cancelErr := a.broker.Cancel(ctx, order.ID); cancelErr != nil {
				return fmt.Errorf("cancel order %s: %w", order.ID, cancelErr)
			}

			delete(a.pendingOrders, order.ID)
		}
	}

	return nil
}

// Clone returns a deep copy of the Account suitable for prediction runs.
// Holdings, metadata, and annotations are independent copies. PerfData is
// deep-copied via DataFrame.Copy. Transactions and tax lots are shallow-copied
// (the clone gets its own slice header but shares the underlying elements,
// which is safe since appending to the clone's slice does not affect the
// original).
func (acct *Account) Clone() *Account {
	holdings := make(map[asset.Asset]float64, len(acct.holdings))
	for held, qty := range acct.holdings {
		holdings[held] = qty
	}

	metadata := make(map[string]string, len(acct.metadata))
	for key, val := range acct.metadata {
		metadata[key] = val
	}

	annotations := make([]Annotation, len(acct.annotations))
	copy(annotations, acct.annotations)

	transactions := make([]Transaction, len(acct.transactions))
	copy(transactions, acct.transactions)

	taxLots := make(map[asset.Asset][]TaxLot, len(acct.taxLots))
	for held, lots := range acct.taxLots {
		lotsCopy := make([]TaxLot, len(lots))
		copy(lotsCopy, lots)
		taxLots[held] = lotsCopy
	}

	pendingOrders := make(map[string]broker.Order, len(acct.pendingOrders))
	for orderID, order := range acct.pendingOrders {
		pendingOrders[orderID] = order
	}

	excursions := make(map[asset.Asset]ExcursionRecord, len(acct.excursions))
	for held, rec := range acct.excursions {
		excursions[held] = rec
	}

	tradeDetailsCopy := make([]TradeDetail, len(acct.tradeDetails))
	copy(tradeDetailsCopy, acct.tradeDetails)

	clone := &Account{
		cash:              acct.cash,
		holdings:          holdings,
		transactions:      transactions,
		broker:            acct.broker,
		prices:            acct.prices,
		benchmark:         acct.benchmark,
		riskFreeValue:     acct.riskFreeValue,
		taxLots:           taxLots,
		metadata:          metadata,
		metrics:           acct.metrics,
		registeredMetrics: acct.registeredMetrics,
		annotations:       annotations,
		middleware:        acct.middleware,
		pendingOrders:     pendingOrders,
		excursions:        excursions,
		tradeDetails:      tradeDetailsCopy,
	}

	if acct.perfData != nil {
		clone.perfData = acct.perfData.Copy()
	}

	return clone
}
