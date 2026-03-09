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
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// Portfolio is the interface that strategy code receives during Compute.
// It provides allocation decisions (RebalanceTo, Order) and read access
// to portfolio state (Cash, Value, Positions, Transactions). Strategy
// code cannot directly modify the transaction log -- only RebalanceTo
// and Order produce trade transactions.
type Portfolio interface {
	// RebalanceTo adjusts the portfolio to match the given allocations.
	// Pass a single Allocation for an immediate rebalance, or spread a
	// PortfolioPlan to apply a series of rebalances in date order. The
	// engine diffs current holdings against each target and generates
	// the necessary buy/sell orders. Each resulting trade appends a
	// BuyTransaction or SellTransaction to the transaction log. Any
	// commissions produce a FeeTransaction.
	RebalanceTo(alloc ...Allocation)

	// Order places an individual order for a specific asset. This is
	// the imperative counterpart to RebalanceTo, used when a strategy
	// needs fine-grained control over individual trades. Optional
	// modifiers (Limit, Stop, GoodTilCancel, FillOrKill, etc.) adjust
	// order behavior. Each fill appends a BuyTransaction or
	// SellTransaction to the transaction log. Any commissions produce
	// a FeeTransaction.
	Order(a asset.Asset, side Side, qty float64, mods ...OrderModifier)

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

	// Holdings iterates over all current positions, calling fn with
	// each asset and its held quantity.
	Holdings(fn func(asset.Asset, float64))

	// Transactions returns the full transaction log in chronological
	// order. The log contains every event that changed the portfolio:
	// trades, dividends, fees, deposits, and withdrawals.
	Transactions() []Transaction

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
	Summary() Summary

	// RiskMetrics returns risk-related measurements (Beta, Alpha,
	// TrackingError, DownsideDeviation, InformationRatio, Treynor)
	// computed over the full history.
	RiskMetrics() RiskMetrics

	// TaxMetrics returns tax-related measurements (LTCG, STCG,
	// unrealized gains, dividends, TaxCostRatio) derived from the
	// transaction log and tax lot tracking.
	TaxMetrics() TaxMetrics

	// TradeMetrics returns trade analysis measurements (WinRate,
	// ProfitFactor, AverageHoldingPeriod, Turnover, etc.) derived
	// from the transaction log.
	TradeMetrics() TradeMetrics

	// WithdrawalMetrics returns sustainable spending rates
	// (SafeWithdrawalRate, PerpetualWithdrawalRate,
	// DynamicWithdrawalRate) derived from Monte Carlo simulation
	// of historical returns.
	WithdrawalMetrics() WithdrawalMetrics
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
}
