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
	"math"

	"github.com/penny-vault/pvbt/data"
)

type calmar struct{}

func (calmar) Name() string { return "Calmar" }

func (calmar) Description() string {
	return "Annualized return divided by maximum drawdown. Measures return per unit of drawdown risk. Higher values indicate the strategy earns more return for each unit of peak-to-trough decline endured."
}

func (calmar) Compute(ctx context.Context, stats PortfolioStats, window *Period) (float64, error) {
	eqDF := stats.EquitySeries(ctx, window)
	if eqDF == nil {
		return 0, nil
	}

	eqCol := eqDF.Column(portfolioAsset, data.PortfolioEquity)
	eqTimes := eqDF.Times()

	if len(eqCol) < 2 || len(eqTimes) < 2 {
		return 0, nil
	}

	years := eqTimes[len(eqTimes)-1].Sub(eqTimes[0]).Hours() / 24 / 365.25
	if years <= 0 {
		return 0, nil
	}

	annualizedReturn := math.Pow(eqCol[len(eqCol)-1]/eqCol[0], 1.0/years) - 1

	ddDF := stats.Drawdown(ctx, window)
	if ddDF == nil {
		return 0, nil
	}

	ddCol := ddDF.Column(portfolioAsset, data.PortfolioEquity)

	minDD := 0.0
	for _, v := range ddCol {
		if v < minDD {
			minDD = v
		}
	}

	if minDD == 0 {
		return 0, nil
	}

	return annualizedReturn / math.Abs(minDD), nil
}

func (calmar) ComputeSeries(_ context.Context, _ PortfolioStats, _ *Period) (*data.DataFrame, error) {
	return nil, nil
}

// Calmar is the Calmar ratio: annualized return divided by maximum drawdown.
func (calmar) BenchmarkTargetable() {}

func (calmar) HigherIsBetter() bool { return true }

var Calmar PerformanceMetric = calmar{}
