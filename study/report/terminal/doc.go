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

// Package terminal renders backtest reports to styled terminal output.
//
// [Render] takes a [report.TerminalReport] view model and writes a styled
// summary to the provided [io.Writer] using lipgloss for formatting.
// The output includes the strategy header, equity curve, return tables,
// risk metrics, drawdown analysis, monthly return heatmap, and trade
// statistics:
//
//	err := terminal.Render(rpt, os.Stdout)
package terminal
