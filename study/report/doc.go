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

// Package report builds structured, composable reports from backtest results.
//
// [Summary] takes a [ReportablePortfolio] (composing [portfolio.Portfolio]
// and [portfolio.PortfolioStats]) and produces a [Report] containing
// Section primitives (MetricPairs, Table, TimeSeries, Text) that together
// form a complete backtest summary.
//
//	rpt, err := report.Summary(acct)
//	rpt.Render(report.FormatText, os.Stdout)
//
// The Report struct delegates rendering to each Section, supporting
// FormatText, FormatJSON, and FormatHTML output formats.
package report
