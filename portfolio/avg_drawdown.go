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

type avgDrawdown struct{}

func (avgDrawdown) Name() string { return "AvgDrawdown" }

func (avgDrawdown) Description() string {
	return "Average of all drawdown values in the equity curve. Gives a sense of typical drawdown depth over the full history, unlike MaxDrawdown which only captures the worst case."
}

func (avgDrawdown) Compute(a *Account, window *Period) float64 {
	eq := windowSlice(a.EquityCurve(), a.EquityTimes(), window)
	dd := drawdownSeries(eq)
	return mean(dd)
}

func (avgDrawdown) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// AvgDrawdown is the mean loss percentage across all drawdown periods.
// A drawdown is the decline from a peak to a subsequent trough.
// Lower values indicate the portfolio recovers quickly from losses.
var AvgDrawdown PerformanceMetric = avgDrawdown{}
