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

// Package report builds structured view models from backtest results.
//
// [Build] takes a [portfolio.Portfolio], strategy metadata, and run
// metadata, and produces a [Report] containing all the data needed to
// render a backtest summary: header info, equity curve, return tables,
// risk metrics, drawdown analysis, monthly return heatmap, and trade
// statistics.
//
//	rpt, err := report.Build(result, strategyInfo, report.RunMeta{
//	    Elapsed:     elapsed,
//	    Steps:       steps,
//	    InitialCash: 100000,
//	})
//
// The Report struct is a pure view model with no rendering logic. It
// can be passed to [terminal.Render] for styled terminal output, or
// serialized to JSON for other consumers.
//
// [Report.Warnings] collects any issues encountered during report
// generation, such as insufficient data for certain return windows.
package report
