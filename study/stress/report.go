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

package stress

import (
	"io"

	"github.com/bytedance/sonic"
)

var nanSafeAPI = sonic.Config{
	EscapeHTML:            true,
	SortMapKeys:           true,
	CompactMarshaler:      true,
	CopyString:            true,
	ValidateString:        true,
	EncodeNullForInfOrNan: true,
}.Froze()

// stressReport implements report.Report for the stress test study.
type stressReport struct {
	Rankings  []scenarioRanking `json:"rankings"`
	Scenarios []scenarioDetail  `json:"scenarios"`
	Summary   string            `json:"summary"`
}

// scenarioRanking is a single row in the ranking table.
type scenarioRanking struct {
	RunName      string  `json:"runName"`
	ScenarioName string  `json:"scenarioName"`
	ErrorMsg     string  `json:"errorMsg,omitempty"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
	TotalReturn  float64 `json:"totalReturn"`
	WorstDay     float64 `json:"worstDay"`
}

// scenarioDetail holds per-scenario metrics across all runs.
type scenarioDetail struct {
	Name       string         `json:"name"`
	DateRange  string         `json:"dateRange"`
	RunMetrics []runMetricSet `json:"runMetrics"`
}

// runMetricSet holds one run's metrics for a single scenario.
type runMetricSet struct {
	RunName     string  `json:"runName"`
	ErrorMsg    string  `json:"errorMsg,omitempty"`
	HasData     bool    `json:"hasData"`
	MaxDrawdown float64 `json:"maxDrawdown"`
	TotalReturn float64 `json:"totalReturn"`
	WorstDay    float64 `json:"worstDay"`
}

func (sr *stressReport) Name() string { return "StressTest" }

func (sr *stressReport) Data(writer io.Writer) error {
	return nanSafeAPI.NewEncoder(writer).Encode(sr)
}
