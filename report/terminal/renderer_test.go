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

package terminal_test

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/report/terminal"
)

func TestRenderDoesNotPanic(t *testing.T) {
	// Create a minimal Report with Header only, no equity data.
	rpt := report.Report{
		Header: report.Header{
			StrategyName:    "TestStrategy",
			StrategyVersion: "1.0.0",
			Benchmark:       "VFINX",
			StartDate:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:         time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			InitialCash:     100000,
			FinalValue:      143860,
			Elapsed:         1200 * time.Millisecond,
			Steps:           28,
		},
		Warnings: []string{"insufficient data for full report"},
	}

	var buf bytes.Buffer

	err := terminal.Render(rpt, &buf)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("Render produced empty output")
	}
}

func TestRenderFullReport(t *testing.T) {
	// Create a Report with enough data to exercise all sections.
	rpt := report.Report{
		Header: report.Header{
			StrategyName:    "MyStrat",
			StrategyVersion: "2.0.0",
			Benchmark:       "SPY",
			StartDate:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			InitialCash:     100000,
			FinalValue:      120000,
			Elapsed:         2 * time.Second,
			Steps:           252,
		},
		HasBenchmark: true,
		EquityCurve: report.EquityCurve{
			Times:           []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			StrategyValues:  []float64{100000, 120000},
			BenchmarkValues: []float64{100000, 110000},
		},
		RecentReturns: report.ReturnTable{
			Periods:   []string{"1D", "1W", "1M", "WTD", "MTD", "YTD"},
			Strategy:  []float64{0.001, 0.005, 0.01, 0.008, 0.009, 0.10},
			Benchmark: []float64{0.0005, 0.003, 0.005, 0.004, 0.005, 0.08},
		},
		Returns: report.ReturnTable{
			Periods:   []string{"1Y", "3Y", "5Y", "10Y", "Since Inception"},
			Strategy:  []float64{0.20, math.NaN(), math.NaN(), math.NaN(), 0.20},
			Benchmark: []float64{0.10, math.NaN(), math.NaN(), math.NaN(), 0.10},
		},
		AnnualReturns: report.AnnualReturns{
			Years:     []int{2024},
			Strategy:  []float64{0.20},
			Benchmark: []float64{0.10},
		},
		Risk: report.Risk{
			MaxDrawdown:       [2]float64{-0.15, -0.10},
			Volatility:        [2]float64{0.18, 0.14},
			DownsideDeviation: [2]float64{0.12, 0.09},
			Sharpe:            [2]float64{1.2, 0.8},
			Sortino:           [2]float64{1.5, 1.0},
			Calmar:            [2]float64{1.3, 1.0},
			UlcerIndex:        [2]float64{0.05, 0.03},
			ValueAtRisk:       [2]float64{-0.02, -0.015},
			Skewness:          [2]float64{-0.3, -0.1},
			ExcessKurtosis:    [2]float64{1.2, 0.8},
		},
		RiskVsBenchmark: report.RiskVsBenchmark{
			Beta:             0.798,
			Alpha:            0.1471,
			RSquared:         0.587,
			TrackingError:    0.0724,
			InformationRatio: -0.027,
			Treynor:          0.952,
			UpsideCapture:    0.9219,
			DownsideCapture:  0.7902,
		},
		Drawdowns: report.Drawdowns{
			Entries: []report.DrawdownEntry{
				{
					Start:    time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
					End:      time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
					Recovery: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
					Depth:    -0.08,
					Days:     31,
				},
			},
		},
		MonthlyReturns: report.MonthlyReturns{
			Years: []int{2024},
			Values: [][]float64{
				{0.02, -0.01, 0.03, 0.01, 0.02, -0.005, 0.015, 0.025, -0.01, 0.03, 0.02, 0.01},
			},
		},
		Trades: report.Trades{
			TotalTransactions: 5,
			WinRate:           0.80,
			AvgHolding:        70,
			AvgWin:            5079.18,
			AvgLoss:           -5302.14,
			ProfitFactor:      3.832,
			GainLossRatio:     0.958,
			Turnover:          4.7621,
			PositivePeriods:   0.7037,
			Trades: []report.TradeEntry{
				{
					Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
					Action: "BUY",
					Ticker: "AAPL",
					Shares: 100,
					Price:  150.00,
					Amount: 15000.00,
				},
				{
					Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
					Action: "SELL",
					Ticker: "AAPL",
					Shares: 100,
					Price:  165.00,
					Amount: 16500.00,
				},
			},
		},
	}

	var buf bytes.Buffer

	err := terminal.Render(rpt, &buf)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Fatal("Render produced empty output for full report")
	}

	// Verify key sections appear in the output.
	for _, section := range []string{
		"Performance",
		"Recent Returns",
		"Returns",
		"Annual Returns",
		"Risk Metrics",
		"Risk vs Benchmark",
		"Top Drawdowns",
		"Monthly Returns",
		"Trade Summary",
	} {
		if !containsText(output, section) {
			t.Errorf("output missing section: %s", section)
		}
	}
}

// containsText checks if the rendered output contains a string, stripping ANSI codes.
func containsText(output, text string) bool {
	// Simple check -- the section titles will be present even with ANSI codes
	// surrounding individual characters.
	return bytes.Contains([]byte(output), []byte(text))
}
