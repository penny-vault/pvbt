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

package study

import (
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study/report"
)

// Metric identifies a performance metric used to score a portfolio over a
// date range. It is an enumeration of the metrics supported by the
// parameter optimisation and validation framework.
type Metric int

const (
	// MetricSharpe scores a portfolio by its annualised Sharpe ratio.
	MetricSharpe Metric = iota
	// MetricCAGR scores a portfolio by its compound annual growth rate.
	MetricCAGR Metric = iota
	// MetricMaxDrawdown scores a portfolio by its maximum drawdown (lower is better; negate if maximising).
	MetricMaxDrawdown Metric = iota
	// MetricSortino scores a portfolio by its Sortino ratio.
	MetricSortino Metric = iota
	// MetricCalmar scores a portfolio by its Calmar ratio.
	MetricCalmar Metric = iota
)

// performanceMetric maps a Metric constant to the corresponding
// portfolio.PerformanceMetric implementation.
func (mt Metric) performanceMetric() portfolio.PerformanceMetric {
	switch mt {
	case MetricSharpe:
		return portfolio.Sharpe
	case MetricCAGR:
		return portfolio.CAGR
	case MetricMaxDrawdown:
		return portfolio.MaxDrawdown
	case MetricSortino:
		return portfolio.Sortino
	case MetricCalmar:
		return portfolio.Calmar
	default:
		panic(fmt.Sprintf("unknown metric: %d", int(mt)))
	}
}

// WindowedScore computes the given metric for rp restricted to the
// closed date interval [window.Start, window.End]. It returns NaN if
// the metric cannot be computed (e.g. the window contains no data).
func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric Metric) float64 {
	val, err := rp.View(window.Start, window.End).PerformanceMetric(metric.performanceMetric()).Value()
	if err != nil {
		return math.NaN()
	}

	return val
}

// WindowedScoreExcluding computes the given metric for rp over window,
// ignoring sub-ranges listed in exclude. It computes the metric on each
// non-excluded segment and returns the duration-weighted average. When
// exclude is empty it delegates directly to WindowedScore.
func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric Metric) float64 {
	if len(exclude) == 0 {
		return WindowedScore(rp, window, metric)
	}

	segments := SubtractRanges(window, exclude)
	if len(segments) == 0 {
		return math.NaN()
	}

	if len(segments) == 1 {
		return WindowedScore(rp, segments[0], metric)
	}

	var totalWeight float64
	var weightedSum float64

	for _, seg := range segments {
		score := WindowedScore(rp, seg, metric)

		// Skip data-poor segments (e.g. too few data points for the metric).
		// The average is computed over segments that produce valid scores.
		if math.IsNaN(score) {
			continue
		}

		weight := seg.End.Sub(seg.Start).Seconds()
		weightedSum += score * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return math.NaN()
	}

	return weightedSum / totalWeight
}
