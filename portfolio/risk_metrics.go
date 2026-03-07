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

// RiskMetrics contains risk-related measurements for a portfolio,
// capturing how the portfolio behaves relative to its benchmark and
// how much downside risk it carries.
type RiskMetrics struct {
	Beta                 float64 // sensitivity to benchmark movements
	Alpha                float64 // excess return over CAPM prediction
	TrackingError        float64 // std dev of return difference vs benchmark
	DownsideDeviation    float64 // volatility of returns below risk-free rate
	InformationRatio     float64 // active return per unit of tracking error
	Treynor              float64 // excess return per unit of beta
	UlcerIndex           float64 // downside risk based on drawdown depth and duration
	ExcessKurtosis       float64 // tail risk relative to normal distribution
	Skewness             float64 // asymmetry of the return distribution
	RSquared             float64 // how well returns are explained by benchmark
	ValueAtRisk          float64 // maximum expected loss at a confidence level
	UpsideCaptureRatio   float64 // percentage of benchmark gains captured
	DownsideCaptureRatio float64 // percentage of benchmark losses captured
}
