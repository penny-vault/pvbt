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

// Option configures an Account during construction.
type Option func(*Account)

// Account is the concrete type that implements both Portfolio and
// PortfolioManager. The user creates an Account with New, passes it to
// the engine, and inspects it after the run. The engine holds it as
// *Account (giving access to both interfaces): it passes it as Portfolio
// to strategy Compute calls, and calls Record/SetBroker directly.
type Account struct{}

// New creates an Account with the given options.
func New(opts ...Option) *Account {
	a := &Account{}
	for _, opt := range opts {
		opt(a)
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

// WithCash returns an Option that sets the initial cash balance.
func WithCash(amount float64) Option {
	return func(a *Account) {}
}

// --- Portfolio interface ---

func (a *Account) RebalanceTo(alloc ...Allocation)                                     {}
func (a *Account) Order(ast asset.Asset, side Side, qty float64, mods ...OrderModifier) {}
func (a *Account) Cash() float64                                                        { return 0 }
func (a *Account) Value() float64                                                       { return 0 }
func (a *Account) Position(ast asset.Asset) float64                                     { return 0 }
func (a *Account) PositionValue(ast asset.Asset) float64                                { return 0 }
func (a *Account) Holdings(fn func(asset.Asset, float64))                               {}
func (a *Account) Transactions() []Transaction                                          { return nil }

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

func (a *Account) Record(tx Transaction)                    {}
func (a *Account) UpdatePrices(df *data.DataFrame) {}
func (a *Account) SetBroker(b broker.Broker)                {}

