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

package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/penny-vault/pvbt/portfolio"
)

// standardWindows returns the fixed set of metric windows.
func standardWindows() []portfolio.Period {
	return []portfolio.Period{
		portfolio.Years(10),
		portfolio.Years(5),
		portfolio.Years(3),
		portfolio.Years(1),
		portfolio.YTD(),
		portfolio.MTD(),
		portfolio.WTD(),
	}
}

// benchmarkStatsProvider is implemented by stats that can return a paired
// benchmark-equity-curve view of themselves. *portfolio.Account satisfies
// this. The engine uses it to emit Benchmark<Name> rows.
type benchmarkStatsProvider interface {
	BenchmarkStats() (portfolio.PortfolioStats, error)
}

// computeMetrics computes all provided metrics on the PortfolioStats for
// the given date across all standard windows plus since-inception. It
// returns the number of metric rows successfully evaluated and appended.
//
// When stats also provides a benchmark stats view (via BenchmarkStats),
// every metric satisfying portfolio.BenchmarkTargetable is also evaluated
// against the benchmark equity curve and emitted under the name
// "Benchmark"+metric.Name(). Rows that return portfolio.ErrInsufficientData
// are omitted entirely so downstream consumers can distinguish "window did
// not span enough data" from a real zero value.
func computeMetrics(stats portfolio.PortfolioStats, date time.Time, metrics []portfolio.PerformanceMetric, appendMetric func(portfolio.MetricRow)) int {
	ctx := context.Background()

	appended := 0

	emit := func(source portfolio.PortfolioStats, metric portfolio.PerformanceMetric, namePrefix string, window *portfolio.Period, label string) {
		val, err := metric.Compute(ctx, source, window)
		if err != nil {
			return
		}

		appendMetric(portfolio.MetricRow{
			Date:   date,
			Name:   namePrefix + metric.Name(),
			Window: label,
			Value:  val,
		})

		appended++
	}

	for _, metric := range metrics {
		emit(stats, metric, "", nil, "since_inception")

		for _, window := range standardWindows() {
			windowCopy := window
			emit(stats, metric, "", &windowCopy, windowLabel(window))
		}
	}

	provider, ok := stats.(benchmarkStatsProvider)
	if !ok {
		return appended
	}

	benchStats, err := provider.BenchmarkStats()
	if err != nil {
		// No benchmark configured -- skip the benchmark pass silently. A
		// missing benchmark is a configuration choice, not an error.
		if errors.Is(err, portfolio.ErrNoBenchmark) {
			return appended
		}

		return appended
	}

	for _, metric := range metrics {
		if _, ok := metric.(portfolio.BenchmarkTargetable); !ok {
			continue
		}

		emit(benchStats, metric, "Benchmark", nil, "since_inception")

		for _, window := range standardWindows() {
			windowCopy := window
			emit(benchStats, metric, "Benchmark", &windowCopy, windowLabel(window))
		}
	}

	return appended
}

// windowLabel returns a human-readable label for a Period.
func windowLabel(period portfolio.Period) string {
	switch period.Unit {
	case portfolio.UnitYear:
		return fmt.Sprintf("%dyr", period.N)
	case portfolio.UnitMonth:
		return fmt.Sprintf("%dmo", period.N)
	case portfolio.UnitDay:
		return fmt.Sprintf("%dd", period.N)
	case portfolio.UnitYTD:
		return "ytd"
	case portfolio.UnitMTD:
		return "mtd"
	case portfolio.UnitWTD:
		return "wtd"
	default:
		return "unknown"
	}
}
