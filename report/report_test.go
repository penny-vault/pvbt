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

package report_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
)

var (
	spy            = asset.Asset{CompositeFigi: "BBG000BDTBL9", Ticker: "SPY"}
	benchmarkAsset = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
)

// buildPriceDF constructs a single-timestamp DataFrame with MetricClose
// and AdjClose for the given assets.
func buildPriceDF(timestamp time.Time, assets []asset.Asset, closes []float64) *data.DataFrame {
	adjCloses := make([]float64, len(closes))
	copy(adjCloses, closes)

	cols := make([][]float64, 0, len(assets)*2)
	for idx := range assets {
		cols = append(cols, []float64{closes[idx]})
		cols = append(cols, []float64{adjCloses[idx]})
	}

	df, err := data.NewDataFrame(
		[]time.Time{timestamp},
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		cols,
	)
	if err != nil {
		panic(err)
	}

	return df
}

// newAccountWithEquity creates an Account with a multi-day equity curve.
// It deposits cash on t0 and calls UpdatePrices for each date so that
// perfData accumulates.
func newAccountWithEquity(dates []time.Time, cash float64, opts ...portfolio.Option) *portfolio.Account {
	allOpts := []portfolio.Option{portfolio.WithCash(cash, dates[0])}
	allOpts = append(allOpts, opts...)
	acct := portfolio.New(allOpts...)

	for _, date := range dates {
		df := buildPriceDF(date, []asset.Asset{spy}, []float64{100.0})
		acct.UpdatePrices(df)
	}

	return acct
}

// newAccountWithBenchmark creates an account that also has a benchmark
// configured, producing perfData with benchmark columns populated.
func newAccountWithBenchmark(dates []time.Time, cash float64) *portfolio.Account {
	acct := portfolio.New(
		portfolio.WithCash(cash, dates[0]),
		portfolio.WithBenchmark(benchmarkAsset),
	)

	for _, date := range dates {
		df := buildPriceDF(date, []asset.Asset{spy, benchmarkAsset}, []float64{100.0, 200.0})
		acct.UpdatePrices(df)
	}

	return acct
}

func TestSummaryHeader(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	acct.SetMetadata(portfolio.MetaStrategyName, "TestStrategy")
	acct.SetMetadata(portfolio.MetaStrategyVersion, "1.0")
	acct.SetMetadata(portfolio.MetaStrategyBenchmark, "SPY")
	acct.SetMetadata(portfolio.MetaRunElapsed, (5 * time.Second).String())
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	// The title should be the strategy name.
	if rpt.Title != "TestStrategy" {
		t.Errorf("expected Title=TestStrategy, got %q", rpt.Title)
	}

	// Render and verify header content appears in text output.
	var buf bytes.Buffer
	if renderErr := rpt.Render(report.FormatText, &buf); renderErr != nil {
		t.Fatalf("Render returned error: %v", renderErr)
	}

	output := buf.String()

	for _, expected := range []string{
		"TestStrategy",
		"1.0",
		"SPY",
		"2024-01-02",
		"2024-01-04",
		"$10000.00",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestSummaryNoBenchmark(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	acct.SetMetadata(portfolio.MetaStrategyName, "NoBench")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	// Should not have a "Risk vs Benchmark" section.
	for _, section := range rpt.Sections {
		if section.Name() == "Risk vs Benchmark" {
			t.Error("expected no 'Risk vs Benchmark' section for account without benchmark")
		}
	}
}

func TestSummaryWithBenchmark(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithBenchmark(dates, 10_000)

	acct.SetMetadata(portfolio.MetaStrategyName, "WithBench")
	acct.SetMetadata(portfolio.MetaStrategyBenchmark, "BENCH")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	// Should have a "Risk vs Benchmark" section.
	var buf bytes.Buffer
	if renderErr := rpt.Render(report.FormatText, &buf); renderErr != nil {
		t.Fatalf("Render returned error: %v", renderErr)
	}

	output := buf.String()
	// Check the section names in the report directly since text rendering
	// does not include section names as headings for MetricPairs.
	foundRvB := false
	for _, section := range rpt.Sections {
		if section.Name() == "Risk vs Benchmark" {
			foundRvB = true

			break
		}
	}

	if !foundRvB {
		sectionNames := make([]string, len(rpt.Sections))
		for idx, section := range rpt.Sections {
			sectionNames[idx] = section.Name()
		}

		t.Errorf("expected 'Risk vs Benchmark' section, got sections: %v\noutput:\n%s", sectionNames, output)
	}
}

func TestSummaryInsufficientData(t *testing.T) {
	// Case 1: nil perfData (no UpdatePrices called).
	acct := portfolio.New(portfolio.WithCash(10_000, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)))

	acct.SetMetadata(portfolio.MetaStrategyName, "Empty")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	var buf bytes.Buffer
	if renderErr := rpt.Render(report.FormatText, &buf); renderErr != nil {
		t.Fatalf("Render returned error: %v", renderErr)
	}

	output := buf.String()
	if !strings.Contains(output, "insufficient data for full report") {
		t.Errorf("expected 'insufficient data for full report' warning, got:\n%s", output)
	}

	// Header data should still be present.
	if !strings.Contains(output, "Empty") {
		t.Errorf("expected strategy name 'Empty' in output, got:\n%s", output)
	}

	// Case 2: only one data point (Len < 2).
	dates := []time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}
	acctOne := newAccountWithEquity(dates, 10_000)

	acctOne.SetMetadata(portfolio.MetaStrategyName, "Single")
	acctOne.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rptOne, err := report.Summary(acctOne)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	var bufOne bytes.Buffer
	if renderErr := rptOne.Render(report.FormatText, &bufOne); renderErr != nil {
		t.Fatalf("Render returned error: %v", renderErr)
	}

	outputOne := bufOne.String()
	if !strings.Contains(outputOne, "insufficient data for full report") {
		t.Errorf("expected 'insufficient data for full report' warning for single-point data, got:\n%s", outputOne)
	}
}

func TestSummaryEquityCurve(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	acct.SetMetadata(portfolio.MetaStrategyName, "EC")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	// Find the Equity Curve section (TimeSeries type).
	found := false
	for _, section := range rpt.Sections {
		if section.Name() == "Equity Curve" && section.Type() == "time_series" {
			found = true

			break
		}
	}

	if !found {
		t.Error("expected an 'Equity Curve' time_series section in the report")
	}
}

func TestSummarySectionCount(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	acct.SetMetadata(portfolio.MetaStrategyName, "Sections")
	acct.SetMetadata(portfolio.MetaRunInitialCash, "10000.00")

	rpt, err := report.Summary(acct)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	// Should have at minimum: Header, Equity Curve, Recent Returns, Returns,
	// Annual Returns, Risk Metrics, Top Drawdowns, Monthly Returns,
	// Trade Summary, plus possibly Recent Trades and Warnings.
	if len(rpt.Sections) < 9 {
		names := make([]string, len(rpt.Sections))
		for idx, section := range rpt.Sections {
			names[idx] = section.Name()
		}

		t.Errorf("expected at least 9 sections, got %d: %v", len(rpt.Sections), names)
	}
}
