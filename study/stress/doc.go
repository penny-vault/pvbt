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

// Package stress implements the stress test study type. It runs a strategy
// against named historical market scenarios (e.g. the 2008 Financial Crisis,
// COVID Crash, Dot-com Bust) and computes per-scenario metrics such as
// maximum drawdown, total return, and worst single-day return. The results are composed into a report with a comparison table
// ranking scenarios by severity.
//
// Usage:
//
//	scenarios := study.AllScenarios()
//	stressStudy := stress.New(scenarios)
//
//	runner := &study.Runner{
//	    Study:       stressStudy,
//	    NewStrategy: func() engine.Strategy { return &myStrategy{} },
//	    Options:     opts,
//	    Workers:     4,
//	}
//
//	progressCh, resultCh, err := runner.Run(ctx)
package stress
