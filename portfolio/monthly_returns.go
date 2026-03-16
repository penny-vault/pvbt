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
	"math"
	"sort"

	"github.com/penny-vault/pvbt/data"
)

// MonthlyReturns computes month-over-month percentage returns for the given
// metric. It returns a sorted slice of years and a 2D grid where each row
// corresponds to a year and each column to a month (Jan=0 .. Dec=11).
// Months with no data or no prior month use NaN. Returns nil, nil, nil when
// perfData is nil or empty.
func (a *Account) MonthlyReturns(metric data.Metric) ([]int, [][]float64, error) {
	if a.perfData == nil || a.perfData.Len() == 0 {
		return nil, nil, nil
	}

	monthly := a.perfData.Metrics(metric).Downsample(data.Monthly).Last()
	if monthly.Err() != nil {
		return nil, nil, monthly.Err()
	}

	times := monthly.Times()
	values := monthly.Column(portfolioAsset, metric)

	if len(times) == 0 || len(values) == 0 {
		return nil, nil, nil
	}

	// Build a map of year -> [12]float64 (NaN-initialized).
	yearRows := make(map[int][]float64)

	for idx, timestamp := range times {
		year := timestamp.Year()
		if _, exists := yearRows[year]; !exists {
			row := make([]float64, 12)
			for col := range row {
				row[col] = math.NaN()
			}
			yearRows[year] = row
		}

		month := int(timestamp.Month()) - 1 // 0-indexed

		if idx == 0 {
			// First data point has no prior month.
			yearRows[year][month] = math.NaN()
		} else {
			prev := values[idx-1]
			curr := values[idx]
			if prev != 0 {
				yearRows[year][month] = (curr - prev) / prev
			} else {
				yearRows[year][month] = math.NaN()
			}
		}
	}

	// Sort years.
	years := make([]int, 0, len(yearRows))
	for year := range yearRows {
		years = append(years, year)
	}
	sort.Ints(years)

	grid := make([][]float64, len(years))
	for idx, year := range years {
		grid[idx] = yearRows[year]
	}

	return years, grid, nil
}
