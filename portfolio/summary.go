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

// Summary contains the headline performance numbers for a portfolio.
// It provides a convenient snapshot of the most commonly referenced
// metrics without calling PerformanceMetric individually for each one.
type Summary struct {
	TWRR        float64 // time-weighted rate of return
	MWRR        float64 // money-weighted rate of return
	Sharpe      float64 // Sharpe ratio
	Sortino     float64 // Sortino ratio
	Calmar      float64 // Calmar ratio
	MaxDrawdown float64 // largest peak-to-trough decline
	StdDev      float64 // standard deviation of monthly returns
}
