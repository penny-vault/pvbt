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

import "github.com/penny-vault/pvbt/data"

type alpha struct{}

func (alpha) Name() string { return "Alpha" }

func (alpha) Description() string {
	return "Annualized excess return above what the CAPM predicts given the portfolio's beta. Positive alpha indicates the portfolio outperformed its risk-adjusted expectation. The \"skill\" component of returns."
}

func (alpha) Compute(a *Account, window *Period) (float64, error) {
	pd := a.PerfData()
	if pd == nil {
		return 0, nil
	}
	rfCol := pd.Column(portfolioAsset, data.PortfolioRiskFree)
	if len(rfCol) == 0 || rfCol[0] == 0 {
		return 0, ErrNoRiskFreeRate
	}
	bmCol := pd.Column(portfolioAsset, data.PortfolioBenchmark)
	if len(bmCol) == 0 || bmCol[0] == 0 {
		return 0, ErrNoBenchmark
	}

	perfDF := pd.Window(window)
	eq := perfDF.Metrics(data.PortfolioEquity)
	eqCol := eq.Column(portfolioAsset, data.PortfolioEquity)
	bmWinCol := perfDF.Column(portfolioAsset, data.PortfolioBenchmark)
	rfWinCol := perfDF.Column(portfolioAsset, data.PortfolioRiskFree)

	if len(eqCol) < 2 || len(bmWinCol) < 2 || len(rfWinCol) < 2 {
		return 0, nil
	}

	portfolioReturn := (eqCol[len(eqCol)-1] / eqCol[0]) - 1
	benchmarkReturn := (bmWinCol[len(bmWinCol)-1] / bmWinCol[0]) - 1
	riskFreeReturn := (rfWinCol[len(rfWinCol)-1] / rfWinCol[0]) - 1

	b, err := Beta.Compute(a, window)
	if err != nil {
		return 0, err
	}

	return portfolioReturn - (riskFreeReturn + b*(benchmarkReturn-riskFreeReturn)), nil
}

func (alpha) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// Alpha is Jensen's alpha: the portfolio's excess return over what CAPM
// would predict given its beta.
var Alpha PerformanceMetric = alpha{}
