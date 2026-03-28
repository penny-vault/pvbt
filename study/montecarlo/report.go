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

package montecarlo

import (
	"encoding/json"
	"io"
	"time"
)

// monteCarloReport implements report.Report for the Monte Carlo study.
type monteCarloReport struct {
	// Fan chart data: percentile series keyed by level.
	FanChart fanChartData `json:"fanChart"`

	// Terminal wealth distribution statistics.
	TerminalWealth []terminalStat `json:"terminalWealth"`

	// Confidence intervals on key metrics.
	ConfidenceIntervals []confidenceRow `json:"confidenceIntervals"`

	// Probability of ruin section.
	Ruin ruinData `json:"ruin"`

	// Historical rank (nil if no historical result).
	HistoricalRank *historicalRankData `json:"historicalRank,omitempty"`

	// Summary narrative text.
	Summary string `json:"summary"`
}

type fanChartData struct {
	Times  []time.Time   `json:"times"`
	P5     []float64     `json:"p5"`
	P25    []float64     `json:"p25"`
	P50    []float64     `json:"p50"`
	P75    []float64     `json:"p75"`
	P95    []float64     `json:"p95"`
	Actual *actualSeries `json:"actual,omitempty"`
}

type actualSeries struct {
	Times  []time.Time `json:"times"`
	Values []float64   `json:"values"`
}

type terminalStat struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type confidenceRow struct {
	Metric string  `json:"metric"`
	P5     float64 `json:"p5"`
	P25    float64 `json:"p25"`
	P50    float64 `json:"p50"`
	P75    float64 `json:"p75"`
	P95    float64 `json:"p95"`
}

type ruinData struct {
	Probability    float64 `json:"probability"`
	Threshold      float64 `json:"threshold"`
	MedianDrawdown float64 `json:"medianDrawdown"`
}

type historicalRankData struct {
	TerminalValuePercentile float64 `json:"terminalValuePercentile"`
	TWRRPercentile          float64 `json:"twrrPercentile"`
	MaxDrawdownPercentile   float64 `json:"maxDrawdownPercentile"`
	SharpePercentile        float64 `json:"sharpePercentile"`
}

func (mr *monteCarloReport) Name() string { return "MonteCarlo" }

func (mr *monteCarloReport) Data(writer io.Writer) error {
	return json.NewEncoder(writer).Encode(mr)
}
