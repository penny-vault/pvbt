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

type ltcgMetric struct{}

func (ltcgMetric) Name() string { return "LTCG" }

func (ltcgMetric) Description() string {
	return "Total realized long-term capital gains from positions held longer than 365 days. Computed by replaying the transaction log with FIFO lot matching. Taxed at preferential rates (typically 15-20%)."
}

func (ltcgMetric) Compute(a *Account, _ *Period) (float64, error) {
	ltcg, _, _, _ := realizedGains(a.Transactions())
	return ltcg, nil
}

func (ltcgMetric) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// LTCG is realized long-term capital gains (holding period > 365 days).
var LTCGMetric PerformanceMetric = ltcgMetric{}
