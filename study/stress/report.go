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
	"fmt"
	"io"
	"math"
	"text/tabwriter"

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

// Text writes a human-readable plain-text rendering of the report. The
// output lists each scenario's per-run drawdown, total return, and
// worst single day, then prints the summary line.
func (sr *stressReport) Text(writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "Stress Test (%d scenarios)\n\n", len(sr.Scenarios)); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "Scenario\tRun\t   MaxDD\tReturn\tWorstDay"); err != nil {
		return err
	}

	for _, scenario := range sr.Scenarios {
		header := fmt.Sprintf("%s (%s)", scenario.Name, scenario.DateRange)

		for runIdx, runMetric := range scenario.RunMetrics {
			scenarioCell := header
			if runIdx > 0 {
				scenarioCell = ""
			}

			if runMetric.ErrorMsg != "" {
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t\n", scenarioCell, runMetric.RunName, "error: "+runMetric.ErrorMsg); err != nil {
					return err
				}

				continue
			}

			if !runMetric.HasData {
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t\n", scenarioCell, runMetric.RunName, "no data"); err != nil {
					return err
				}

				continue
			}

			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				scenarioCell,
				runMetric.RunName,
				formatPercent(runMetric.MaxDrawdown),
				formatPercent(runMetric.TotalReturn),
				formatPercent(runMetric.WorstDay),
			); err != nil {
				return err
			}
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	if sr.Summary != "" {
		if _, err := fmt.Fprintf(writer, "\n%s\n", sr.Summary); err != nil {
			return err
		}
	}

	return nil
}

// formatPercent renders a fractional value (0.05 → "  5.00%"). NaN and
// Inf become "    n/a" so the column stays aligned.
func formatPercent(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "    n/a"
	}

	return fmt.Sprintf("%6.2f%%", value*100)
}
