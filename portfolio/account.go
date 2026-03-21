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
	recentLossSales   map[asset.Asset][]recentLossSale
	recentBuys        map[asset.Asset][]recentBuy
	washSales         []WashSaleRecord
	metadata          map[string]string
	metrics           []MetricRow
	registeredMetrics []PerformanceMetric
	annotations       []Annotation
	middleware        []Middleware
	pendingOrders     map[string]broker.Order
	pendingGroups     map[string]*broker.OrderGroup // groupID -> group
	brokerHasGroups   bool                          // cached GroupSubmitter check
	deferredExits     map[string]OrderGroupSpec     // groupID -> bracket spec
	lotSelection      LotSelection
	substitutions     map[asset.Asset]Substitution
	excursions        map[asset.Asset]ExcursionRecord
	tradeDetails      []TradeDetail
}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	acct := &Account{
		holdings:        make(map[asset.Asset]float64),
		taxLots:         make(map[asset.Asset][]TaxLot),
		recentLossSales: make(map[asset.Asset][]recentLossSale),
		recentBuys:      make(map[asset.Asset][]recentBuy),
		metadata:        make(map[string]string),
		pendingOrders:   make(map[string]broker.Order),
		pendingGroups:   make(map[string]*broker.OrderGroup),
		deferredExits:   make(map[string]OrderGroupSpec),
		substitutions:   make(map[asset.Asset]Substitution),
		excursions:      make(map[asset.Asset]ExcursionRecord),
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

// WithDefaultLotSelection sets the lot selection method used for all sell
// transactions that do not carry a per-order override.
func WithDefaultLotSelection(method LotSelection) Option {
	return func(acct *Account) {
		acct.lotSelection = method
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
		// Filter out $CASH entries -- cash is the implicit remainder.
		filtered := make(map[asset.Asset]float64, len(alloc.Members))
		for memberAsset, weight := range alloc.Members {
			if memberAsset.Ticker != "$CASH" {
				filtered[memberAsset] = weight
			}
		}

		alloc = Allocation{
			Date:          alloc.Date,
			Members:       filtered,
			Justification: alloc.Justification,
		}

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
		case lotSelectionModifier:
			order.LotSelection = int(modifier.method)
		case bracketModifier:
			return fmt.Errorf("bracket/OCO modifiers require batch submission")
		case ocoModifier:
			return fmt.Errorf("bracket/OCO modifiers require batch submission")
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
				LotSelection:  LotSelection(order.LotSelection),
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
// asset and its held quantity. When active substitutions exist, real
// assets are mapped back to their logical originals so that strategy
// code sees the canonical asset names.
func (a *Account) Holdings(fn func(asset.Asset, float64)) {
	var asOf time.Time
	if a.prices != nil {
		asOf = a.prices.End()
	}

	// When substitutions are active, multiple real assets may map to the
	// same logical asset. Aggregate quantities under the logical key.
	// A zero asOf is safe here: time.Time{}.Before(expiry) is true for any
	// real expiry date, so substitutions remain active when prices are absent.
	if len(a.substitutions) > 0 {
		logical := make(map[asset.Asset]float64, len(a.holdings))
		for realAsset, qty := range a.holdings {
			key := mapToLogical(realAsset, a.substitutions, asOf)
			logical[key] += qty
		}

		for ast, qty := range logical {
			fn(ast, qty)
		}

		return
	}

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

	taxMetrics.TaxDrag, err = a.PerformanceMetric(TaxDragMetric).Value()
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

	tradeMetrics.MedianMFE, err = a.PerformanceMetric(MedianMFE).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.MedianMAE, err = a.PerformanceMetric(MedianMAE).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.EdgeRatio, err = a.PerformanceMetric(EdgeRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.TradeCaptureRatio, err = a.PerformanceMetric(TradeCaptureRatio).Value()
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
// and tax lots accordingly. It also performs wash sale detection on both
// buy and sell transactions.
func (a *Account) Record(txn Transaction) {
	if txn.Type == DividendTransaction {
		txn.Qualified = a.isDividendQualified(txn.Asset, txn.Date)
	}

	// Prune expired wash sale tracking entries.
	a.pruneWashSaleTracking(txn.Date)

	a.transactions = append(a.transactions, txn)
	a.cash += txn.Amount

	switch txn.Type {
	case BuyTransaction:
		a.holdings[txn.Asset] += txn.Qty

		lotID := fmt.Sprintf("lot-%d-%d", txn.Date.UnixNano(), len(a.taxLots[txn.Asset]))
		newLot := TaxLot{
			ID:    lotID,
			Date:  txn.Date,
			Qty:   txn.Qty,
			Price: txn.Price,
		}

		a.taxLots[txn.Asset] = append(a.taxLots[txn.Asset], newLot)

		// Check for wash sale: buy after a recent loss sale.
		a.checkWashSaleOnBuy(txn.Asset, txn.Date, txn.Qty, lotID)

		// Track this buy for the reverse direction.
		a.recentBuys[txn.Asset] = append(a.recentBuys[txn.Asset], recentBuy{
			date:  txn.Date,
			lotID: lotID,
			qty:   txn.Qty,
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

		method := txn.LotSelection
		if method == LotFIFO && a.lotSelection != LotFIFO {
			method = a.lotSelection
		}

		// Compute info about the lots to be consumed before actually
		// consuming them, so we can determine gain/loss.
		consumed := a.computeConsumedLotInfo(txn.Asset, txn.Qty, method)

		a.consumeLots(txn.Asset, txn.Qty, method)

		// Determine if this was a loss sale.
		lossPerShare := consumed.avgCostBasis - txn.Price
		if lossPerShare > 0 {
			// Check for wash sale: loss sale after a recent buy.
			// Only consider buys that occurred after the consumed lot's
			// purchase date (buys before the sold lot was purchased are
			// original holdings, not replacement purchases).
			disallowedQty := a.checkWashSaleOnSell(txn.Asset, txn.Date, txn.Qty, lossPerShare, consumed.latestBuyDate)

			// Track only the remaining (non-disallowed) quantity in
			// recentLossSales for the forward direction.
			remainingLossQty := txn.Qty - disallowedQty
			if remainingLossQty > 0 {
				a.recentLossSales[txn.Asset] = append(a.recentLossSales[txn.Asset], recentLossSale{
					date:         txn.Date,
					lossPerShare: lossPerShare,
					qty:          remainingLossQty,
				})
			}
		}

		if a.holdings[txn.Asset] == 0 {
			delete(a.holdings, txn.Asset)
			delete(a.taxLots, txn.Asset)
			delete(a.excursions, txn.Asset)
		}
	}
}

// WashSaleRecords returns all wash sale records detected during the
// account's lifetime. The returned slice is a copy; mutations do not
// affect the account's internal state.
func (a *Account) WashSaleRecords() []WashSaleRecord {
	result := make([]WashSaleRecord, len(a.washSales))
	copy(result, a.washSales)

	return result
}

// WashSaleWindow returns wash sale records for the given asset only.
// The returned slice is a copy; mutations do not affect internal state.
// This implements the TaxAware interface.
func (a *Account) WashSaleWindow(ast asset.Asset) []WashSaleRecord {
	var result []WashSaleRecord

	for _, rec := range a.washSales {
		if rec.Asset == ast {
			result = append(result, rec)
		}
	}

	return result
}

// UnrealizedLots returns a deep copy of the open tax lots for the given asset.
// The caller may mutate the returned slice without affecting internal state.
// This implements the TaxAware interface.
func (a *Account) UnrealizedLots(ast asset.Asset) []TaxLot {
	src := a.taxLots[ast]
	if len(src) == 0 {
		return nil
	}

	result := make([]TaxLot, len(src))
	copy(result, src)

	return result
}

// RealizedGainsYTD returns the total realized long-term and short-term
// capital gains across all transactions. The "YTD" label is aspirational:
// for a single-year backtest this equals year-to-date gains.
// This implements the TaxAware interface.
func (a *Account) RealizedGainsYTD() (ltcg, stcg float64) {
	ltcg, stcg, _, _ = realizedGains(a.Transactions())
	return ltcg, stcg
}

// RegisterSubstitution records that substitute is being held in place of
// original until the given expiry time. This implements the TaxAware interface.
func (a *Account) RegisterSubstitution(original, substitute asset.Asset, until time.Time) {
	a.substitutions[original] = Substitution{
		Original:   original,
		Substitute: substitute,
		Until:      until,
	}
}

// ActiveSubstitutions returns a copy of the substitution map containing only
// non-expired entries. A substitution is considered expired when its Until date
// is before the price end date. If no prices are loaded the cutoff is unknown
// and all entries are returned. This implements the TaxAware interface.
func (a *Account) ActiveSubstitutions() map[asset.Asset]Substitution {
	if len(a.substitutions) == 0 {
		return nil
	}

	// Return all substitutions. Callers that need expiry filtering (e.g.,
	// mapToLogical in ProjectedHoldings) perform their own timestamp check.
	// Returning all entries allows the TaxLossHarvester's swap-back logic
	// to detect recently-expired substitutions that need reversal.
	result := make(map[asset.Asset]Substitution, len(a.substitutions))
	for key, sub := range a.substitutions {
		result[key] = sub
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// pruneWashSaleTracking removes entries older than 30 days from the
// wash sale tracking maps.
func (a *Account) pruneWashSaleTracking(asOf time.Time) {
	cutoff := asOf.AddDate(0, 0, -washSaleWindowDays)

	for ast, sales := range a.recentLossSales {
		pruned := sales[:0]
		for _, sale := range sales {
			if sale.date.After(cutoff) || sale.date.Equal(cutoff) {
				pruned = append(pruned, sale)
			}
		}

		if len(pruned) == 0 {
			delete(a.recentLossSales, ast)
		} else {
			a.recentLossSales[ast] = pruned
		}
	}

	for ast, buys := range a.recentBuys {
		pruned := buys[:0]
		for _, buy := range buys {
			if buy.date.After(cutoff) || buy.date.Equal(cutoff) {
				pruned = append(pruned, buy)
			}
		}

		if len(pruned) == 0 {
			delete(a.recentBuys, ast)
		} else {
			a.recentBuys[ast] = pruned
		}
	}
}

// checkWashSaleOnBuy checks if there are recent loss sales within 30 days
// of this buy. If so, it disallows the loss by adding it to the new lot's
// cost basis.
func (a *Account) checkWashSaleOnBuy(ast asset.Asset, buyDate time.Time, buyQty float64, lotID string) {
	sales := a.recentLossSales[ast]
	if len(sales) == 0 {
		return
	}

	remaining := buyQty

	for idx := 0; idx < len(sales) && remaining > 0; idx++ {
		daysDiff := buyDate.Sub(sales[idx].date).Hours() / 24
		if daysDiff > float64(washSaleWindowDays) || daysDiff < 0 {
			continue
		}

		// Compute how many shares trigger the wash sale.
		matchQty := sales[idx].qty
		if matchQty > remaining {
			matchQty = remaining
		}

		disallowedLoss := matchQty * sales[idx].lossPerShare

		// Adjust the new lot's cost basis.
		a.adjustLotBasis(ast, lotID, sales[idx].lossPerShare)

		// Record the wash sale.
		a.washSales = append(a.washSales, WashSaleRecord{
			Asset:          ast,
			SellDate:       sales[idx].date,
			RebuyDate:      buyDate,
			DisallowedLoss: disallowedLoss,
			AdjustedLotID:  lotID,
		})

		// Consume from the loss sale entry.
		sales[idx].qty -= matchQty
		remaining -= matchQty
	}

	// Remove fully consumed entries.
	pruned := sales[:0]
	for _, sale := range sales {
		if sale.qty > 0 {
			pruned = append(pruned, sale)
		}
	}

	if len(pruned) == 0 {
		delete(a.recentLossSales, ast)
	} else {
		a.recentLossSales[ast] = pruned
	}
}

// checkWashSaleOnSell checks if there are recent buys within 30 days of
// this loss sale. If so, it adjusts the recent buy's lot cost basis.
// Only buys that occurred after consumedLotDate are considered as potential
// replacement purchases (buys before the sold lot's purchase are original
// holdings). It returns the quantity of shares whose loss was disallowed
// (so the caller can reduce the amount tracked in recentLossSales).
func (a *Account) checkWashSaleOnSell(ast asset.Asset, sellDate time.Time, sellQty, lossPerShare float64, consumedLotDate time.Time) float64 {
	buys := a.recentBuys[ast]
	if len(buys) == 0 {
		return 0
	}

	remaining := sellQty
	disallowedQty := 0.0

	for idx := 0; idx < len(buys) && remaining > 0; idx++ {
		daysDiff := sellDate.Sub(buys[idx].date).Hours() / 24
		if daysDiff > float64(washSaleWindowDays) || daysDiff < 0 {
			continue
		}

		// Only consider buys that occurred after the consumed lot's
		// purchase date. Earlier buys are original holdings.
		if !buys[idx].date.After(consumedLotDate) {
			continue
		}

		// Only adjust if the lot still exists in taxLots.
		if !a.lotExists(ast, buys[idx].lotID) {
			continue
		}

		matchQty := buys[idx].qty
		if matchQty > remaining {
			matchQty = remaining
		}

		disallowedLoss := matchQty * lossPerShare

		// When matchQty is less than the full lot size, split the lot so
		// that only the matched shares receive the basis adjustment. The
		// remainder keeps its original basis.
		adjustedLotID := buys[idx].lotID
		if matchQty < buys[idx].qty {
			headID, tailID := a.splitLot(ast, buys[idx].lotID, matchQty)
			adjustedLotID = headID

			// Update the recentBuy entry to refer to the tail lot for
			// any future matching against the leftover shares.
			buys[idx].lotID = tailID
		}

		// Adjust only the matched portion's cost basis.
		a.adjustLotBasis(ast, adjustedLotID, lossPerShare)

		// Record the wash sale.
		a.washSales = append(a.washSales, WashSaleRecord{
			Asset:          ast,
			SellDate:       sellDate,
			RebuyDate:      buys[idx].date,
			DisallowedLoss: disallowedLoss,
			AdjustedLotID:  adjustedLotID,
		})

		buys[idx].qty -= matchQty
		remaining -= matchQty
		disallowedQty += matchQty
	}

	// Remove fully consumed entries.
	pruned := buys[:0]
	for _, buy := range buys {
		if buy.qty > 0 {
			pruned = append(pruned, buy)
		}
	}

	if len(pruned) == 0 {
		delete(a.recentBuys, ast)
	} else {
		a.recentBuys[ast] = pruned
	}

	return disallowedQty
}

// lotExists returns true if a lot with the given ID exists in the
// account's tax lots for the specified asset.
func (a *Account) lotExists(ast asset.Asset, lotID string) bool {
	for _, lot := range a.taxLots[ast] {
		if lot.ID == lotID {
			return true
		}
	}

	return false
}

// adjustLotBasis adds the given per-share adjustment to the cost basis
// of the specified lot.
func (a *Account) adjustLotBasis(ast asset.Asset, lotID string, perShareAdjustment float64) {
	lots := a.taxLots[ast]
	for idx := range lots {
		if lots[idx].ID == lotID {
			lots[idx].Price += perShareAdjustment
			return
		}
	}
}

// splitLot splits a tax lot with the given ID into two lots: a head lot
// of headQty shares (which keeps the original lot ID) and a tail lot of
// the remaining shares (which gets a new derived ID). The head lot is
// returned first, then the tail. This is used to apply basis adjustments
// to only a portion of a lot's shares.
func (a *Account) splitLot(ast asset.Asset, lotID string, headQty float64) (headID, tailID string) {
	lots := a.taxLots[ast]
	for idx := range lots {
		if lots[idx].ID != lotID {
			continue
		}

		original := lots[idx]
		tailQty := original.Qty - headQty

		// Shrink the existing lot to headQty shares; it keeps the original ID.
		lots[idx].Qty = headQty
		headID = original.ID

		// Create a new lot for the remaining shares with a derived ID.
		tailID = fmt.Sprintf("%s-split", original.ID)
		tail := TaxLot{
			ID:    tailID,
			Date:  original.Date,
			Qty:   tailQty,
			Price: original.Price,
		}

		a.taxLots[ast] = append(lots, tail)

		return headID, tailID
	}

	return lotID, ""
}

// consumedLotInfo holds information about the lots that would be consumed
// by a sell operation, without actually consuming them.
type consumedLotInfo struct {
	avgCostBasis  float64
	latestBuyDate time.Time
}

// computeConsumedLotInfo computes the weighted average cost basis and the
// date range of lots that would be consumed by a sell of the given quantity
// using the specified lot selection method. This does NOT modify the lots.
func (a *Account) computeConsumedLotInfo(ast asset.Asset, qty float64, method LotSelection) consumedLotInfo {
	lots := a.taxLots[ast]
	if len(lots) == 0 || qty == 0 {
		return consumedLotInfo{}
	}

	// Make a working copy to avoid mutating the real lots.
	work := make([]TaxLot, len(lots))
	copy(work, lots)

	var totalCost, totalQty float64

	var latest time.Time

	accumulate := func(lot TaxLot, consumed float64) {
		totalCost += consumed * lot.Price
		totalQty += consumed

		if latest.IsZero() || lot.Date.After(latest) {
			latest = lot.Date
		}
	}

	switch method {
	case LotLIFO:
		remaining := qty

		for idx := len(work) - 1; idx >= 0 && remaining > 0; idx-- {
			consumed := work[idx].Qty
			if consumed > remaining {
				consumed = remaining
			}

			accumulate(work[idx], consumed)
			remaining -= consumed
		}

	case LotHighestCost:
		sortLotsByPriceDesc(work)

		remaining := qty

		for idx := 0; idx < len(work) && remaining > 0; idx++ {
			consumed := work[idx].Qty
			if consumed > remaining {
				consumed = remaining
			}

			accumulate(work[idx], consumed)
			remaining -= consumed
		}

	default: // LotFIFO
		remaining := qty

		for idx := 0; idx < len(work) && remaining > 0; idx++ {
			consumed := work[idx].Qty
			if consumed > remaining {
				consumed = remaining
			}

			accumulate(work[idx], consumed)
			remaining -= consumed
		}
	}

	if totalQty == 0 {
		return consumedLotInfo{}
	}

	return consumedLotInfo{
		avgCostBasis:  totalCost / totalQty,
		latestBuyDate: latest,
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

// consumeLots removes qty shares from the tax lot list for ast using the
// specified lot selection method. The caller is responsible for updating
// holdings and cleaning up empty map entries.
func (a *Account) consumeLots(ast asset.Asset, qty float64, method LotSelection) {
	lots := a.taxLots[ast]

	switch method {
	case LotLIFO:
		// Consume from the back of the slice (most recently acquired first).
		remaining := qty

		end := len(lots)

		for end > 0 && remaining > 0 {
			idx := end - 1
			if lots[idx].Qty <= remaining {
				remaining -= lots[idx].Qty
				end--
			} else {
				lots[idx].Qty -= remaining
				remaining = 0
			}
		}

		a.taxLots[ast] = lots[:end]

	case LotHighestCost:
		// Sort a copy by price descending, consume highest first, then
		// restore the remaining lots to date-ascending order.
		sortLotsByPriceDesc(lots)

		remaining := qty

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

		remainingLots := lots[lotIdx:]
		sortLotsByDateAsc(remainingLots)

		a.taxLots[ast] = remainingLots

	default: // LotFIFO
		// Consume from the front of the slice (earliest acquired first).
		remaining := qty

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

		a.taxLots[ast] = lots[lotIdx:]
	}
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

func (a *Account) SetBroker(b broker.Broker) {
	a.broker = b
	_, a.brokerHasGroups = b.(broker.GroupSubmitter)
}

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

	// 4. Assign IDs to all orders and add to pendingOrders. Collect orders by groupID.
	groupOrders := make(map[string][]broker.Order) // groupID -> orders in that group

	for idx := range batch.Orders {
		order := &batch.Orders[idx]
		if order.ID == "" {
			order.ID = fmt.Sprintf("batch-%d-%d", batch.Timestamp.UnixNano(), idx)
		}

		a.pendingOrders[order.ID] = *order

		if order.GroupID != "" {
			groupOrders[order.GroupID] = append(groupOrders[order.GroupID], *order)
		}
	}

	// 5. Store deferred bracket exits: for each GroupBracket spec, record the spec in
	// deferredExits keyed by groupID so exit orders can be submitted after the entry fills.
	for _, spec := range batch.Groups() {
		if spec.Type == broker.GroupBracket {
			a.deferredExits[spec.GroupID] = spec
		}
	}

	// 6. Submit grouped orders.
	submittedGroups := make(map[string]bool) // track which groupIDs have been handled

	for _, spec := range batch.Groups() {
		orders := groupOrders[spec.GroupID]

		switch spec.Type {
		case broker.GroupOCO:
			// Standalone OCO: submit as a group if broker supports it, else individually.
			if a.brokerHasGroups {
				gs := a.broker.(broker.GroupSubmitter)
				if err := gs.SubmitGroup(ctx, orders, broker.GroupOCO); err != nil {
					return fmt.Errorf("execute batch: submit group %s: %w", spec.GroupID, err)
				}

				group := &broker.OrderGroup{ID: spec.GroupID, Type: broker.GroupOCO}
				for _, ord := range orders {
					group.OrderIDs = append(group.OrderIDs, ord.ID)
				}

				a.pendingGroups[spec.GroupID] = group
			} else {
				for _, ord := range orders {
					if err := a.broker.Submit(ctx, ord); err != nil {
						return fmt.Errorf("execute batch: submit %s: %w", ord.Asset.Ticker, err)
					}
				}
			}

		case broker.GroupBracket:
			// For brackets, submit only the entry order now; exits are deferred.
			if spec.EntryIndex >= 0 && spec.EntryIndex < len(batch.Orders) {
				entryOrder := batch.Orders[spec.EntryIndex]
				if err := a.broker.Submit(ctx, entryOrder); err != nil {
					return fmt.Errorf("execute batch: submit %s: %w", entryOrder.Asset.Ticker, err)
				}

				group := &broker.OrderGroup{ID: spec.GroupID, Type: broker.GroupBracket, OrderIDs: []string{entryOrder.ID}}
				a.pendingGroups[spec.GroupID] = group
			}
		}

		submittedGroups[spec.GroupID] = true
	}

	// 7. Submit remaining non-group orders individually.
	for idx := range batch.Orders {
		order := &batch.Orders[idx]
		if order.GroupID != "" && submittedGroups[order.GroupID] {
			// Already handled as part of a group.
			continue
		}

		if err := a.broker.Submit(ctx, *order); err != nil {
			return fmt.Errorf("execute batch: submit %s: %w", order.Asset.Ticker, err)
		}
	}

	// 8. Drain immediate fills.
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
				LotSelection:  LotSelection(order.LotSelection),
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

	pendingGroups := make(map[string]*broker.OrderGroup, len(acct.pendingGroups))
	for groupID, group := range acct.pendingGroups {
		groupCopy := *group
		groupCopy.OrderIDs = make([]string, len(group.OrderIDs))
		copy(groupCopy.OrderIDs, group.OrderIDs)
		pendingGroups[groupID] = &groupCopy
	}

	deferredExits := make(map[string]OrderGroupSpec, len(acct.deferredExits))
	for groupID, spec := range acct.deferredExits {
		deferredExits[groupID] = spec
	}

	recentLossSales := make(map[asset.Asset][]recentLossSale, len(acct.recentLossSales))
	for ast, sales := range acct.recentLossSales {
		salesCopy := make([]recentLossSale, len(sales))
		copy(salesCopy, sales)
		recentLossSales[ast] = salesCopy
	}

	recentBuys := make(map[asset.Asset][]recentBuy, len(acct.recentBuys))
	for ast, buys := range acct.recentBuys {
		buysCopy := make([]recentBuy, len(buys))
		copy(buysCopy, buys)
		recentBuys[ast] = buysCopy
	}

	washSales := make([]WashSaleRecord, len(acct.washSales))
	copy(washSales, acct.washSales)

	substitutions := make(map[asset.Asset]Substitution, len(acct.substitutions))
	for key, sub := range acct.substitutions {
		substitutions[key] = sub
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
		recentLossSales:   recentLossSales,
		recentBuys:        recentBuys,
		washSales:         washSales,
		lotSelection:      acct.lotSelection,
		substitutions:     substitutions,
		metadata:          metadata,
		metrics:           acct.metrics,
		registeredMetrics: acct.registeredMetrics,
		annotations:       annotations,
		middleware:        acct.middleware,
		pendingOrders:     pendingOrders,
		pendingGroups:     pendingGroups,
		brokerHasGroups:   acct.brokerHasGroups,
		deferredExits:     deferredExits,
		excursions:        excursions,
		tradeDetails:      tradeDetailsCopy,
	}

	if acct.perfData != nil {
		clone.perfData = acct.perfData.Copy()
	}

	return clone
}
