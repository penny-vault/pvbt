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

// batchRecord captures the timestamp of a single ExecuteBatch call.
// The index in Account.batches plus one is the batch id.
type batchRecord struct {
	BatchID   int
	Timestamp time.Time
}

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
	shortLots         map[asset.Asset][]TaxLot
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
	initialMargin     float64
	maintenanceMargin float64
	borrowRate        float64
	dfCache           map[dfCacheKey]*data.DataFrame
	seenTransactions  map[string]struct{}
	batches           []batchRecord
	currentBatchID    int
}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	acct := &Account{
		holdings:         make(map[asset.Asset]float64),
		taxLots:          make(map[asset.Asset][]TaxLot),
		shortLots:        make(map[asset.Asset][]TaxLot),
		recentLossSales:  make(map[asset.Asset][]recentLossSale),
		recentBuys:       make(map[asset.Asset][]recentBuy),
		metadata:         make(map[string]string),
		pendingOrders:    make(map[string]broker.Order),
		pendingGroups:    make(map[string]*broker.OrderGroup),
		deferredExits:    make(map[string]OrderGroupSpec),
		substitutions:    make(map[asset.Asset]Substitution),
		excursions:       make(map[asset.Asset]ExcursionRecord),
		seenTransactions: make(map[string]struct{}),
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
			Type:   asset.DepositTransaction,
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

		// Liquidate all positions not in the target allocation.
		// Long positions are sold; short positions are covered (bought back).
		var coverBuys []pendingOrder

		for ast, qty := range a.holdings {
			if _, ok := alloc.Members[ast]; !ok && qty != 0 {
				if qty > 0 {
					sells = append(sells, pendingOrder{asset: ast, side: Sell, qty: qty})
				} else {
					coverBuys = append(coverBuys, pendingOrder{asset: ast, side: Buy, qty: math.Abs(qty)})
				}
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

		// Cover short positions not in the target allocation.
		for _, coverOrder := range coverBuys {
			order := broker.Order{
				Asset:       coverOrder.asset,
				Side:        broker.Buy,
				Qty:         coverOrder.qty,
				OrderType:   broker.Market,
				TimeInForce: broker.Day,
			}
			if err := a.submitAndRecord(ctx, coverOrder.asset, Buy, order, alloc.Justification); err != nil {
				return fmt.Errorf("RebalanceTo: cover %s: %w", coverOrder.asset.Ticker, err)
			}
		}

		// Recompute target values after sells/covers so buys use actual
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
				txType asset.TransactionType
				amount float64
			)

			switch side {
			case Buy:
				txType = asset.BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case Sell:
				txType = asset.SellTransaction
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

// Holdings returns a map of all current positions keyed by asset with the
// held quantity as the value. When active substitutions exist, real assets
// are mapped back to their logical originals so that strategy code sees
// the canonical asset names.
func (a *Account) Holdings() map[asset.Asset]float64 {
	if len(a.substitutions) == 0 {
		result := make(map[asset.Asset]float64, len(a.holdings))
		for ast, qty := range a.holdings {
			result[ast] = qty
		}

		return result
	}

	var asOf time.Time
	if a.prices != nil {
		asOf = a.prices.End()
	}

	logical := make(map[asset.Asset]float64, len(a.holdings))
	for realAsset, qty := range a.holdings {
		key := mapToLogical(realAsset, a.substitutions, asOf)
		logical[key] += qty
	}

	return logical
}

// Transactions returns the full transaction log in chronological order.
func (a *Account) Transactions() []Transaction {
	return a.transactions
}

func (a *Account) PerformanceMetric(m PerformanceMetric) PerformanceMetricQuery {
	return PerformanceMetricQuery{account: a, metric: m}
}

// View returns a read-only Portfolio restricted to the date range
// [start, end]. Metrics computed on the view use only data within
// this range.
func (a *Account) View(start, end time.Time) Portfolio {
	return &viewedPortfolio{
		acct:  a,
		stats: newWindowedStats(a, start, end),
		start: start,
		end:   end,
	}
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

	tradeMetrics.LongWinRate, err = a.PerformanceMetric(LongWinRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.ShortWinRate, err = a.PerformanceMetric(ShortWinRate).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.LongProfitFactor, err = a.PerformanceMetric(LongProfitFactor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	tradeMetrics.ShortProfitFactor, err = a.PerformanceMetric(ShortProfitFactor).Value()
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
	if txn.Type == asset.DividendTransaction {
		txn.Qualified = a.isDividendQualified(txn.Asset, txn.Date)
	}

	// Prune expired wash sale tracking entries.
	a.pruneWashSaleTracking(txn.Date)

	a.transactions = append(a.transactions, txn)

	if txn.ID != "" {
		a.seenTransactions[txn.ID] = struct{}{}
	}

	a.cash += txn.Amount

	switch txn.Type {
	case asset.BuyTransaction:
		a.holdings[txn.Asset] += txn.Qty

		// Phase 1: Cover short lots (if any exist).
		shortLots := a.shortLots[txn.Asset]

		shortQty := 0.0
		for _, lot := range shortLots {
			shortQty += lot.Qty
		}

		coverQty := txn.Qty
		if coverQty > shortQty {
			coverQty = shortQty
		}

		if coverQty > 0 {
			method := txn.LotSelection
			if method == LotFIFO && a.lotSelection != LotFIFO {
				method = a.lotSelection
			}

			// Generate TradeDetail entries for the short cover.
			if excursion, hasExcursion := a.excursions[txn.Asset]; hasExcursion {
				// For shorts: MFE is entry-low (price fell), MAE is entry-high (price rose)
				mfe := (excursion.EntryPrice - excursion.LowPrice) / excursion.EntryPrice
				mae := (excursion.EntryPrice - excursion.HighPrice) / excursion.EntryPrice

				tdRemaining := coverQty

				tdLots := shortLots
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
						PnL:        (tdLots[tdLotIdx].Price - txn.Price) * matched,
						HoldDays:   txn.Date.Sub(tdLots[tdLotIdx].Date).Hours() / 24.0,
						MFE:        mfe,
						MAE:        mae,
						Direction:  TradeShort,
					})

					tdRemaining -= matched
				}
			}

			// Compute average short entry price BEFORE consuming lots.
			avgShortEntry := a.avgShortEntryPrice(txn.Asset, coverQty, method)

			// Consume short lots.
			a.consumeShortLots(txn.Asset, coverQty, method)

			// Wash sale check: covering a short at a loss.
			lossPerShare := txn.Price - avgShortEntry
			if lossPerShare > 0 {
				a.recentLossSales[txn.Asset] = append(a.recentLossSales[txn.Asset], recentLossSale{
					date:         txn.Date,
					lossPerShare: lossPerShare,
					qty:          coverQty,
				})
			}

			// If all short lots were consumed, remove the stale short excursion
			// so Phase 2 creates a fresh one for the long position.
			if len(a.shortLots[txn.Asset]) == 0 {
				delete(a.excursions, txn.Asset)
			}
		}

		// Phase 2: Create long lots for the remainder.
		longQty := txn.Qty - coverQty
		if longQty > 0 {
			lotID := fmt.Sprintf("lot-%d-%d", txn.Date.UnixNano(), len(a.taxLots[txn.Asset]))
			newLot := TaxLot{
				ID:    lotID,
				Date:  txn.Date,
				Qty:   longQty,
				Price: txn.Price,
			}

			a.taxLots[txn.Asset] = append(a.taxLots[txn.Asset], newLot)
			a.checkWashSaleOnBuy(txn.Asset, txn.Date, longQty, lotID)
			a.recentBuys[txn.Asset] = append(a.recentBuys[txn.Asset], recentBuy{
				date:  txn.Date,
				lotID: lotID,
				qty:   longQty,
			})

			if _, exists := a.excursions[txn.Asset]; !exists {
				a.excursions[txn.Asset] = ExcursionRecord{
					EntryPrice: txn.Price,
					HighPrice:  txn.Price,
					LowPrice:   txn.Price,
				}
			}
		}

		// Cleanup: remove tracking when fully flat.
		if a.holdings[txn.Asset] == 0 {
			delete(a.holdings, txn.Asset)
			delete(a.taxLots, txn.Asset)
			delete(a.shortLots, txn.Asset)
			delete(a.excursions, txn.Asset)
		}
	case asset.SellTransaction:
		a.holdings[txn.Asset] -= txn.Qty

		method := txn.LotSelection
		if method == LotFIFO && a.lotSelection != LotFIFO {
			method = a.lotSelection
		}

		// Phase 1: Close long lots (if any exist).
		longLots := a.taxLots[txn.Asset]

		longQty := 0.0
		for _, lot := range longLots {
			longQty += lot.Qty
		}

		closeLongQty := txn.Qty
		if closeLongQty > longQty {
			closeLongQty = longQty
		}

		if closeLongQty > 0 {
			// Generate TradeDetail entries from excursion data.
			if excursion, hasExcursion := a.excursions[txn.Asset]; hasExcursion {
				mfe := (excursion.HighPrice - excursion.EntryPrice) / excursion.EntryPrice
				mae := (excursion.LowPrice - excursion.EntryPrice) / excursion.EntryPrice

				tdRemaining := closeLongQty

				tdLots := longLots
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
						Direction:  TradeLong,
					})

					tdRemaining -= matched
				}
			}

			consumed := a.computeConsumedLotInfo(txn.Asset, closeLongQty, method)
			a.consumeLots(txn.Asset, closeLongQty, method)

			// Check for wash sale on the long close.
			lossPerShare := consumed.avgCostBasis - txn.Price
			if lossPerShare > 0 {
				disallowedQty := a.checkWashSaleOnSell(txn.Asset, txn.Date, closeLongQty, lossPerShare, consumed.latestBuyDate)

				remainingLossQty := closeLongQty - disallowedQty
				if remainingLossQty > 0 {
					a.recentLossSales[txn.Asset] = append(a.recentLossSales[txn.Asset], recentLossSale{
						date:         txn.Date,
						lossPerShare: lossPerShare,
						qty:          remainingLossQty,
					})
				}
			}
		}

		// If all long lots were consumed, remove the stale long excursion so
		// Phase 2 creates a fresh one for the short position.
		if closeLongQty > 0 && len(a.taxLots[txn.Asset]) == 0 {
			delete(a.excursions, txn.Asset)
		}

		// Phase 2: Open short lots for the remainder.
		shortQty := txn.Qty - closeLongQty
		if shortQty > 0 {
			lotID := fmt.Sprintf("short-%d-%d", txn.Date.UnixNano(), len(a.shortLots[txn.Asset]))
			a.shortLots[txn.Asset] = append(a.shortLots[txn.Asset], TaxLot{
				ID:    lotID,
				Date:  txn.Date,
				Qty:   shortQty,
				Price: txn.Price,
			})

			// Initialize excursion tracking for the short position.
			// Note: MFE/MAE semantics are inverted for shorts (price drop = favorable).
			// The correct interpretation is handled in Task 12 (P&L metrics) when
			// generating TradeDetail entries for short covers.
			if _, exists := a.excursions[txn.Asset]; !exists {
				a.excursions[txn.Asset] = ExcursionRecord{
					EntryPrice: txn.Price,
					HighPrice:  txn.Price,
					LowPrice:   txn.Price,
				}
			}
		}

		// Cleanup: remove tracking when fully flat.
		if a.holdings[txn.Asset] == 0 {
			delete(a.holdings, txn.Asset)
			delete(a.taxLots, txn.Asset)
			delete(a.shortLots, txn.Asset)
			delete(a.excursions, txn.Asset)
		}
	}
}

// SyncTransactions applies broker-reported transactions to the account,
// skipping any that have already been recorded (by ID).
func (a *Account) SyncTransactions(txns []broker.Transaction) error {
	for _, bt := range txns {
		if _, seen := a.seenTransactions[bt.ID]; seen {
			continue
		}

		// Splits require special handling -- they adjust holdings and
		// tax lots rather than just recording a cash event.
		if bt.Type == asset.SplitTransaction {
			if err := a.ApplySplit(bt.Asset, bt.Date, bt.Price); err != nil {
				return fmt.Errorf("sync transactions: %w", err)
			}

			a.seenTransactions[bt.ID] = struct{}{}

			continue
		}

		a.Record(Transaction{
			ID:            bt.ID,
			Date:          bt.Date,
			Asset:         bt.Asset,
			Type:          bt.Type,
			Qty:           bt.Qty,
			Price:         bt.Price,
			Amount:        bt.Amount,
			Justification: bt.Justification,
		})
	}

	return nil
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

// consumeShortLots removes qty shares from the short lots for the given
// asset using the specified lot selection method. Mirrors consumeLots.
func (a *Account) consumeShortLots(ast asset.Asset, qty float64, method LotSelection) {
	lots := a.shortLots[ast]

	switch method {
	case LotLIFO:
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

		a.shortLots[ast] = lots[:end]

	case LotHighestCost:
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

		a.shortLots[ast] = remainingLots

	default: // LotFIFO
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

		a.shortLots[ast] = lots[lotIdx:]
	}
}

// avgShortEntryPrice computes the weighted average entry price of short
// lots that would be consumed for the given qty and method.
func (a *Account) avgShortEntryPrice(ast asset.Asset, qty float64, method LotSelection) float64 {
	lots := make([]TaxLot, len(a.shortLots[ast]))
	copy(lots, a.shortLots[ast])

	switch method {
	case LotLIFO:
		remaining := qty
		totalCost := 0.0
		totalQty := 0.0

		for idx := len(lots) - 1; idx >= 0 && remaining > 0; idx-- {
			matched := lots[idx].Qty
			if matched > remaining {
				matched = remaining
			}

			totalCost += matched * lots[idx].Price
			totalQty += matched
			remaining -= matched
		}

		if totalQty == 0 {
			return 0
		}

		return totalCost / totalQty

	case LotHighestCost:
		sortLotsByPriceDesc(lots)

		fallthrough

	default: // FIFO or HighestCost after sort
		remaining := qty
		totalCost := 0.0
		totalQty := 0.0

		for idx := 0; idx < len(lots) && remaining > 0; idx++ {
			matched := lots[idx].Qty
			if matched > remaining {
				matched = remaining
			}

			totalCost += matched * lots[idx].Price
			totalQty += matched
			remaining -= matched
		}

		if totalQty == 0 {
			return 0
		}

		return totalCost / totalQty
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

		row, err := data.NewDataFrame(timestamps, assets, metrics, data.Daily, [][]float64{{total}, {benchVal}, {rfVal}})
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

	// Invalidate lazily-computed DataFrames so they are recomputed on next access.
	a.dfCache = nil
}

// PerfData returns the accumulated performance DataFrame, or nil if no
// prices have been recorded yet.
func (a *Account) PerfData() *data.DataFrame { return a.perfData }

// SetPerfData replaces the performance DataFrame. This is intended for
// testing scenarios where a synthetic equity curve is needed.
func (a *Account) SetPerfData(df *data.DataFrame) {
	a.perfData = df
}

// TaxLots returns the current tax lot positions keyed by asset.
func (a *Account) TaxLots() map[asset.Asset][]TaxLot { return a.taxLots }

// ShortLots iterates over all open short tax lots, calling fn with each asset and its lots.
func (a *Account) ShortLots(fn func(asset.Asset, []TaxLot)) {
	for ast, lots := range a.shortLots {
		fn(ast, lots)
	}
}

// ApplySplit adjusts holdings, tax lots, short lots, and excursion records
// for a stock split. splitFactor is the multiplier applied to share
// quantities (e.g. 2.0 for a 2-for-1 split). A SplitTransaction is
// appended to the transaction log.
func (a *Account) ApplySplit(ast asset.Asset, date time.Time, splitFactor float64) error {
	if splitFactor == 0 {
		return fmt.Errorf("apply split: split factor cannot be zero for %s", ast.Ticker)
	}

	qty := a.holdings[ast]
	if qty == 0 {
		return nil
	}

	oldQty := qty
	newQty := qty * splitFactor
	a.holdings[ast] = newQty

	for idx := range a.taxLots[ast] {
		a.taxLots[ast][idx].Qty *= splitFactor
		a.taxLots[ast][idx].Price /= splitFactor
	}

	for idx := range a.shortLots[ast] {
		a.shortLots[ast][idx].Qty *= splitFactor
		a.shortLots[ast][idx].Price /= splitFactor
	}

	if excursion, exists := a.excursions[ast]; exists {
		excursion.EntryPrice /= splitFactor
		excursion.HighPrice /= splitFactor
		excursion.LowPrice /= splitFactor
		a.excursions[ast] = excursion
	}

	a.transactions = append(a.transactions, Transaction{
		Date:          date,
		Asset:         ast,
		Type:          asset.SplitTransaction,
		Qty:           newQty,
		Price:         splitFactor,
		Amount:        0,
		Justification: fmt.Sprintf("split %.4g:1 old_qty=%.4g new_qty=%.4g", splitFactor, oldQty, newQty),
	})

	return nil
}

// Excursions returns the current excursion records keyed by asset.
func (a *Account) Excursions() map[asset.Asset]ExcursionRecord { return a.excursions }

// TradeDetails returns all completed round-trip trades with per-trade
// MFE and MAE excursion data.
func (a *Account) TradeDetails() []TradeDetail { return a.tradeDetails }

// Prices returns the most recent price DataFrame, or nil if no prices
// have been recorded yet.
func (a *Account) Prices() *data.DataFrame { return a.prices }

// SetPrices stores a price DataFrame on the account without recording an
// equity point. This is used to make margin calculations available before
// the full UpdatePrices call that records performance data.
func (a *Account) SetPrices(priceData *data.DataFrame) {
	if priceData != nil && priceData.Len() > 0 {
		a.prices = priceData
	}
}

func (a *Account) SetBroker(b broker.Broker) {
	a.broker = b
	_, a.brokerHasGroups = b.(broker.GroupSubmitter)
}

// HasBroker returns true if a broker has been set on the account.
func (a *Account) HasBroker() bool { return a.broker != nil }

// SetPendingOrder inserts an order into the pending-orders map. This is
// intended for test setup; production code uses ExecuteBatch.
func (a *Account) SetPendingOrder(order broker.Order) {
	a.pendingOrders[order.ID] = order
}

// SetPendingGroup inserts a group into the pending-groups map. This is
// intended for test setup; production code uses ExecuteBatch.
func (a *Account) SetPendingGroup(group *broker.OrderGroup) {
	a.pendingGroups[group.ID] = group
}

// PendingOrderIDs returns a slice of all pending order IDs.
func (a *Account) PendingOrderIDs() []string {
	ids := make([]string, 0, len(a.pendingOrders))
	for id := range a.pendingOrders {
		ids = append(ids, id)
	}

	return ids
}

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
			a.annotations[idx].BatchID = a.currentBatchID
			return
		}
	}

	a.annotations = append(a.annotations, Annotation{
		Timestamp: timestamp,
		Key:       key,
		Value:     value,
		BatchID:   a.currentBatchID,
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

// ClearMiddleware removes all registered middleware.
func (a *Account) ClearMiddleware() {
	a.middleware = nil
}

// NewBatch creates an empty Batch for the given timestamp, bound to this
// account for position and price queries.
func (a *Account) NewBatch(timestamp time.Time) *Batch {
	return NewBatch(timestamp, a)
}

// ExecuteBatch runs the middleware chain, records annotations, assigns
// order IDs, submits orders to the broker, and drains immediate fills.
func (a *Account) ExecuteBatch(ctx context.Context, batch *Batch) error {
	batchID := len(a.batches) + 1
	a.batches = append(a.batches, batchRecord{BatchID: batchID, Timestamp: batch.Timestamp})
	a.currentBatchID = batchID
	defer func() { a.currentBatchID = 0 }()

	// 1. Run middleware chain.
	if !batch.SkipMiddleware {
		for _, mw := range a.middleware {
			if err := mw.Process(ctx, batch); err != nil {
				return err
			}
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
		order.BatchID = batchID

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

// deferredExitInfo records the information needed to submit bracket exit
// orders after a bracket entry fill.
type deferredExitInfo struct {
	groupID   string
	spec      OrderGroupSpec
	entrySide broker.Side
	fillPrice float64
	asset     asset.Asset
	qty       float64
	// batchID is the BatchID of the originating entry order. Exit fills carry
	// this id rather than the batch active at fill time (which is typically
	// zero — fills arrive outside ExecuteBatch).
	batchID int
}

// drainFillsFromChannel reads all available fills from the broker's
// fill channel (non-blocking) and records each as a transaction. After
// draining, it submits deferred bracket exit orders for any entry fills.
func (a *Account) drainFillsFromChannel() {
	if a.broker == nil {
		return
	}

	fillCh := a.broker.Fills()

	// Phase 1: Collect fills and handle OCO cancellations.
	var pendingExits []deferredExitInfo

	for {
		select {
		case fill := <-fillCh:
			order, ok := a.pendingOrders[fill.OrderID]
			if !ok {
				log.Warn().Str("orderID", fill.OrderID).Msg("received fill for unknown order")
				continue
			}

			var (
				txType asset.TransactionType
				amount float64
			)

			switch order.Side {
			case broker.Buy:
				txType = asset.BuyTransaction
				amount = -(fill.Price * fill.Qty)
			case broker.Sell:
				txType = asset.SellTransaction
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
				BatchID:       order.BatchID,
			})

			delete(a.pendingOrders, fill.OrderID)

			// Group handling for the filled order.
			if order.GroupID != "" {
				switch order.GroupRole {
				case broker.RoleEntry:
					// Bracket entry filled: collect deferred exits for Phase 2.
					if spec, found := a.deferredExits[order.GroupID]; found {
						pendingExits = append(pendingExits, deferredExitInfo{
							groupID:   order.GroupID,
							spec:      spec,
							entrySide: order.Side,
							fillPrice: fill.Price,
							asset:     order.Asset,
							qty:       fill.Qty,
							batchID:   order.BatchID,
						})
						delete(a.deferredExits, order.GroupID)
					}

				case broker.RoleStopLoss, broker.RoleTakeProfit:
					// OCO leg filled: cancel siblings.
					a.cancelOCOSiblings(order.GroupID, fill.OrderID)
				}
			}

		default:
			goto phase2
		}
	}

phase2:
	// Phase 2: Submit deferred bracket exit orders.
	for _, exitInfo := range pendingExits {
		a.submitBracketExits(exitInfo)
	}
}

// cancelOCOSiblings removes sibling orders from an OCO group when one
// leg fills. For GroupSubmitter brokers, siblings are removed from
// pendingOrders directly (the broker handles cancellation). For fallback
// brokers, Cancel is called on each sibling.
func (a *Account) cancelOCOSiblings(groupID, filledOrderID string) {
	group, ok := a.pendingGroups[groupID]
	if !ok {
		return
	}

	for _, siblingID := range group.OrderIDs {
		if siblingID == filledOrderID {
			continue
		}

		if a.brokerHasGroups {
			delete(a.pendingOrders, siblingID)
		} else {
			if err := a.broker.Cancel(context.Background(), siblingID); err != nil {
				log.Warn().Err(err).
					Str("orderID", siblingID).
					Str("groupID", groupID).
					Msg("failed to cancel OCO sibling")
			}

			delete(a.pendingOrders, siblingID)
		}
	}

	delete(a.pendingGroups, groupID)
}

// submitBracketExits creates and submits the stop-loss and take-profit
// exit orders for a filled bracket entry.
func (a *Account) submitBracketExits(info deferredExitInfo) {
	exitGroupID := info.groupID + "-exits"

	// Determine exit prices.
	stopPrice := resolveExitPrice(info.fillPrice, info.spec.StopLoss)
	takeProfitPrice := resolveExitPrice(info.fillPrice, info.spec.TakeProfit)

	// Exit side is opposite of entry side.
	var exitSide broker.Side
	if info.entrySide == broker.Buy {
		exitSide = broker.Sell
	} else {
		exitSide = broker.Buy
	}

	stopLossOrder := broker.Order{
		ID:          exitGroupID + "-sl",
		Asset:       info.asset,
		Side:        exitSide,
		Qty:         info.qty,
		OrderType:   broker.Stop,
		StopPrice:   stopPrice,
		TimeInForce: broker.GTC,
		GroupID:     exitGroupID,
		GroupRole:   broker.RoleStopLoss,
		BatchID:     info.batchID,
	}

	takeProfitOrder := broker.Order{
		ID:          exitGroupID + "-tp",
		Asset:       info.asset,
		Side:        exitSide,
		Qty:         info.qty,
		OrderType:   broker.Limit,
		LimitPrice:  takeProfitPrice,
		TimeInForce: broker.GTC,
		GroupID:     exitGroupID,
		GroupRole:   broker.RoleTakeProfit,
		BatchID:     info.batchID,
	}

	// Track in pendingOrders and pendingGroups.
	a.pendingOrders[stopLossOrder.ID] = stopLossOrder
	a.pendingOrders[takeProfitOrder.ID] = takeProfitOrder
	a.pendingGroups[exitGroupID] = &broker.OrderGroup{
		ID:       exitGroupID,
		Type:     broker.GroupOCO,
		OrderIDs: []string{stopLossOrder.ID, takeProfitOrder.ID},
	}

	// Submit via GroupSubmitter or individual Submit.
	exitOrders := []broker.Order{stopLossOrder, takeProfitOrder}

	if a.brokerHasGroups {
		gs := a.broker.(broker.GroupSubmitter)
		if err := gs.SubmitGroup(context.Background(), exitOrders, broker.GroupOCO); err != nil {
			log.Warn().Err(err).
				Str("groupID", exitGroupID).
				Msg("failed to submit bracket exit group")
		}
	} else {
		for _, ord := range exitOrders {
			if err := a.broker.Submit(context.Background(), ord); err != nil {
				log.Warn().Err(err).
					Str("orderID", ord.ID).
					Str("groupID", exitGroupID).
					Msg("failed to submit bracket exit order")
			}
		}
	}

	// Clean up the parent bracket group.
	delete(a.pendingGroups, info.groupID)
}

// resolveExitPrice computes the exit price from an ExitTarget. If
// AbsolutePrice is non-zero, it is used directly; otherwise the price
// is computed as fillPrice * (1 + PercentOffset).
func resolveExitPrice(fillPrice float64, target ExitTarget) float64 {
	if target.AbsolutePrice != 0 {
		return target.AbsolutePrice
	}

	return fillPrice * (1 + target.PercentOffset)
}

// CancelOpenOrders cancels all pending orders tracked by the account and
// clears all group state (pendingOrders, pendingGroups, deferredExits).
func (a *Account) CancelOpenOrders(ctx context.Context) error {
	if a.broker == nil {
		return nil
	}

	var errs []error

	// Separate bracket exit orders (stop-loss/take-profit) from regular orders.
	// Bracket exits persist across bars until triggered by EvaluatePending.
	survivingOrders := make(map[string]broker.Order)
	survivingGroups := make(map[string]*broker.OrderGroup)

	for orderID, order := range a.pendingOrders {
		if order.GroupRole == broker.RoleStopLoss || order.GroupRole == broker.RoleTakeProfit {
			survivingOrders[orderID] = order

			if order.GroupID != "" {
				if grp, ok := a.pendingGroups[order.GroupID]; ok {
					survivingGroups[order.GroupID] = grp
				}
			}

			continue
		}

		// Best-effort cancel: order may have already been filled by the broker
		// (e.g. market orders fill immediately in SimulatedBroker.Submit).
		if cancelErr := a.broker.Cancel(ctx, orderID); cancelErr != nil {
			_ = cancelErr
		}
	}

	a.pendingOrders = survivingOrders
	a.pendingGroups = survivingGroups
	a.deferredExits = make(map[string]OrderGroupSpec)

	return errors.Join(errs...)
}

// Clone returns a deep copy of the Account suitable for prediction runs.
// Holdings, metadata, and annotations are independent copies. PerfData is
// deep-copied via DataFrame.Copy. Transactions and tax lots are shallow-copied
// (the clone gets its own slice header but shares the underlying elements,
// which is safe since appending to the clone's slice does not affect the
// original).
func (acct *Account) Clone() PortfolioManager {
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

	shortLots := make(map[asset.Asset][]TaxLot, len(acct.shortLots))
	for held, lots := range acct.shortLots {
		lotsCopy := make([]TaxLot, len(lots))
		copy(lotsCopy, lots)
		shortLots[held] = lotsCopy
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

	seenTxns := make(map[string]struct{}, len(acct.seenTransactions))
	for id := range acct.seenTransactions {
		seenTxns[id] = struct{}{}
	}

	clone := &Account{
		cash:              acct.cash,
		holdings:          holdings,
		transactions:      transactions,
		broker:            acct.broker,
		prices:            acct.prices,
		benchmark:         acct.benchmark,
		riskFreeValue:     acct.riskFreeValue,
		taxLots:           taxLots,
		shortLots:         shortLots,
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
		seenTransactions:  seenTxns,
	}

	if acct.perfData != nil {
		clone.perfData = acct.perfData.Copy()
	}

	return clone
}

// BorrowRate returns the configured annualized borrow fee rate.
func (a *Account) BorrowRate() float64 {
	if a.borrowRate > 0 {
		return a.borrowRate
	}

	return 0.005 // default 0.5%
}

// ---------------------------------------------------------------------------
// PortfolioStats interface implementation
// ---------------------------------------------------------------------------

// dfCacheKey identifies a cached DataFrame result by derived metric and window.
type dfCacheKey struct {
	metric data.Metric
	window string
}

// windowKey returns a stable string key for a Period (or "nil" for nil).
func windowKey(window *Period) string {
	if window == nil {
		return "nil"
	}

	return fmt.Sprintf("%d_%d", window.Unit, window.N)
}

// cachedDF returns a previously computed DataFrame, or nil if not cached.
func (a *Account) cachedDF(metric data.Metric, window *Period) *data.DataFrame {
	if a.dfCache == nil {
		return nil
	}

	return a.dfCache[dfCacheKey{metric: metric, window: windowKey(window)}]
}

// putDF stores a computed DataFrame in the cache.
func (a *Account) putDF(metric data.Metric, window *Period, df *data.DataFrame) {
	if a.dfCache == nil {
		a.dfCache = make(map[dfCacheKey]*data.DataFrame)
	}

	a.dfCache[dfCacheKey{metric: metric, window: windowKey(window)}] = df
}

// Returns computes period-over-period percentage returns of the portfolio
// equity curve within the given window. Results are cached per window.
func (a *Account) Returns(_ context.Context, window *Period) *data.DataFrame {
	if a.perfData == nil {
		return nil
	}

	if cached := a.cachedDF(data.PortfolioReturns, window); cached != nil {
		return cached
	}

	df := a.perfData.Window(window).Metrics(data.PortfolioEquity).Pct()
	a.putDF(data.PortfolioReturns, window, df)

	return df
}

// ExcessReturns computes portfolio returns minus the risk-free rate within
// the given window. Results are cached per window.
func (a *Account) ExcessReturns(_ context.Context, window *Period) *data.DataFrame {
	if a.perfData == nil {
		return nil
	}

	if cached := a.cachedDF(data.PortfolioExcessReturns, window); cached != nil {
		return cached
	}

	perfDF := a.perfData.Window(window).Metrics(data.PortfolioEquity, data.PortfolioRiskFree).Pct()
	df := perfDF.Metrics(data.PortfolioEquity).Sub(perfDF, data.PortfolioRiskFree)
	a.putDF(data.PortfolioExcessReturns, window, df)

	return df
}

// Drawdown computes the percentage drawdown from the running equity peak
// within the given window. Results are cached per window.
func (a *Account) Drawdown(_ context.Context, window *Period) *data.DataFrame {
	if a.perfData == nil {
		return nil
	}

	if cached := a.cachedDF(data.PortfolioDrawdown, window); cached != nil {
		return cached
	}

	equity := a.perfData.Window(window).Metrics(data.PortfolioEquity)
	peak := equity.CumMax()
	df := equity.Sub(peak).Div(peak)
	a.putDF(data.PortfolioDrawdown, window, df)

	return df
}

// BenchmarkReturns computes period-over-period percentage returns of the
// benchmark within the given window. Results are cached per window.
func (a *Account) BenchmarkReturns(_ context.Context, window *Period) *data.DataFrame {
	if a.perfData == nil {
		return nil
	}

	if cached := a.cachedDF(data.PortfolioBenchReturns, window); cached != nil {
		return cached
	}

	df := a.perfData.Window(window).Metrics(data.PortfolioBenchmark).Pct()
	a.putDF(data.PortfolioBenchReturns, window, df)

	return df
}

// EquitySeries returns the windowed perfData DataFrame containing the equity curve.
func (a *Account) EquitySeries(_ context.Context, window *Period) *data.DataFrame {
	if a.perfData == nil {
		return nil
	}

	return a.perfData.Window(window)
}

// TransactionsView returns the full transaction log in chronological order.
// This satisfies the PortfolioStats interface.
func (a *Account) TransactionsView(_ context.Context) []Transaction {
	return a.transactions
}

// TradeDetailsView returns all completed round-trip trades with per-trade
// MFE and MAE excursion data. This satisfies the PortfolioStats interface.
func (a *Account) TradeDetailsView(_ context.Context) []TradeDetail {
	return a.tradeDetails
}

// PricesView returns the most recent price DataFrame.
// This satisfies the PortfolioStats interface.
func (a *Account) PricesView(_ context.Context) *data.DataFrame {
	return a.prices
}

// TaxLotsView returns the current tax lot positions keyed by asset.
// This satisfies the PortfolioStats interface.
func (a *Account) TaxLotsView(_ context.Context) map[asset.Asset][]TaxLot {
	return a.taxLots
}

// ShortLotsView iterates over all open short tax lots, calling fn with each
// asset and its lots. This satisfies the PortfolioStats interface.
func (a *Account) ShortLotsView(_ context.Context, fn func(asset.Asset, []TaxLot)) {
	for ast, lots := range a.shortLots {
		fn(ast, lots)
	}
}

// PerfDataView returns the accumulated performance DataFrame, or nil if no
// prices have been recorded yet. This satisfies the PortfolioStats interface.
func (a *Account) PerfDataView(_ context.Context) *data.DataFrame {
	return a.perfData
}
