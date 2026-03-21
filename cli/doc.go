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

// Package cli provides the command-line interface for pvbt strategies.
//
// [Run] is the main entry point for strategy binaries. It builds a Cobra
// command tree with backtest, live, snapshot, and describe subcommands,
// registers CLI flags from the strategy's exported struct fields, and
// handles execution:
//
//	func main() {
//	    cli.Run(&MyStrategy{})
//	}
//
// Strategy parameters declared with pvbt, desc, and default struct tags
// are automatically registered as CLI flags. Universe fields accept
// comma-separated ticker lists. The --preset flag applies named parameter
// presets from the strategy's suggest tags.
//
// # Subcommands
//
// The following subcommands are generated for every strategy:
//
//   - backtest: run the strategy over a historical date range and write
//     results to a SQLite database.
//   - live: run the strategy in real time on its declared schedule.
//   - snapshot: run a backtest and capture all data accesses into a
//     SQLite file for deterministic offline testing.
//   - describe: print strategy metadata, parameters, and presets in
//     human-readable or JSON format.
//
// [RunPVBT] is the entry point for the standalone pvbt tool, which adds
// commands for discovering, installing, and managing community strategies
// from GitHub.
package cli
