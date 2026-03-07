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
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// DataFrame stores contiguous float64 data in column-major order. Each
// column represents a single (Asset, Metric) pair across all timestamps.
// Columns are stored contiguously so that time-series operations and SIMD
// can work on a single uninterrupted []float64 slice.
//
// For a frame with T timestamps, A assets, and M metrics the total length
// of Data is T*A*M. The column for asset index a and metric index m starts
// at offset (a*M + m) * T and runs for T elements.
type DataFrame struct {
	// data holds all values in column-major order: each (asset, metric)
	// column is a contiguous run of T values in chronological order.
	data []float64

	// times lists the timestamps in chronological order.
	times []time.Time

	// assets lists the assets in the order they appear in the data slab.
	assets []asset.Asset

	// metrics lists the metrics in the order they appear in the data slab.
	metrics []Metric

	// assetIndex maps CompositeFigi to the asset's position in assets
	// for O(1) lookup.
	assetIndex map[string]int
}

// NewDataFrame constructs a DataFrame from the given dimensions and data.
// The data slice must have length len(times) * len(assets) * len(metrics),
// laid out in column-major order as described on DataFrame.
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, data []float64) *DataFrame {
	idx := make(map[string]int, len(assets))
	for i, a := range assets {
		idx[a.CompositeFigi] = i
	}

	return &DataFrame{
		data:       data,
		times:      times,
		assets:     assets,
		metrics:    metrics,
		assetIndex: idx,
	}
}

// -- Accessors ---------------------------------------------------------------

// Start returns the earliest timestamp in the frame. Returns the zero
// time if the frame is empty.
func (df *DataFrame) Start() time.Time {
	if len(df.times) == 0 {
		return time.Time{}
	}

	return df.times[0]
}

// End returns the latest timestamp in the frame. Returns the zero
// time if the frame is empty.
func (df *DataFrame) End() time.Time {
	if len(df.times) == 0 {
		return time.Time{}
	}

	return df.times[len(df.times)-1]
}

// Duration returns the time span from the first to the last timestamp.
func (df *DataFrame) Duration() time.Duration { return df.End().Sub(df.Start()) }

// Len returns the number of timestamps.
func (df *DataFrame) Len() int { return len(df.times) }

// ColCount returns the number of columns (assets * metrics).
func (df *DataFrame) ColCount() int { return len(df.assets) * len(df.metrics) }

// Value returns the most recent float64 for the given asset and metric.
func (df *DataFrame) Value(a asset.Asset, m Metric) float64 { return 0 }

// ValueAt returns the float64 for the given asset, metric, and timestamp.
func (df *DataFrame) ValueAt(a asset.Asset, m Metric, t time.Time) float64 { return 0 }

// Column returns the contiguous []float64 slice for the given asset and
// metric. The returned slice shares the underlying Data array and is
// directly compatible with gonum.
func (df *DataFrame) Column(a asset.Asset, m Metric) []float64 { return nil }

// At returns a single-row DataFrame containing all assets and metrics at
// the given timestamp.
func (df *DataFrame) At(t time.Time) *DataFrame { return nil }

// Last returns a single-row DataFrame containing the most recent timestamp.
func (df *DataFrame) Last() *DataFrame { return nil }

// Copy returns a deep copy of the DataFrame. The underlying Data slab is
// duplicated so modifications to the copy do not affect the original.
func (df *DataFrame) Copy() *DataFrame { return nil }

// Table returns an ASCII table representation of the DataFrame for
// debugging and interactive use.
func (df *DataFrame) Table() string { return "" }

// -- Narrowing and filtering -------------------------------------------------

// Assets returns a new DataFrame containing only the specified assets.
func (df *DataFrame) Assets(assets ...asset.Asset) *DataFrame { return nil }

// Metrics returns a new DataFrame containing only the specified metrics.
func (df *DataFrame) Metrics(metrics ...Metric) *DataFrame { return nil }

// Between returns a new DataFrame containing only timestamps within the
// inclusive range [start, end].
func (df *DataFrame) Between(start, end time.Time) *DataFrame { return nil }

// Filter returns a new DataFrame keeping only the timestamps for which fn
// returns true. The function receives the timestamp and a single-row
// DataFrame with all assets and metrics at that point.
func (df *DataFrame) Filter(fn func(t time.Time, row *DataFrame) bool) *DataFrame { return nil }

// Drop removes all timestamps where any value equals val (e.g. NaN).
func (df *DataFrame) Drop(val float64) *DataFrame { return nil }

// -- Mutation ----------------------------------------------------------------

// Insert adds a new column to the DataFrame for the given asset and metric.
// The length of values must equal Len().
func (df *DataFrame) Insert(a asset.Asset, m Metric, values []float64) *DataFrame { return nil }

// -- DataFrame arithmetic (align by asset and metric) ------------------------

// Add returns a new DataFrame with element-wise addition of two DataFrames
// aligned by asset and metric.
func (df *DataFrame) Add(other *DataFrame) *DataFrame { return nil }

// Sub returns a new DataFrame with element-wise subtraction.
func (df *DataFrame) Sub(other *DataFrame) *DataFrame { return nil }

// Mul returns a new DataFrame with element-wise multiplication.
func (df *DataFrame) Mul(other *DataFrame) *DataFrame { return nil }

// Div returns a new DataFrame with element-wise division.
func (df *DataFrame) Div(other *DataFrame) *DataFrame { return nil }

// -- Scalar arithmetic -------------------------------------------------------

// AddScalar adds a constant to every value in the DataFrame.
func (df *DataFrame) AddScalar(f float64) *DataFrame { return nil }

// MulScalar multiplies every value in the DataFrame by a constant.
func (df *DataFrame) MulScalar(f float64) *DataFrame { return nil }

// SubScalar subtracts a constant from every value in the DataFrame.
func (df *DataFrame) SubScalar(f float64) *DataFrame { return nil }

// DivScalar divides every value in the DataFrame by a constant.
func (df *DataFrame) DivScalar(f float64) *DataFrame { return nil }

// -- Aggregation across assets per timestamp ---------------------------------

// Max returns a new DataFrame with the maximum value across all assets for
// each timestamp and metric.
func (df *DataFrame) Max() *DataFrame { return nil }

// Min returns a new DataFrame with the minimum value across all assets for
// each timestamp and metric.
func (df *DataFrame) Min() *DataFrame { return nil }

// IdxMax returns, for each timestamp and metric, the asset that holds the
// maximum value.
func (df *DataFrame) IdxMax() []asset.Asset { return nil }

// -- Common transforms -------------------------------------------------------

// Pct returns the percent change over n periods. If n is omitted it
// defaults to 1.
func (df *DataFrame) Pct(n ...int) *DataFrame { return nil }

// Diff returns the first difference between consecutive values.
func (df *DataFrame) Diff() *DataFrame { return nil }

// Log returns the natural logarithm of every value.
func (df *DataFrame) Log() *DataFrame { return nil }

// CumSum returns the cumulative sum along the time axis for each column.
func (df *DataFrame) CumSum() *DataFrame { return nil }

// Shift shifts every column forward by n periods, filling leading values
// with NaN. Negative n shifts backward.
func (df *DataFrame) Shift(n int) *DataFrame { return nil }

// -- Resampling --------------------------------------------------------------

// Resample converts the DataFrame to a lower frequency by aggregating
// values within each period using the specified method.
func (df *DataFrame) Resample(freq Frequency, agg Aggregation) *DataFrame { return nil }

// -- Windowed operations -----------------------------------------------------

// Rolling returns a RollingDataFrame that applies rolling-window operations
// with a window of n periods.
func (df *DataFrame) Rolling(n int) *RollingDataFrame { return nil }

// -- Extensibility -----------------------------------------------------------

// Apply runs fn on each column and returns a new DataFrame with the
// transformed values. The function receives a contiguous []float64 column
// and must return a slice of the same length.
func (df *DataFrame) Apply(fn func([]float64) []float64) *DataFrame { return nil }

// Reduce runs fn on each column, collapsing it to a single value. The
// result is a single-row DataFrame with the same assets and metrics.
func (df *DataFrame) Reduce(fn func([]float64) float64) *DataFrame { return nil }
