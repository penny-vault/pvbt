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
	"context"

	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// StressTest implements study.Study by running a strategy over the full date
// range that spans all configured stress scenarios and then slicing the
// resulting equity curve into per-scenario windows for analysis.
type StressTest struct {
	scenarios []study.Scenario
}

// Ensure StressTest satisfies the study.Study interface at compile time.
var _ study.Study = (*StressTest)(nil)

// New creates a StressTest using the provided scenarios. If scenarios is empty
// or nil, study.AllScenarios() is used.
func New(scenarios []study.Scenario) *StressTest {
	if len(scenarios) == 0 {
		scenarios = study.AllScenarios()
	}

	return &StressTest{scenarios: scenarios}
}

// Name returns the human-readable study name.
func (stressTest *StressTest) Name() string { return "Stress Test" }

// Description returns a short explanation of what the study does.
func (stressTest *StressTest) Description() string {
	return "Run strategy against historical market stress scenarios"
}

// Configurations returns a single RunConfig whose date range spans the
// earliest start and latest end across all configured scenarios.
func (stressTest *StressTest) Configurations(_ context.Context) ([]study.RunConfig, error) {
	earliest := stressTest.scenarios[0].Start
	latest := stressTest.scenarios[0].End

	for _, scenario := range stressTest.scenarios[1:] {
		if scenario.Start.Before(earliest) {
			earliest = scenario.Start
		}

		if scenario.End.After(latest) {
			latest = scenario.End
		}
	}

	return []study.RunConfig{
		{
			Name:  "Full Range",
			Start: earliest,
			End:   latest,
			Metadata: map[string]string{
				"study": "stress-test",
			},
		},
	}, nil
}

// Analyze slices each run's equity curve into per-scenario windows and builds
// a report with a ranking table, per-scenario metric pairs, and a summary.
func (stressTest *StressTest) Analyze(results []study.RunResult) (report.ComposableReport, error) {
	return analyzeResults(stressTest.scenarios, results)
}
