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
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// analyzeResults computes the Monte Carlo report from the collected simulation
// results. Full implementation is provided in Task 6; this stub returns a
// placeholder report.
func analyzeResults(results []study.RunResult, historicalResult report.ReportablePortfolio, ruinThreshold float64) (report.Report, error) {
	return report.Report{Title: "Monte Carlo Simulation"}, nil
}
