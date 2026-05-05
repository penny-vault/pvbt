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
	"github.com/penny-vault/pvbt/data"
)

// windowedStats wraps a PortfolioStats and restricts all DataFrame-returning
// methods to the inclusive date range [start, end] by applying
// DataFrame.Between after the inner stats method returns.
type windowedStats struct {
	inner PortfolioStats
	start time.Time
	end   time.Time
}

// newWindowedStats returns a windowedStats that restricts the inner stats to
// the inclusive date range [start, end].
func newWindowedStats(inner PortfolioStats, start, end time.Time) PortfolioStats {
	return &windowedStats{inner: inner, start: start, end: end}
}

// windowBounds reports the user-supplied [start, end] range so that
// transaction-walking metrics (LTCG, STCG, TaxDrag, dividend metrics) can
// attribute events that fall inside the window even when no equity-curve
// row was recorded at those dates.
func (ws *windowedStats) windowBounds() (time.Time, time.Time) {
	return ws.start, ws.end
}

func (ws *windowedStats) Returns(ctx context.Context, window *Period) *data.DataFrame {
	df := ws.inner.Returns(ctx, window)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) ExcessReturns(ctx context.Context, window *Period) *data.DataFrame {
	df := ws.inner.ExcessReturns(ctx, window)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) Drawdown(ctx context.Context, window *Period) *data.DataFrame {
	df := ws.inner.Drawdown(ctx, window)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) BenchmarkReturns(ctx context.Context, window *Period) *data.DataFrame {
	df := ws.inner.BenchmarkReturns(ctx, window)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) EquitySeries(ctx context.Context, window *Period) *data.DataFrame {
	df := ws.inner.EquitySeries(ctx, window)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) PerfDataView(ctx context.Context) *data.DataFrame {
	df := ws.inner.PerfDataView(ctx)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

func (ws *windowedStats) PricesView(ctx context.Context) *data.DataFrame {
	df := ws.inner.PricesView(ctx)
	if df == nil {
		return nil
	}

	return df.Between(ws.start, ws.end)
}

// The following methods are passed through unchanged because they do not
// return DataFrames that need date-range restriction.

func (ws *windowedStats) TransactionsView(ctx context.Context) []Transaction {
	return ws.inner.TransactionsView(ctx)
}

func (ws *windowedStats) TradeDetailsView(ctx context.Context) []TradeDetail {
	return ws.inner.TradeDetailsView(ctx)
}

func (ws *windowedStats) TaxLotsView(ctx context.Context) map[asset.Asset][]TaxLot {
	return ws.inner.TaxLotsView(ctx)
}

func (ws *windowedStats) ShortLotsView(ctx context.Context, fn func(asset.Asset, []TaxLot)) {
	ws.inner.ShortLotsView(ctx, fn)
}

func (ws *windowedStats) AnnualReturns(metric data.Metric) ([]int, []float64, error) {
	return ws.inner.AnnualReturns(metric)
}

func (ws *windowedStats) DrawdownDetails(topN int) ([]DrawdownDetail, error) {
	return ws.inner.DrawdownDetails(topN)
}

func (ws *windowedStats) MonthlyReturns(metric data.Metric) ([]int, [][]float64, error) {
	return ws.inner.MonthlyReturns(metric)
}
