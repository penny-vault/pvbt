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

// benchmarkView returns a shallow copy of the Account whose perfData has the
// benchmark equity curve written into the PortfolioEquity column. The
// benchmark values are normalised so the curve starts at the same level as
// the original equity curve. This lets any single-series metric (TWRR,
// Sharpe, etc.) compute against the benchmark without per-metric changes.
func (a *Account) benchmarkView() (*Account, error) {
	perfDF := a.PerfData()
	if perfDF == nil {
		return nil, ErrNoBenchmark
	}

	bmCol := perfDF.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return nil, ErrNoBenchmark
	}

	eqCol := perfDF.Column(portfolioAsset, data.PortfolioEquity)
	if len(eqCol) == 0 || eqCol[0] == 0 {
		return nil, ErrNoBenchmark
	}

	// Normalise: scale benchmark so it starts at the same value as equity.
	scale := eqCol[0] / bmCol[0]
	normalized := make([]float64, len(bmCol))
	for idx, val := range bmCol {
		normalized[idx] = val * scale
	}

	// Build a new perfData with the normalised benchmark as PortfolioEquity.
	newPerfData := perfDF.Copy()
	if err := newPerfData.Insert(portfolioAsset, data.PortfolioEquity, normalized); err != nil {
		return nil, err
	}

	// Shallow copy the account, swap perfData.
	view := *a
	view.perfData = newPerfData

	return &view, nil
}
