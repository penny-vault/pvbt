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

type activeReturn struct{}

func (activeReturn) Name() string { return "ActiveReturn" }

func (activeReturn) Description() string {
	return "The difference between portfolio return and benchmark return over the full period. Positive values indicate the portfolio outperformed the benchmark. Unlike Alpha, this is a raw return difference without adjusting for risk."
}

// Compute returns the portfolio total return minus the benchmark total
// return. Total return is (end/start) - 1.
func (activeReturn) Compute(a *Account, window *Period) float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bench := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	if len(equity) < 2 || len(bench) < 2 {
		return 0
	}

	portReturn := (equity[len(equity)-1] / equity[0]) - 1
	benchReturn := (bench[len(bench)-1] / bench[0]) - 1

	return portReturn - benchReturn
}

// ComputeSeries returns the element-wise difference between the
// portfolio cumulative return series and the benchmark cumulative
// return series.
func (activeReturn) ComputeSeries(a *Account, window *Period) []float64 {
	equity := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	bench := windowSlice(a.BenchmarkPrices(), a.EquityTimes(), window)

	portR := returns(equity)
	benchR := returns(bench)

	n := len(portR)
	if len(benchR) < n {
		n = len(benchR)
	}
	if n == 0 {
		return nil
	}

	series := make([]float64, n)
	cumPort := 1.0
	cumBench := 1.0
	for i := 0; i < n; i++ {
		cumPort *= (1 + portR[i])
		cumBench *= (1 + benchR[i])
		series[i] = (cumPort - 1) - (cumBench - 1)
	}

	return series
}

// ActiveReturn is the difference between portfolio return and benchmark
// return (strategy - benchmark). Measures the value added by active
// management. Highly dependent on appropriate benchmark selection.
var ActiveReturn PerformanceMetric = activeReturn{}
