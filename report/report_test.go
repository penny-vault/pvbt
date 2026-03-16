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
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/report"
)

var (
	spy = asset.Asset{CompositeFigi: "BBG000BDTBL9", Ticker: "SPY"}
	bm  = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
)

// buildPriceDF constructs a single-timestamp DataFrame with MetricClose
// and AdjClose for the given assets.
func buildPriceDF(timestamp time.Time, assets []asset.Asset, closes []float64) *data.DataFrame {
	adjCloses := make([]float64, len(closes))
	copy(adjCloses, closes)

	vals := make([]float64, 0, len(assets)*2)
	for idx := range assets {
		vals = append(vals, closes[idx])
		vals = append(vals, adjCloses[idx])
	}

	df, err := data.NewDataFrame(
		[]time.Time{timestamp},
		assets,
		[]data.Metric{data.MetricClose, data.AdjClose},
		data.Daily,
		vals,
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
		portfolio.WithBenchmark(bm),
	)

	for _, date := range dates {
		df := buildPriceDF(date, []asset.Asset{spy, bm}, []float64{100.0, 200.0})
		acct.UpdatePrices(df)
	}

	return acct
}

func TestBuildHeader(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	info := engine.StrategyInfo{
		Name:      "TestStrategy",
		Version:   "1.0",
		Benchmark: "SPY",
	}

	meta := report.RunMeta{
		Elapsed:     5 * time.Second,
		Steps:       100,
		InitialCash: 10_000,
	}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if rpt.Header.StrategyName != "TestStrategy" {
		t.Errorf("expected StrategyName=TestStrategy, got %q", rpt.Header.StrategyName)
	}

	if rpt.Header.StrategyVersion != "1.0" {
		t.Errorf("expected StrategyVersion=1.0, got %q", rpt.Header.StrategyVersion)
	}

	if rpt.Header.Benchmark != "SPY" {
		t.Errorf("expected Benchmark=SPY, got %q", rpt.Header.Benchmark)
	}

	if rpt.Header.InitialCash != 10_000 {
		t.Errorf("expected InitialCash=10000, got %f", rpt.Header.InitialCash)
	}

	if rpt.Header.FinalValue != 10_000 {
		t.Errorf("expected FinalValue=10000, got %f", rpt.Header.FinalValue)
	}

	if rpt.Header.Elapsed != 5*time.Second {
		t.Errorf("expected Elapsed=5s, got %v", rpt.Header.Elapsed)
	}

	if rpt.Header.Steps != 100 {
		t.Errorf("expected Steps=100, got %d", rpt.Header.Steps)
	}

	if rpt.Header.StartDate != dates[0] {
		t.Errorf("expected StartDate=%v, got %v", dates[0], rpt.Header.StartDate)
	}

	if rpt.Header.EndDate != dates[2] {
		t.Errorf("expected EndDate=%v, got %v", dates[2], rpt.Header.EndDate)
	}
}

func TestBuildNoBenchmark(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	info := engine.StrategyInfo{Name: "NoBench"}
	meta := report.RunMeta{InitialCash: 10_000}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if rpt.HasBenchmark {
		t.Error("expected HasBenchmark=false for account without benchmark")
	}

	// RiskVsBenchmark should be zero value.
	zero := report.RiskVsBenchmark{}
	if rpt.RiskVsBenchmark != zero {
		t.Errorf("expected zero RiskVsBenchmark, got %+v", rpt.RiskVsBenchmark)
	}
}

func TestBuildWithBenchmark(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithBenchmark(dates, 10_000)

	info := engine.StrategyInfo{Name: "WithBench", Benchmark: "BENCH"}
	meta := report.RunMeta{InitialCash: 10_000}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if !rpt.HasBenchmark {
		t.Error("expected HasBenchmark=true for account with benchmark")
	}
}

func TestBuildInsufficientData(t *testing.T) {
	// Case 1: nil perfData (no UpdatePrices called).
	acct := portfolio.New(portfolio.WithCash(10_000, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)))

	info := engine.StrategyInfo{Name: "Empty"}
	meta := report.RunMeta{InitialCash: 10_000}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(rpt.Warnings) == 0 {
		t.Error("expected at least one warning for nil perfData")
	}

	found := false
	for _, warning := range rpt.Warnings {
		if warning == "insufficient data for full report" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected 'insufficient data for full report' warning, got %v", rpt.Warnings)
	}

	// Header should still be populated.
	if rpt.Header.StrategyName != "Empty" {
		t.Errorf("expected StrategyName=Empty, got %q", rpt.Header.StrategyName)
	}

	if rpt.Header.InitialCash != 10_000 {
		t.Errorf("expected InitialCash=10000, got %f", rpt.Header.InitialCash)
	}

	// Case 2: only one data point (Len < 2).
	dates := []time.Time{time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}
	acctOne := newAccountWithEquity(dates, 10_000)

	rptOne, err := report.Build(acctOne, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	foundOne := false
	for _, warning := range rptOne.Warnings {
		if warning == "insufficient data for full report" {
			foundOne = true
			break
		}
	}

	if !foundOne {
		t.Errorf("expected 'insufficient data for full report' warning for single-point data, got %v", rptOne.Warnings)
	}
}

func TestBuildEquityCurve(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	acct := newAccountWithEquity(dates, 10_000)

	info := engine.StrategyInfo{Name: "EC"}
	meta := report.RunMeta{InitialCash: 10_000}

	rpt, err := report.Build(acct, info, meta)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(rpt.EquityCurve.Times) != 3 {
		t.Errorf("expected 3 equity curve points, got %d", len(rpt.EquityCurve.Times))
	}

	if len(rpt.EquityCurve.StrategyValues) != 3 {
		t.Errorf("expected 3 strategy values, got %d", len(rpt.EquityCurve.StrategyValues))
	}

	// All equity values should be 10_000 since cash never changed.
	for idx, val := range rpt.EquityCurve.StrategyValues {
		if val != 10_000 {
			t.Errorf("StrategyValues[%d] = %f, expected 10000", idx, val)
		}
	}
}
