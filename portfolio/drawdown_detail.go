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
	"sort"
	"time"

	"github.com/penny-vault/pvbt/data"
)

// DrawdownDetail describes a single peak-to-recovery drawdown period.
type DrawdownDetail struct {
	Start    time.Time // when the drawdown began (peak date)
	Trough   time.Time // date of the deepest point
	Recovery time.Time // date equity returned to peak (zero if unrecovered)
	Depth    float64   // most negative fraction (e.g. -0.167)
	Days     int       // trading days from start to recovery (or end of data)
}

// DrawdownDetails walks the equity curve and identifies distinct drawdown
// periods (peak to recovery). It returns the topN deepest drawdowns sorted
// by depth (most negative first). An unrecovered drawdown at the end of the
// data has Recovery set to the zero time.
func (a *Account) DrawdownDetails(topN int) ([]DrawdownDetail, error) {
	if a.perfData == nil || a.perfData.Len() == 0 {
		return nil, nil
	}

	times := a.perfData.Times()
	equity := a.perfData.Column(portfolioAsset, data.PortfolioEquity)

	if len(times) == 0 || len(equity) == 0 {
		return nil, nil
	}

	var drawdowns []DrawdownDetail

	peak := equity[0]
	peakIdx := 0
	troughVal := equity[0]
	troughIdx := 0
	inDrawdown := false

	for idx := 1; idx < len(equity); idx++ {
		if equity[idx] >= peak {
			// Recovered or set new peak.
			if inDrawdown {
				detail := DrawdownDetail{
					Start:    times[peakIdx],
					Trough:   times[troughIdx],
					Recovery: times[idx],
					Depth:    (troughVal - peak) / peak,
					Days:     idx - peakIdx,
				}
				drawdowns = append(drawdowns, detail)
				inDrawdown = false
			}

			peak = equity[idx]
			peakIdx = idx
			troughVal = equity[idx]
			troughIdx = idx
		} else {
			// In a drawdown.
			inDrawdown = true

			if equity[idx] < troughVal {
				troughVal = equity[idx]
				troughIdx = idx
			}
		}
	}

	// Handle unrecovered drawdown at end of data.
	if inDrawdown {
		detail := DrawdownDetail{
			Start:  times[peakIdx],
			Trough: times[troughIdx],
			// Recovery stays zero time (unrecovered).
			Depth: (troughVal - peak) / peak,
			Days:  len(equity) - 1 - peakIdx,
		}
		drawdowns = append(drawdowns, detail)
	}

	// Sort by depth (most negative first).
	sort.Slice(drawdowns, func(i, j int) bool {
		return drawdowns[i].Depth < drawdowns[j].Depth
	})

	if topN > 0 && topN < len(drawdowns) {
		drawdowns = drawdowns[:topN]
	}

	return drawdowns, nil
}
