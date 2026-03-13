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

// WithMetric registers a single PerformanceMetric for computation.
func WithMetric(m PerformanceMetric) Option {
	return func(a *Account) {
		for _, existing := range a.registeredMetrics {
			if existing.Name() == m.Name() {
				return // deduplicate
			}
		}
		a.registeredMetrics = append(a.registeredMetrics, m)
	}
}

// RegisteredMetrics returns the list of metrics registered for computation.
func (a *Account) RegisteredMetrics() []PerformanceMetric {
	return a.registeredMetrics
}

// WithSummaryMetrics registers the summary metric group.
func WithSummaryMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{TWRR, MWRR, Sharpe, Sortino, Calmar, MaxDrawdown, StdDev} {
			WithMetric(m)(a)
		}
	}
}

// WithRiskMetrics registers the risk metric group.
func WithRiskMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			Beta, Alpha, TrackingError, DownsideDeviation,
			InformationRatio, Treynor, UlcerIndex, ExcessKurtosis,
			Skewness, RSquared, ValueAtRisk, UpsideCaptureRatio, DownsideCaptureRatio,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithTradeMetrics registers the trade metric group.
func WithTradeMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			WinRate, AverageWin, AverageLoss, ProfitFactor,
			AverageHoldingPeriod, Turnover, NPositivePeriods, TradeGainLossRatio,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithWithdrawalMetrics registers the withdrawal metric group.
func WithWithdrawalMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{SafeWithdrawalRate, PerpetualWithdrawalRate, DynamicWithdrawalRate} {
			WithMetric(m)(a)
		}
	}
}

// WithTaxMetrics registers the tax metric group.
func WithTaxMetrics() Option {
	return func(a *Account) {
		for _, m := range []PerformanceMetric{
			LTCGMetric, STCGMetric, UnrealizedLTCGMetric, UnrealizedSTCGMetric,
			QualifiedDividendsMetric, NonQualifiedIncomeMetric, TaxCostRatioMetric,
		} {
			WithMetric(m)(a)
		}
	}
}

// WithAllMetrics registers every known PerformanceMetric.
func WithAllMetrics() Option {
	return func(a *Account) {
		WithSummaryMetrics()(a)
		WithRiskMetrics()(a)
		WithTradeMetrics()(a)
		WithWithdrawalMetrics()(a)
		WithTaxMetrics()(a)
		for _, m := range []PerformanceMetric{
			CAGR, ActiveReturn, SmartSharpe, SmartSortino,
			ProbabilisticSharpe, KRatio, KellerRatio, KellyCriterion,
			OmegaRatio, GainToPainRatio, CVaR, TailRatio, RecoveryFactor,
			Exposure, ConsecutiveWins, ConsecutiveLosses,
			AvgDrawdown, AvgDrawdownDays, GainLossRatio,
		} {
			WithMetric(m)(a)
		}
	}
}
