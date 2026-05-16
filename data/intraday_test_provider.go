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

package data

import (
	"context"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/rs/zerolog"
)

// IntradayTestProvider is an in-memory IntradayProvider backed by a
// pre-built DataFrame. Used by tests to exercise the intraday data-fetch
// and intra-day firing paths without a ClickHouse connection.
type IntradayTestProvider struct {
	frame *DataFrame
}

// NewIntradayTestProvider returns an IntradayTestProvider that serves
// IntradayFetch from the given frame.
func NewIntradayTestProvider(frame *DataFrame) *IntradayTestProvider {
	return &IntradayTestProvider{frame: frame}
}

// Provides reports no metrics. The engine routes intraday lookbacks to
// any provider implementing IntradayFetch regardless of Provides()
// (the Provides set drives daily metric routing).
func (p *IntradayTestProvider) Provides() []Metric { return nil }

// Close is a no-op.
func (p *IntradayTestProvider) Close() error { return nil }

// Fetch satisfies BatchProvider but is never called for intraday
// lookbacks. It returns an empty DataFrame.
func (p *IntradayTestProvider) Fetch(_ context.Context, _ DataRequest) (*DataFrame, error) {
	empty, err := NewDataFrame(nil, nil, nil, Tick, nil)
	if err != nil {
		return nil, err
	}

	return empty, nil
}

// IntradayFetch returns rows from the stored frame matching the request.
// When timesOfDay is non-empty, only rows whose timestamp's hour:minute
// (in the row's local time) appears in the set are returned, simulating
// the ClickHouse time-of-day predicate pushdown.
func (p *IntradayTestProvider) IntradayFetch(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	start, end time.Time,
	timesOfDay []TimeOfDay,
) (*DataFrame, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().
		Int("assets", len(assets)).
		Int("metrics", len(metrics)).
		Time("start", start).
		Time("end", end).
		Int("times_of_day", len(timesOfDay)).
		Msg("IntradayTestProvider.IntradayFetch")

	if p.frame == nil || p.frame.Len() == 0 {
		empty, err := NewDataFrame(nil, nil, nil, Tick, nil)
		if err != nil {
			return nil, err
		}

		return empty, nil
	}

	result := p.frame.Assets(assets...).Metrics(metrics...).Between(start, end)

	if len(timesOfDay) > 0 {
		result = filterFrameByTimeOfDay(result, timesOfDay)
	}

	return result, nil
}

// filterFrameByTimeOfDay narrows the frame to rows whose timestamps
// match one of the allowed minutes-since-midnight.
func filterFrameByTimeOfDay(df *DataFrame, tods []TimeOfDay) *DataFrame {
	if df == nil || df.Len() == 0 || len(tods) == 0 {
		return df
	}

	allow := make(map[int]bool, len(tods))
	for _, tod := range tods {
		allow[tod.MinutesSinceMidnight()] = true
	}

	keep := make([]time.Time, 0, df.Len())

	for _, t := range df.Times() {
		minutes := t.Hour()*60 + t.Minute()
		if allow[minutes] {
			keep = append(keep, t)
		}
	}

	if len(keep) == 0 {
		empty, err := NewDataFrame(nil, nil, nil, Tick, nil)
		if err != nil {
			panic(err)
		}

		return empty
	}

	sort.Slice(keep, func(i, j int) bool { return keep[i].Before(keep[j]) })

	// Build a result frame by appending row-views via At. Iterate and
	// stitch the columns; this is O(n*m) and only used in tests.
	first := df.At(keep[0])
	for _, t := range keep[1:] {
		first = appendDataFrame(first, df.At(t))
	}

	return first
}

// appendDataFrame stacks two DataFrames vertically (same assets/metrics).
// Test-only utility; not exposed via the public DataFrame API.
func appendDataFrame(top, bottom *DataFrame) *DataFrame {
	if top.Len() == 0 {
		return bottom
	}

	if bottom.Len() == 0 {
		return top
	}

	times := make([]time.Time, 0, top.Len()+bottom.Len())
	times = append(times, top.Times()...)
	times = append(times, bottom.Times()...)

	assets := top.AssetList()
	metrics := top.MetricList()

	cols := make([][]float64, 0, len(assets)*len(metrics))

	for _, aa := range assets {
		for _, mm := range metrics {
			combined := make([]float64, 0, len(times))

			topCol := top.Column(aa, mm)
			if topCol == nil {
				topCol = make([]float64, top.Len())
			}

			botCol := bottom.Column(aa, mm)
			if botCol == nil {
				botCol = make([]float64, bottom.Len())
			}

			combined = append(combined, topCol...)
			combined = append(combined, botCol...)
			cols = append(cols, combined)
		}
	}

	stacked, err := NewDataFrame(times, assets, metrics, Tick, cols)
	if err != nil {
		// Tests only -- panic so a malformed frame is loud.
		panic(err)
	}

	return stacked
}
