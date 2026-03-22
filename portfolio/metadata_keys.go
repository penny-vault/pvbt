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

// Metadata key constants for storing run-level and strategy-level
// information in the portfolio's metadata map.
const (
	// MetaStrategyName is the human-readable name of the strategy.
	MetaStrategyName = "strategy.name"

	// MetaStrategyShortCode is the abbreviated identifier for the strategy.
	MetaStrategyShortCode = "strategy.shortcode"

	// MetaStrategyVersion is the version string of the strategy.
	MetaStrategyVersion = "strategy.version"

	// MetaStrategyDesc is a brief description of the strategy.
	MetaStrategyDesc = "strategy.description"

	// MetaStrategyBenchmark is the ticker or identifier for the benchmark asset.
	MetaStrategyBenchmark = "strategy.benchmark"

	// MetaRunMode indicates the execution mode (e.g., "backtest" or "live").
	MetaRunMode = "run.mode"

	// MetaRunStart is the start date of the run in RFC 3339 format.
	MetaRunStart = "run.start"

	// MetaRunEnd is the end date of the run in RFC 3339 format.
	MetaRunEnd = "run.end"

	// MetaRunElapsed is the wall-clock duration of the run as a string.
	MetaRunElapsed = "run.elapsed"

	// MetaRunInitialCash is the initial cash balance as a decimal string.
	MetaRunInitialCash = "run.initial_cash"
)
