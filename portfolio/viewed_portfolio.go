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
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

// Compile-time checks that viewedPortfolio satisfies both interfaces.
var (
	_ Portfolio      = (*viewedPortfolio)(nil)
	_ PortfolioStats = (*viewedPortfolio)(nil)
)

// viewedPortfolio wraps an *Account with a date range and restricts all
// metric computations and DataFrame-returning methods to [start, end].
// Point-in-time methods (Cash, Value, Holdings, etc.) delegate directly
// to the underlying account.
type viewedPortfolio struct {
	acct  *Account
	stats PortfolioStats
	start time.Time
	end   time.Time
}

// --- Portfolio interface: metric methods ---

// PerformanceMetric returns a query builder that pre-applies AbsoluteWindow
// so the metric is computed only over the view's date range.
func (vp *viewedPortfolio) PerformanceMetric(pm PerformanceMetric) PerformanceMetricQuery {
	return vp.acct.PerformanceMetric(pm).AbsoluteWindow(vp.start, vp.end)
}

func (vp *viewedPortfolio) Summary() (Summary, error) {
	var errs []error

	summary := Summary{}

	var err error

	summary.TWRR, err = vp.PerformanceMetric(TWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.MWRR, err = vp.PerformanceMetric(MWRR).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Sharpe, err = vp.PerformanceMetric(Sharpe).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Sortino, err = vp.PerformanceMetric(Sortino).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.Calmar, err = vp.PerformanceMetric(Calmar).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.MaxDrawdown, err = vp.PerformanceMetric(MaxDrawdown).Value()
	if err != nil {
		errs = append(errs, err)
	}

	summary.StdDev, err = vp.PerformanceMetric(StdDev).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return summary, errors.Join(errs...)
}

func (vp *viewedPortfolio) RiskMetrics() (RiskMetrics, error) {
	var errs []error

	risk := RiskMetrics{}

	var err error

	risk.Beta, err = vp.PerformanceMetric(Beta).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.Alpha, err = vp.PerformanceMetric(Alpha).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.TrackingError, err = vp.PerformanceMetric(TrackingError).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.DownsideDeviation, err = vp.PerformanceMetric(DownsideDeviation).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.InformationRatio, err = vp.PerformanceMetric(InformationRatio).Value()
	if err != nil {
		errs = append(errs, err)
	}

	risk.Treynor, err = vp.PerformanceMetric(Treynor).Value()
	if err != nil {
		errs = append(errs, err)
	}

	return risk, errors.Join(errs...)
}

func (vp *viewedPortfolio) TaxMetrics() (TaxMetrics, error) {
	return vp.acct.TaxMetrics()
}

func (vp *viewedPortfolio) TradeMetrics() (TradeMetrics, error) {
	return vp.acct.TradeMetrics()
}

func (vp *viewedPortfolio) WithdrawalMetrics() (WithdrawalMetrics, error) {
	return vp.acct.WithdrawalMetrics()
}

func (vp *viewedPortfolio) FactorAnalysis(factors *data.DataFrame) (*FactorRegression, error) {
	return vp.acct.FactorAnalysis(factors)
}

func (vp *viewedPortfolio) StepwiseFactorAnalysis(factors *data.DataFrame) (*StepwiseResult, error) {
	return vp.acct.StepwiseFactorAnalysis(factors)
}

// --- Portfolio interface: point-in-time pass-through methods ---

func (vp *viewedPortfolio) Cash() float64                         { return vp.acct.Cash() }
func (vp *viewedPortfolio) Value() float64                        { return vp.acct.Value() }
func (vp *viewedPortfolio) Position(ast asset.Asset) float64      { return vp.acct.Position(ast) }
func (vp *viewedPortfolio) PositionValue(ast asset.Asset) float64 { return vp.acct.PositionValue(ast) }
func (vp *viewedPortfolio) Holdings() map[asset.Asset]float64     { return vp.acct.Holdings() }
func (vp *viewedPortfolio) Transactions() []Transaction           { return vp.acct.Transactions() }
func (vp *viewedPortfolio) Annotations() []Annotation             { return vp.acct.Annotations() }
func (vp *viewedPortfolio) TradeDetails() []TradeDetail           { return vp.acct.TradeDetails() }
func (vp *viewedPortfolio) Equity() float64                       { return vp.acct.Equity() }
func (vp *viewedPortfolio) LongMarketValue() float64              { return vp.acct.LongMarketValue() }
func (vp *viewedPortfolio) ShortMarketValue() float64             { return vp.acct.ShortMarketValue() }
func (vp *viewedPortfolio) MarginRatio() float64                  { return vp.acct.MarginRatio() }
func (vp *viewedPortfolio) MarginDeficiency() float64             { return vp.acct.MarginDeficiency() }
func (vp *viewedPortfolio) BuyingPower() float64                  { return vp.acct.BuyingPower() }
func (vp *viewedPortfolio) Benchmark() asset.Asset                { return vp.acct.Benchmark() }
func (vp *viewedPortfolio) SetMetadata(key, value string)         { vp.acct.SetMetadata(key, value) }
func (vp *viewedPortfolio) GetMetadata(key string) string         { return vp.acct.GetMetadata(key) }

// Prices returns the windowed price DataFrame.
func (vp *viewedPortfolio) Prices() *data.DataFrame {
	df := vp.acct.Prices()
	if df == nil {
		return nil
	}

	return df.Between(vp.start, vp.end)
}

// PerfData returns the windowed performance DataFrame.
func (vp *viewedPortfolio) PerfData() *data.DataFrame {
	df := vp.acct.PerfData()
	if df == nil {
		return nil
	}

	return df.Between(vp.start, vp.end)
}

// View creates a fresh view of the underlying account with the given range.
func (vp *viewedPortfolio) View(start, end time.Time) Portfolio {
	return vp.acct.View(start, end)
}

// --- PortfolioStats interface: delegated to windowedStats ---

func (vp *viewedPortfolio) Returns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.Returns(ctx, window)
}

func (vp *viewedPortfolio) ExcessReturns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.ExcessReturns(ctx, window)
}

func (vp *viewedPortfolio) Drawdown(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.Drawdown(ctx, window)
}

func (vp *viewedPortfolio) BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.BenchmarkReturns(ctx, window)
}

func (vp *viewedPortfolio) EquitySeries(ctx context.Context, window *Period) *data.DataFrame {
	return vp.stats.EquitySeries(ctx, window)
}

func (vp *viewedPortfolio) PerfDataView(ctx context.Context) *data.DataFrame {
	return vp.stats.PerfDataView(ctx)
}

func (vp *viewedPortfolio) PricesView(ctx context.Context) *data.DataFrame {
	return vp.stats.PricesView(ctx)
}

func (vp *viewedPortfolio) TransactionsView(ctx context.Context) []Transaction {
	return vp.stats.TransactionsView(ctx)
}

func (vp *viewedPortfolio) TradeDetailsView(ctx context.Context) []TradeDetail {
	return vp.stats.TradeDetailsView(ctx)
}

func (vp *viewedPortfolio) TaxLotsView(ctx context.Context) map[asset.Asset][]TaxLot {
	return vp.stats.TaxLotsView(ctx)
}

func (vp *viewedPortfolio) ShortLotsView(ctx context.Context, fn func(asset.Asset, []TaxLot)) {
	vp.stats.ShortLotsView(ctx, fn)
}

func (vp *viewedPortfolio) AnnualReturns(metric data.Metric) ([]int, []float64, error) {
	return vp.stats.AnnualReturns(metric)
}

func (vp *viewedPortfolio) DrawdownDetails(topN int) ([]DrawdownDetail, error) {
	return vp.stats.DrawdownDetails(topN)
}

func (vp *viewedPortfolio) MonthlyReturns(metric data.Metric) ([]int, [][]float64, error) {
	return vp.stats.MonthlyReturns(metric)
}
