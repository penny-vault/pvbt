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
	"github.com/penny-vault/pvbt/data"
)

// AnnualReturns computes year-over-year percentage returns for the given
// metric. It returns a sorted slice of years and a corresponding slice of
// returns. The first year's return is computed relative to the very first
// equity value in the full perfData. Subsequent years use the prior year's
// end-of-year value as baseline. Returns nil, nil, nil when perfData is
// nil or empty.
func (a *Account) AnnualReturns(metric data.Metric) ([]int, []float64, error) {
	if a.perfData == nil || a.perfData.Len() == 0 {
		return nil, nil, nil
	}

	// Get the very first equity value from the full (daily) perfData.
	fullValues := a.perfData.Column(portfolioAsset, metric)
	if len(fullValues) == 0 {
		return nil, nil, nil
	}
	firstValue := fullValues[0]

	// Downsample to yearly end-of-year values.
	yearly := a.perfData.Metrics(metric).Downsample(data.Yearly).Last()
	if yearly.Err() != nil {
		return nil, nil, yearly.Err()
	}

	times := yearly.Times()
	values := yearly.Column(portfolioAsset, metric)

	if len(times) == 0 || len(values) == 0 {
		return nil, nil, nil
	}

	years := make([]int, len(times))
	returns := make([]float64, len(times))

	for idx, timestamp := range times {
		var baseline float64
		if idx == 0 {
			baseline = firstValue
		} else {
			baseline = values[idx-1]
		}

		if baseline != 0 {
			returns[idx] = (values[idx] - baseline) / baseline
		}

		years[idx] = timestamp.Year()
	}

	return years, returns, nil
}
