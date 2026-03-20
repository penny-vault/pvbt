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

// DateRangeMode controls how the engine handles a backtest date range
// when warmup data is insufficient.
type DateRangeMode int

const (
	// DateRangeModeStrict returns an error if any asset lacks sufficient
	// warmup data before the requested start date.
	DateRangeModeStrict DateRangeMode = iota

	// DateRangeModePermissive adjusts the start date forward until all
	// assets have sufficient warmup data. Returns an error only if no
	// valid start date exists before the end date.
	DateRangeModePermissive
)
