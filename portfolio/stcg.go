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

type stcgMetric struct{}

func (stcgMetric) Name() string { return "STCG" }

func (stcgMetric) Description() string {
	return "Total realized short-term capital gains from positions held 365 days or fewer. Computed by replaying the transaction log with FIFO lot matching. Taxed as ordinary income."
}

func (stcgMetric) Compute(a *Account, _ *Period) float64 {
	_, stcg, _, _ := realizedGains(a.Transactions())
	return stcg
}

func (stcgMetric) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// STCG is realized short-term capital gains (holding period <= 365 days).
var STCGMetric PerformanceMetric = stcgMetric{}
