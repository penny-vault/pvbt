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

package engine

import "time"

// ProgressEvent reports the engine's position within a backtest run. The
// engine emits one ProgressEvent per simulation step after the step has been
// fully processed (housekeeping, strategy compute if scheduled, equity
// recording, and metric evaluation). Receivers can compute completion
// fraction from either step counts or the date span.
type ProgressEvent struct {
	// Step is the 1-based index of the step that just completed.
	Step int

	// TotalSteps is the total number of simulation steps in the run.
	TotalSteps int

	// Date is the simulation date that just finished processing.
	Date time.Time

	// Start is the first simulation date of the run (after warmup
	// adjustment).
	Start time.Time

	// End is the last simulation date of the run.
	End time.Time

	// MeasurementsEvaluated is the cumulative number of performance metric
	// rows that have been computed and appended to the portfolio since the
	// run began.
	MeasurementsEvaluated int
}

// ProgressCallback receives progress events from the engine during a
// backtest. Callbacks must not block; the engine continues to the next step
// as soon as the callback returns.
type ProgressCallback func(ProgressEvent)
