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
	"math"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study/report"
)

// WindowedScore computes the given metric for rp restricted to the
// closed date interval [window.Start, window.End]. It returns NaN if
// the metric cannot be computed (e.g. the window contains no data).
func WindowedScore(rp report.ReportablePortfolio, window DateRange, metric portfolio.PerformanceMetric) float64 {
	val, err := rp.PerformanceMetric(metric).AbsoluteWindow(window.Start, window.End).Value()
	if err != nil {
		return math.NaN()
	}

	return val
}

// WindowedScoreExcluding computes the given metric for rp over window,
// ignoring sub-ranges listed in exclude. When exclude is empty it
// delegates directly to WindowedScore.
//
// Full exclusion filtering (splicing out date ranges from the equity curve
// before computing the metric) will be implemented when KFold end-to-end
// testing is in place. For now, only the delegation path is active.
func WindowedScoreExcluding(rp report.ReportablePortfolio, window DateRange, exclude []DateRange, metric portfolio.PerformanceMetric) float64 {
	if len(exclude) == 0 {
		return WindowedScore(rp, window, metric)
	}

	// TODO: implement actual exclusion filtering by splicing the equity curve.
	return WindowedScore(rp, window, metric)
}
