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

package report

import (
	"io"
)

// ---------------------------------------------------------------------------
// Header implements Section
// ---------------------------------------------------------------------------

func (hd *Header) Type() string { return "header" }
func (hd *Header) Name() string { return "Header" }

func (hd *Header) Render(format Format, writer io.Writer) error {
	return headerToMetricPairs(*hd).Render(format, writer)
}

// ---------------------------------------------------------------------------
// EquityCurve implements Section
// ---------------------------------------------------------------------------

func (ec *EquityCurve) Type() string { return "equity_curve" }
func (ec *EquityCurve) Name() string { return "Performance" }

func (ec *EquityCurve) Render(format Format, writer io.Writer) error {
	return equityCurveToTimeSeries(*ec).Render(format, writer)
}

// ---------------------------------------------------------------------------
// ReturnTable implements Section
// ---------------------------------------------------------------------------

func (rt *ReturnTable) Type() string { return "return_table" }
func (rt *ReturnTable) Name() string { return rt.SectionName }

func (rt *ReturnTable) Render(format Format, writer io.Writer) error {
	return returnTableToSection(rt.SectionName, *rt, len(rt.Benchmark) > 0).Render(format, writer)
}

// ---------------------------------------------------------------------------
// AnnualReturns implements Section
// ---------------------------------------------------------------------------

func (ar *AnnualReturns) Type() string { return "annual_returns" }
func (ar *AnnualReturns) Name() string { return "Annual Returns" }

func (ar *AnnualReturns) Render(format Format, writer io.Writer) error {
	return annualReturnsToSection(*ar, len(ar.Benchmark) > 0).Render(format, writer)
}

// ---------------------------------------------------------------------------
// Risk implements Section
// ---------------------------------------------------------------------------

func (rk *Risk) Type() string { return "risk" }
func (rk *Risk) Name() string { return "Risk Metrics" }

func (rk *Risk) Render(format Format, writer io.Writer) error {
	return riskToMetricPairs(*rk, rk.HasBenchmark).Render(format, writer)
}

// ---------------------------------------------------------------------------
// RiskVsBenchmark implements Section
// ---------------------------------------------------------------------------

func (rvb *RiskVsBenchmark) Type() string { return "risk_vs_benchmark" }
func (rvb *RiskVsBenchmark) Name() string { return "Risk vs Benchmark" }

func (rvb *RiskVsBenchmark) Render(format Format, writer io.Writer) error {
	return riskVsBenchmarkToMetricPairs(*rvb).Render(format, writer)
}

// ---------------------------------------------------------------------------
// Drawdowns implements Section
// ---------------------------------------------------------------------------

func (dd *Drawdowns) Type() string { return "drawdowns" }
func (dd *Drawdowns) Name() string { return "Top Drawdowns" }

func (dd *Drawdowns) Render(format Format, writer io.Writer) error {
	return drawdownsToSection(*dd).Render(format, writer)
}

// ---------------------------------------------------------------------------
// MonthlyReturns implements Section
// ---------------------------------------------------------------------------

func (mr *MonthlyReturns) Type() string { return "monthly_returns" }
func (mr *MonthlyReturns) Name() string { return "Monthly Returns" }

func (mr *MonthlyReturns) Render(format Format, writer io.Writer) error {
	return monthlyReturnsToSection(*mr).Render(format, writer)
}

// ---------------------------------------------------------------------------
// Trades implements Section
// ---------------------------------------------------------------------------

func (tr *Trades) Type() string { return "trades" }
func (tr *Trades) Name() string { return "Trade Summary" }

func (tr *Trades) Render(format Format, writer io.Writer) error {
	sections := tradesToSections(*tr)
	for _, sec := range sections {
		if err := sec.Render(format, writer); err != nil {
			return err
		}
	}

	return nil
}
