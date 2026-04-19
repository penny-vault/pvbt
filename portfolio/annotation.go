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

import "time"

// Annotation is a single key-value entry that captures an intermediate
// computation at a point in time. Unlike a justification (a human-readable
// sentence on a [Transaction] explaining why a trade was made), an annotation
// records a raw value -- a momentum score, signal strength, or bond fraction
// -- that helps you debug or understand what the strategy computed at each
// step.
type Annotation struct {
	Timestamp time.Time
	Key       string
	Value     string
	// BatchID is the portfolio batch that was active when the
	// annotation was recorded. Zero means no batch was active.
	BatchID int
}
