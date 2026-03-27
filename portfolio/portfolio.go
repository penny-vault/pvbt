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
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// Ensure Account implements all portfolio interfaces at compile time.
var (
	_ Portfolio        = (*Account)(nil)
	_ PortfolioManager = (*Account)(nil)
	_ PortfolioStats   = (*Account)(nil)
)

// Portfolio is the read-only interface that strategy code and middleware
// receive during execution. It provides query access to portfolio state
// (Cash, Value, Positions, Transactions, metrics, metadata, annotations)
// but no mutation methods. Orders are placed through a Batch, which the
// engine submits via PortfolioManager.ExecuteBatch.
type Portfolio interface {
	// Cash returns the current cash balance available in the portfolio.
	Cash() float64

	// Value returns the total portfolio value: cash plus all holdings
	// marked to current prices.
	Value() float64

	// Position returns the quantity held of a specific asset.
	Position(a asset.Asset) float64

	// PositionValue returns the current market value of the position
	// in a specific asset (quantity * current price).
	PositionValue(a asset.Asset) float64

	// Holdings returns a map of all current positions keyed by asset
	// with the held quantity as the value.
	Holdings() map[asset.Asset]float64

	// Transactions returns the full transaction log in chronological
	// order. The log contains every event that changed the portfolio:
	// trades, dividends, fees, deposits, and withdrawals.
	Transactions() []Transaction

	// Prices returns the most-recent price DataFrame supplied to
	// UpdatePrices. This includes prices for all assets that were in
	// the last price update, not just currently-held positions. Returns
	// nil if UpdatePrices has not yet been called.
	Prices() *data.DataFrame

	// PerfData returns the accumulated performance DataFrame containing
	// equity curve, benchmark, and risk-free price series. Returns nil
	// if no prices have been recorded yet.
	PerfData() *data.DataFrame

	// PerformanceMetric returns a PerformanceMetricQuery builder for the given
	// metric. Use .Value() to get a single number over the full
	// history, .Window() to set a lookback period, or .Series() to
	// get a rolling time series. Example:
	//
	//   p.PerformanceMetric(Sharpe).Window(Months(36)).Value()
	//
	PerformanceMetric(m PerformanceMetric) PerformanceMetricQuery

	// Summary returns the headline performance numbers (TWRR, Sharpe,
	// Sortino, Calmar, MaxDrawdown, StdDev) computed over the full
	// history.
	Summary() (Summary, error)

	// RiskMetrics returns risk-related measurements (Beta, Alpha,
	// TrackingError, DownsideDeviation, InformationRatio, Treynor)
	// computed over the full history.
	RiskMetrics() (RiskMetrics, error)

	// TaxMetrics returns tax-related measurements (LTCG, STCG,
	// unrealized gains, dividends, TaxCostRatio) derived from the
	// transaction log and tax lot tracking.
	TaxMetrics() (TaxMetrics, error)

	// TradeMetrics returns trade analysis measurements (WinRate,
	// ProfitFactor, AverageHoldingPeriod, Turnover, etc.) derived
	// from the transaction log.
	TradeMetrics() (TradeMetrics, error)

	// WithdrawalMetrics returns sustainable spending rates
	// (SafeWithdrawalRate, PerpetualWithdrawalRate,
	// DynamicWithdrawalRate) derived from Monte Carlo simulation
	// of historical returns.
	WithdrawalMetrics() (WithdrawalMetrics, error)

	// SetMetadata stores a key-value string pair in the portfolio's
	// metadata map. Use this for run-level information like run ID,
	// strategy name, start/end dates, and strategy parameters.
	SetMetadata(key, value string)

	// GetMetadata retrieves a metadata value by key. Returns empty
	// string if the key has not been set.
	GetMetadata(key string) string

	// Annotations returns the full annotation log in the order entries
	// were recorded.
	Annotations() []Annotation

	// TradeDetails returns all completed round-trip trades with per-trade
	// MFE and MAE excursion data. Strategies may use this during Compute()
	// to adapt behavior based on past trade quality.
	TradeDetails() []TradeDetail

	// Equity returns the portfolio equity: cash + long market value - short market value.
	Equity() float64

	// LongMarketValue returns the total market value of all long positions.
	LongMarketValue() float64

	// ShortMarketValue returns the total absolute market value of all short positions.
	ShortMarketValue() float64

	// MarginRatio returns equity divided by short market value.
	// Returns NaN if there are no short positions.
	MarginRatio() float64

	// MarginDeficiency returns the dollar amount needed to restore maintenance
	// margin. Returns 0 if the account is healthy or has no short positions.
	MarginDeficiency() float64

	// BuyingPower returns cash minus margin reserved for short positions.
	BuyingPower() float64

	// Benchmark returns the asset used as the performance benchmark
	// for this portfolio.
	Benchmark() asset.Asset
}

// PortfolioManager is the interface the engine uses to manage the
// portfolio between and around strategy execution steps. The strategy
// never sees this interface. The engine calls these methods in a
// specific order at each step:
//
//  1. Record -- inject dividends for held assets
//  2. Strategy.Compute -- strategy trades via the Portfolio interface
//  3. UpdatePrices -- mark all positions to current market prices
//
// After UpdatePrices, calls to Portfolio.Value() and
// Portfolio.PositionValue() reflect the latest prices.
type PortfolioManager interface {
	Portfolio

	// Record appends a transaction to the log and updates the
	// portfolio's cash balance and positions accordingly. The engine
	// calls this for dividends, fees, deposits, withdrawals, and any
	// other events that originate outside strategy code.
	Record(tx Transaction)

	// UpdatePrices marks all positions to current market prices using
	// the provided DataFrame, which contains prices for held assets
	// as well as benchmark and risk-free data. The engine calls this
	// after strategy execution at each step so that Value() and
	// PositionValue() reflect current marks. This also records the
	// portfolio's total value at this point in time for use by
	// performance metrics (equity curve, drawdowns, returns, etc.).
	UpdatePrices(df *data.DataFrame)

	// SetBroker replaces the broker used for order execution. A broker
	// is always required -- for backtesting the engine provides a
	// simulated broker, for live trading a real one.
	SetBroker(b broker.Broker)

	// Use appends one or more middleware to the processing chain.
	// Middleware run in order during ExecuteBatch, before any orders
	// are submitted to the broker.
	Use(middleware ...Middleware)

	// ClearMiddleware removes all registered middleware from the processing
	// chain. The engine calls this when config-driven middleware replaces
	// strategy-declared middleware.
	ClearMiddleware()

	// NewBatch creates an empty Batch for the given timestamp, bound
	// to this portfolio for position and price queries.
	NewBatch(timestamp time.Time) *Batch

	// ExecuteBatch runs the middleware chain on the batch, records
	// annotations, assigns order IDs, submits orders to the broker,
	// and drains immediate fills.
	ExecuteBatch(ctx context.Context, batch *Batch) error

	// UpdateExcursions reads daily High and Low prices from the DataFrame
	// and updates the running extremes for each open position. Called by
	// the engine after UpdatePrices at each step.
	UpdateExcursions(df *data.DataFrame)

	// DrainFills drains any pending fills from the broker's fill
	// channel and records them as transactions.
	DrainFills(ctx context.Context) error

	// CancelOpenOrders cancels all open or submitted orders and
	// removes them from the pending-orders tracker.
	CancelOpenOrders(ctx context.Context) error

	// SetBenchmark sets the benchmark asset for performance comparison.
	SetBenchmark(benchmark asset.Asset)

	// SetRiskFreeValue sets the annualized risk-free rate used for
	// risk-adjusted return calculations (Sharpe, Sortino, etc.).
	SetRiskFreeValue(value float64)

	// HasBroker returns true if a broker has been assigned to this
	// portfolio manager.
	HasBroker() bool

	// ApplySplit adjusts holdings and tax lots for a stock split on the
	// given asset at the given date with the provided split factor.
	ApplySplit(ast asset.Asset, date time.Time, splitFactor float64) error

	// BorrowRate returns the annualized borrow fee rate charged on
	// short positions.
	BorrowRate() float64

	// RegisteredMetrics returns the list of performance metrics that
	// have been registered with this portfolio.
	RegisteredMetrics() []PerformanceMetric

	// AppendMetric appends a computed metric row to the portfolio's
	// metric history.
	AppendMetric(row MetricRow)

	// SetPrices replaces the current price data with the provided
	// DataFrame. Unlike UpdatePrices, this does not record the
	// portfolio value -- it only sets the price reference used by
	// position valuation.
	SetPrices(priceData *data.DataFrame)

	// SyncTransactions applies broker-reported transactions (dividends, splits,
	// fees) to the portfolio, deduplicating by transaction ID.
	SyncTransactions(txns []broker.Transaction) error

	// Clone returns a deep copy of the portfolio manager. The clone
	// is independent: mutations to one do not affect the other.
	Clone() PortfolioManager
}
