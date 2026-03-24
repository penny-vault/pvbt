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

package study

import (
	"context"
	"time"

	"github.com/penny-vault/pvbt/report"
)

// RunStatus represents the state of a single run within a study.
type RunStatus int

const (
	RunStarted RunStatus = iota
	RunCompleted
	RunFailed
)

// RunConfig fully specifies what the engine should do for a single run.
type RunConfig struct {
	Name     string
	Start    time.Time
	End      time.Time
	Deposit  float64
	Preset   string
	Params   map[string]string
	Metadata map[string]string
}

// RunResult pairs a config with its outcome.
type RunResult struct {
	Config    RunConfig
	Portfolio report.ReportablePortfolio
	Err       error
}

// Progress is sent on a channel as runs execute.
type Progress struct {
	RunName    string
	RunIndex   int
	TotalRuns  int
	BatchIndex int
	BatchSize  int
	Status     RunStatus
	Err        error
}

// Result is sent on a channel when the study completes.
type Result struct {
	Runs   []RunResult
	Report report.Report
	Err    error
}

// Study is the interface that each study type implements.
type Study interface {
	Name() string
	Description() string
	Configurations(ctx context.Context) ([]RunConfig, error)
	Analyze(results []RunResult) (report.Report, error)
}
