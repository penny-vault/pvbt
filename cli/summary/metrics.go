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

package summary

import (
	"fmt"
	"math"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// portfolioAsset mirrors the unexported var in portfolio/account.go so that
// we can look up columns in the perfData DataFrame.
var portfolioAsset = asset.Asset{
	CompositeFigi: "_PORTFOLIO_",
	Ticker:        "_PORTFOLIO_",
}

func metricVal(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s: %v", metric.Name(), err))
		return math.NaN()
	}

	return val
}

func metricValBenchmark(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (benchmark): %v", metric.Name(), err))
		return math.NaN()
	}

	return val
}

func metricValWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Window(window).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (window %v): %v", metric.Name(), window, err))
		return math.NaN()
	}

	return val
}

func metricValBenchmarkWindow(acct portfolio.Portfolio, metric portfolio.PerformanceMetric, window portfolio.Period, warnings *[]string) float64 {
	val, err := acct.PerformanceMetric(metric).Benchmark().Window(window).Value()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s (benchmark, window %v): %v", metric.Name(), window, err))
		return math.NaN()
	}

	return val
}
