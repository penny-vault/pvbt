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
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
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
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, data []float64) (*DataFrame, error) {
	expected := len(times) * len(assets) * len(metrics)
	if len(data) != expected {
		return nil, fmt.Errorf("data length %d does not match dimensions %d (times=%d, assets=%d, metrics=%d)",
			len(data), expected, len(times), len(assets), len(metrics))
	}

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
	}, nil
}

// mustNewDataFrame is an internal helper that calls NewDataFrame and panics
// on error. Use only when dimensions are guaranteed correct by construction.
func mustNewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, data []float64) *DataFrame {
	df, err := NewDataFrame(times, assets, metrics, data)
	if err != nil {
		panic("internal error: " + err.Error())
	}
	return df
}

// -- Private helpers ---------------------------------------------------------

func (df *DataFrame) metricIndex(m Metric) (int, bool) {
	for i, met := range df.metrics {
		if met == m {
			return i, true
		}
	}
	return 0, false
}

func (df *DataFrame) colOffset(aIdx, mIdx int) int {
	return (aIdx*len(df.metrics) + mIdx) * len(df.times)
}

func (df *DataFrame) timeIndex(t time.Time) (int, bool) {
	i := sort.Search(len(df.times), func(i int) bool {
		return !df.times[i].Before(t)
	})
	if i < len(df.times) && df.times[i].Equal(t) {
		return i, true
	}
	return 0, false
}

func (df *DataFrame) colSlice(aIdx, mIdx int) []float64 {
	off := df.colOffset(aIdx, mIdx)
	return df.data[off : off+len(df.times)]
}

// CompositeAsset creates an asset representing a pair, with fields
// joined by ":". Used by Covariance for multi-asset results.
func CompositeAsset(a, b asset.Asset) asset.Asset {
	return asset.Asset{
		CompositeFigi: a.CompositeFigi + ":" + b.CompositeFigi,
		Ticker:        a.Ticker + ":" + b.Ticker,
	}
}

// CompositeMetric creates a metric representing a pair, joined by ":".
// Used by Covariance for cross-metric results.
func CompositeMetric(a, b Metric) Metric {
	return Metric(string(a) + ":" + string(b))
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

// Times returns a copy of the timestamp axis.
func (df *DataFrame) Times() []time.Time {
	out := make([]time.Time, len(df.times))
	copy(out, df.times)
	return out
}

// AssetList returns a copy of the asset list.
func (df *DataFrame) AssetList() []asset.Asset {
	out := make([]asset.Asset, len(df.assets))
	copy(out, df.assets)
	return out
}

// MetricList returns a copy of the metric list.
func (df *DataFrame) MetricList() []Metric {
	out := make([]Metric, len(df.metrics))
	copy(out, df.metrics)
	return out
}

// Value returns the most recent float64 for the given asset and metric.
func (df *DataFrame) Value(a asset.Asset, m Metric) float64 {
	aIdx, ok := df.assetIndex[a.CompositeFigi]
	if !ok {
		return math.NaN()
	}

	mIdx, ok := df.metricIndex(m)
	if !ok {
		return math.NaN()
	}

	off := df.colOffset(aIdx, mIdx)
	return df.data[off+len(df.times)-1]
}

// ValueAt returns the float64 for the given asset, metric, and timestamp.
func (df *DataFrame) ValueAt(a asset.Asset, m Metric, t time.Time) float64 {
	aIdx, ok := df.assetIndex[a.CompositeFigi]
	if !ok {
		return math.NaN()
	}

	mIdx, ok := df.metricIndex(m)
	if !ok {
		return math.NaN()
	}

	tIdx, ok := df.timeIndex(t)
	if !ok {
		return math.NaN()
	}

	off := df.colOffset(aIdx, mIdx)
	return df.data[off+tIdx]
}

// Column returns the contiguous []float64 slice for the given asset and
// metric. The returned slice shares the underlying Data array and is
// directly compatible with gonum.
func (df *DataFrame) Column(a asset.Asset, m Metric) []float64 {
	aIdx, ok := df.assetIndex[a.CompositeFigi]
	if !ok {
		return nil
	}

	mIdx, ok := df.metricIndex(m)
	if !ok {
		return nil
	}

	return df.colSlice(aIdx, mIdx)
}

// At returns a single-row DataFrame containing all assets and metrics at
// the given timestamp.
func (df *DataFrame) At(t time.Time) *DataFrame {
	tIdx, ok := df.timeIndex(t)
	if !ok {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcOff := df.colOffset(aIdx, mIdx) + tIdx
			dstOff := aIdx*metricLen + mIdx
			newData[dstOff] = df.data[srcOff]
		}
	}

	times := []time.Time{df.times[tIdx]}
	assets := make([]asset.Asset, assetLen)
	copy(assets, df.assets)
	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, assets, metrics, newData)
}

// Last returns a single-row DataFrame containing the most recent timestamp.
func (df *DataFrame) Last() *DataFrame {
	if len(df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}
	return df.At(df.times[len(df.times)-1])
}

// Copy returns a deep copy of the DataFrame. The underlying Data slab is
// duplicated so modifications to the copy do not affect the original.
func (df *DataFrame) Copy() *DataFrame {
	newData := make([]float64, len(df.data))
	copy(newData, df.data)

	times := make([]time.Time, len(df.times))
	copy(times, df.times)

	assets := make([]asset.Asset, len(df.assets))
	copy(assets, df.assets)

	metrics := make([]Metric, len(df.metrics))
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, assets, metrics, newData)
}

// Table returns an ASCII table representation of the DataFrame for
// debugging and interactive use.
func (df *DataFrame) Table() string {
	if len(df.times) == 0 {
		return "(empty DataFrame)"
	}

	var sb strings.Builder

	// Build header.
	sb.WriteString(fmt.Sprintf("%-20s", "Time"))

	for _, a := range df.assets {
		for _, m := range df.metrics {
			sb.WriteString(fmt.Sprintf(" %15s", a.Ticker+"/"+string(m)))
		}
	}

	sb.WriteString("\n")

	// Build rows.
	for tIdx, t := range df.times {
		sb.WriteString(fmt.Sprintf("%-20s", t.Format("2006-01-02")))

		for aIdx := range df.assets {
			for mIdx := range df.metrics {
				off := df.colOffset(aIdx, mIdx) + tIdx
				sb.WriteString(fmt.Sprintf(" %15.4f", df.data[off]))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// -- Narrowing and filtering -------------------------------------------------

// Assets returns a new DataFrame containing only the specified assets.
func (df *DataFrame) Assets(assets ...asset.Asset) *DataFrame {
	timeLen := len(df.times)
	metricLen := len(df.metrics)

	var matched []asset.Asset
	var matchedIdx []int

	for _, a := range assets {
		if idx, ok := df.assetIndex[a.CompositeFigi]; ok {
			matched = append(matched, df.assets[idx])
			matchedIdx = append(matchedIdx, idx)
		}
	}

	if len(matched) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newData := make([]float64, len(matched)*metricLen*timeLen)

	for newAIdx, oldAIdx := range matchedIdx {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcOff := df.colOffset(oldAIdx, mIdx)
			dstOff := (newAIdx*metricLen + mIdx) * timeLen
			copy(newData[dstOff:dstOff+timeLen], df.data[srcOff:srcOff+timeLen])
		}
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, matched, metrics, newData)
}

// Metrics returns a new DataFrame containing only the specified metrics.
func (df *DataFrame) Metrics(metrics ...Metric) *DataFrame {
	timeLen := len(df.times)
	assetLen := len(df.assets)

	var matched []Metric
	var matchedIdx []int

	for _, m := range metrics {
		if idx, ok := df.metricIndex(m); ok {
			matched = append(matched, m)
			matchedIdx = append(matchedIdx, idx)
		}
	}

	if len(matched) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newMetricLen := len(matched)
	newData := make([]float64, assetLen*newMetricLen*timeLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for newMIdx, oldMIdx := range matchedIdx {
			srcOff := df.colOffset(aIdx, oldMIdx)
			dstOff := (aIdx*newMetricLen + newMIdx) * timeLen
			copy(newData[dstOff:dstOff+timeLen], df.data[srcOff:srcOff+timeLen])
		}
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	assetsCopy := make([]asset.Asset, assetLen)
	copy(assetsCopy, df.assets)

	return mustNewDataFrame(times, assetsCopy, matched, newData)
}

// Between returns a new DataFrame containing only timestamps within the
// inclusive range [start, end].
func (df *DataFrame) Between(start, end time.Time) *DataFrame {
	startIdx := sort.Search(len(df.times), func(i int) bool {
		return !df.times[i].Before(start)
	})

	endIdx := sort.Search(len(df.times), func(i int) bool {
		return df.times[i].After(end)
	})

	if startIdx >= endIdx {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	return df.sliceByTimeIndices(startIdx, endIdx)
}

func (df *DataFrame) sliceByTimeIndices(startIdx, endIdx int) *DataFrame {
	newTimeLen := endIdx - startIdx
	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen*newTimeLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcOff := df.colOffset(aIdx, mIdx) + startIdx
			dstOff := (aIdx*metricLen + mIdx) * newTimeLen
			copy(newData[dstOff:dstOff+newTimeLen], df.data[srcOff:srcOff+newTimeLen])
		}
	}

	times := make([]time.Time, newTimeLen)
	copy(times, df.times[startIdx:endIdx])

	assets := make([]asset.Asset, assetLen)
	copy(assets, df.assets)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, assets, metrics, newData)
}

// Filter returns a new DataFrame keeping only the timestamps for which fn
// returns true. The function receives the timestamp and a single-row
// DataFrame with all assets and metrics at that point.
func (df *DataFrame) Filter(fn func(t time.Time, row *DataFrame) bool) *DataFrame {
	var indices []int

	for tIdx, t := range df.times {
		row := df.At(t)
		if fn(t, row) {
			indices = append(indices, tIdx)
		}
	}

	return df.sliceByIndices(indices)
}

func (df *DataFrame) sliceByIndices(indices []int) *DataFrame {
	if len(indices) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newTimeLen := len(indices)
	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen*newTimeLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcBase := df.colOffset(aIdx, mIdx)
			dstBase := (aIdx*metricLen + mIdx) * newTimeLen

			for newTIdx, oldTIdx := range indices {
				newData[dstBase+newTIdx] = df.data[srcBase+oldTIdx]
			}
		}
	}

	times := make([]time.Time, newTimeLen)
	for i, idx := range indices {
		times[i] = df.times[idx]
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, df.assets)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, assets, metrics, newData)
}

// Drop removes all timestamps where any value equals val (e.g. NaN).
func (df *DataFrame) Drop(val float64) *DataFrame {
	isNaN := math.IsNaN(val)

	return df.Filter(func(_ time.Time, row *DataFrame) bool {
		if isNaN {
			return !floats.HasNaN(row.data)
		}

		for _, v := range row.data {
			if v == val {
				return false
			}
		}

		return true
	})
}

// -- Mutation ----------------------------------------------------------------

// Insert adds or overwrites a column in the DataFrame for the given asset
// and metric. The length of values must equal Len(). Returns an error if
// the values length does not match.
func (df *DataFrame) Insert(a asset.Asset, m Metric, values []float64) error {
	if len(values) != len(df.times) {
		return fmt.Errorf("Insert: values length %d does not match Len() %d", len(values), len(df.times))
	}

	timeLen := len(df.times)

	// Check if asset exists; if not, add it.
	_, assetExists := df.assetIndex[a.CompositeFigi]
	if !assetExists {
		df.assetIndex[a.CompositeFigi] = len(df.assets)
		df.assets = append(df.assets, a)
	}

	// Check if metric exists; if not, add it.
	_, metricExists := df.metricIndex(m)
	if !metricExists {
		df.metrics = append(df.metrics, m)
	}

	// Rebuild data slab if dimensions changed.
	assetLen := len(df.assets)
	metricLen := len(df.metrics)

	if !assetExists || !metricExists {
		oldAssetLen := assetLen
		oldMetricLen := metricLen

		if !assetExists {
			oldAssetLen = assetLen - 1
		}

		if !metricExists {
			oldMetricLen = metricLen - 1
		}

		newData := make([]float64, assetLen*metricLen*timeLen)

		for oldAIdx := 0; oldAIdx < oldAssetLen; oldAIdx++ {
			for oldMIdx := 0; oldMIdx < oldMetricLen; oldMIdx++ {
				oldOff := (oldAIdx*oldMetricLen + oldMIdx) * timeLen
				newOff := (oldAIdx*metricLen + oldMIdx) * timeLen
				copy(newData[newOff:newOff+timeLen], df.data[oldOff:oldOff+timeLen])
			}
		}

		df.data = newData
	}

	// Write the values into the correct column.
	aIdx := df.assetIndex[a.CompositeFigi]
	mIdx, _ := df.metricIndex(m)
	off := df.colOffset(aIdx, mIdx)
	copy(df.data[off:off+timeLen], values)

	return nil
}

// -- DataFrame arithmetic (align by asset and metric) ------------------------

type colPair struct {
	a        asset.Asset
	m        Metric
	selfOff  int
	otherOff int
}

func (df *DataFrame) findIntersection(other *DataFrame) ([]colPair, []asset.Asset, []Metric) {
	var pairs []colPair
	var resAssets []asset.Asset
	assetSeen := make(map[string]bool)
	var resMetrics []Metric
	metricSeen := make(map[Metric]bool)

	for aIdx, a := range df.assets {
		otherAIdx, ok := other.assetIndex[a.CompositeFigi]
		if !ok {
			continue
		}

		for mIdx, m := range df.metrics {
			otherMIdx, ok := other.metricIndex(m)
			if !ok {
				continue
			}

			pairs = append(pairs, colPair{
				a:        a,
				m:        m,
				selfOff:  df.colOffset(aIdx, mIdx),
				otherOff: other.colOffset(otherAIdx, otherMIdx),
			})

			if !assetSeen[a.CompositeFigi] {
				resAssets = append(resAssets, a)
				assetSeen[a.CompositeFigi] = true
			}

			if !metricSeen[m] {
				resMetrics = append(resMetrics, m)
				metricSeen[m] = true
			}
		}
	}

	return pairs, resAssets, resMetrics
}

func (df *DataFrame) elemWiseOp(other *DataFrame, apply func(dst, s, t []float64) []float64) (*DataFrame, error) {
	timeLen := len(df.times)
	if len(other.times) != timeLen {
		return nil, fmt.Errorf("elemWiseOp: timestamp count mismatch: %d vs %d", timeLen, len(other.times))
	}

	// Validate that timestamps match, not just count.
	for i := 0; i < timeLen; i++ {
		if !df.times[i].Equal(other.times[i]) {
			return nil, fmt.Errorf("elemWiseOp: timestamp mismatch at index %d: %s vs %s",
				i, df.times[i].Format(time.RFC3339), other.times[i].Format(time.RFC3339))
		}
	}

	pairs, resAssets, resMetrics := df.findIntersection(other)
	if len(pairs) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil), nil
	}

	resIdx := make(map[string]int, len(resAssets))
	for i, a := range resAssets {
		resIdx[a.CompositeFigi] = i
	}

	resMIdx := make(map[Metric]int, len(resMetrics))
	for i, m := range resMetrics {
		resMIdx[m] = i
	}

	resMetricLen := len(resMetrics)
	newData := make([]float64, len(resAssets)*resMetricLen*timeLen)

	for _, p := range pairs {
		raIdx := resIdx[p.a.CompositeFigi]
		rmIdx := resMIdx[p.m]
		dstOff := (raIdx*resMetricLen + rmIdx) * timeLen
		dst := newData[dstOff : dstOff+timeLen]
		s := df.data[p.selfOff : p.selfOff+timeLen]
		t := other.data[p.otherOff : p.otherOff+timeLen]
		apply(dst, s, t)
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	return mustNewDataFrame(times, resAssets, resMetrics, newData), nil
}

// Add returns a new DataFrame with element-wise addition of two DataFrames
// aligned by asset and metric.
func (df *DataFrame) Add(other *DataFrame) (*DataFrame, error) {
	return df.elemWiseOp(other, floats.AddTo)
}

// Sub returns a new DataFrame with element-wise subtraction.
func (df *DataFrame) Sub(other *DataFrame) (*DataFrame, error) {
	return df.elemWiseOp(other, floats.SubTo)
}

// Mul returns a new DataFrame with element-wise multiplication.
func (df *DataFrame) Mul(other *DataFrame) (*DataFrame, error) {
	return df.elemWiseOp(other, floats.MulTo)
}

// Div returns a new DataFrame with element-wise division.
func (df *DataFrame) Div(other *DataFrame) (*DataFrame, error) {
	return df.elemWiseOp(other, floats.DivTo)
}

// -- Scalar arithmetic -------------------------------------------------------

// AddScalar adds a constant to every value in the DataFrame.
func (df *DataFrame) AddScalar(f float64) *DataFrame {
	result := df.Copy()
	floats.AddConst(f, result.data)
	return result
}

// SubScalar subtracts a constant from every value in the DataFrame.
func (df *DataFrame) SubScalar(f float64) *DataFrame {
	return df.AddScalar(-f)
}

// MulScalar multiplies every value in the DataFrame by a constant.
func (df *DataFrame) MulScalar(f float64) *DataFrame {
	result := df.Copy()
	floats.Scale(f, result.data)
	return result
}

// DivScalar divides every value in the DataFrame by a constant.
func (df *DataFrame) DivScalar(f float64) *DataFrame {
	return df.MulScalar(1.0 / f)
}

// -- Aggregation across assets per timestamp ---------------------------------

// MaxAcrossAssets returns a new DataFrame with the maximum value across all
// assets for each timestamp and metric. The result has a single synthetic
// asset with Ticker "MAX".
func (df *DataFrame) MaxAcrossAssets() *DataFrame {
	timeLen := len(df.times)
	metricLen := len(df.metrics)
	assetLen := len(df.assets)

	synth := asset.Asset{Ticker: "MAX"}
	newData := make([]float64, metricLen*timeLen)

	for mIdx := 0; mIdx < metricLen; mIdx++ {
		dstOff := mIdx * timeLen

		for tIdx := 0; tIdx < timeLen; tIdx++ {
			best := math.Inf(-1)

			for aIdx := 0; aIdx < assetLen; aIdx++ {
				v := df.data[df.colOffset(aIdx, mIdx)+tIdx]
				if v > best {
					best = v
				}
			}

			newData[dstOff+tIdx] = best
		}
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, []asset.Asset{synth}, metrics, newData)
}

// MinAcrossAssets returns a new DataFrame with the minimum value across all
// assets for each timestamp and metric. The result has a single synthetic
// asset with Ticker "MIN".
func (df *DataFrame) MinAcrossAssets() *DataFrame {
	timeLen := len(df.times)
	metricLen := len(df.metrics)
	assetLen := len(df.assets)

	synth := asset.Asset{Ticker: "MIN"}
	newData := make([]float64, metricLen*timeLen)

	for mIdx := 0; mIdx < metricLen; mIdx++ {
		dstOff := mIdx * timeLen

		for tIdx := 0; tIdx < timeLen; tIdx++ {
			best := math.Inf(1)

			for aIdx := 0; aIdx < assetLen; aIdx++ {
				v := df.data[df.colOffset(aIdx, mIdx)+tIdx]
				if v < best {
					best = v
				}
			}

			newData[dstOff+tIdx] = best
		}
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, []asset.Asset{synth}, metrics, newData)
}

// IdxMaxAcrossAssets returns, for each timestamp, the asset that holds the
// maximum value for the first metric across all assets.
func (df *DataFrame) IdxMaxAcrossAssets() []asset.Asset {
	timeLen := len(df.times)
	assetLen := len(df.assets)

	mIdx := 0
	result := make([]asset.Asset, timeLen)

	for tIdx := 0; tIdx < timeLen; tIdx++ {
		best := math.Inf(-1)
		bestIdx := 0

		for aIdx := 0; aIdx < assetLen; aIdx++ {
			v := df.data[df.colOffset(aIdx, mIdx)+tIdx]
			if v > best {
				best = v
				bestIdx = aIdx
			}
		}

		result[tIdx] = df.assets[bestIdx]
	}

	return result
}

// -- Column-wise stats (reduce time dimension) ------------------------------

// Mean returns a single-row DataFrame with the arithmetic mean of each
// column over the time dimension.
func (df *DataFrame) Mean() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		if len(col) == 0 {
			return math.NaN()
		}
		return stat.Mean(col, nil)
	})
}

// Sum returns a single-row DataFrame with the sum of each column over
// the time dimension.
func (df *DataFrame) Sum() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		return floats.Sum(col)
	})
}

// Max returns a single-row DataFrame with the maximum value of each
// column over the time dimension.
func (df *DataFrame) Max() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		if len(col) == 0 {
			return math.NaN()
		}
		return floats.Max(col)
	})
}

// Min returns a single-row DataFrame with the minimum value of each
// column over the time dimension.
func (df *DataFrame) Min() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		if len(col) == 0 {
			return math.NaN()
		}
		return floats.Min(col)
	})
}

// Variance returns a single-row DataFrame with the sample variance (N-1
// denominator) of each column over the time dimension.
func (df *DataFrame) Variance() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		if len(col) < 2 {
			return 0
		}
		m := stat.Mean(col, nil)
		sum := 0.0
		for _, v := range col {
			d := v - m
			sum += d * d
		}
		return sum / float64(len(col)-1)
	})
}

// Std returns a single-row DataFrame with the sample standard deviation
// (N-1 denominator) of each column over the time dimension.
func (df *DataFrame) Std() *DataFrame {
	return df.Reduce(func(col []float64) float64 {
		if len(col) < 2 {
			return 0
		}
		m := stat.Mean(col, nil)
		sum := 0.0
		for _, v := range col {
			d := v - m
			sum += d * d
		}
		return math.Sqrt(sum / float64(len(col)-1))
	})
}

// -- Covariance --------------------------------------------------------------

// Covariance computes sample covariance (N-1 denominator) between columns.
//   - 1 asset: cross-metric covariance. Returns composite metric keys.
//   - 2+ assets: per-metric covariance for all unique pairs. Returns composite asset keys.
func (df *DataFrame) Covariance(assets ...asset.Asset) *DataFrame {
	if len(assets) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	var lastTime []time.Time
	if len(df.times) > 0 {
		lastTime = []time.Time{df.times[len(df.times)-1]}
	}

	if len(assets) == 1 {
		return df.crossMetricCovariance(assets[0], lastTime)
	}

	return df.crossAssetCovariance(assets, lastTime)
}

func (df *DataFrame) crossMetricCovariance(a asset.Asset, lastTime []time.Time) *DataFrame {
	aIdx, ok := df.assetIndex[a.CompositeFigi]
	if !ok {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	metricLen := len(df.metrics)
	var pairMetrics []Metric
	var pairData []float64

	for i := 0; i < metricLen; i++ {
		for j := i + 1; j < metricLen; j++ {
			pairMetrics = append(pairMetrics, CompositeMetric(df.metrics[i], df.metrics[j]))
			pairData = append(pairData, sampleCov(
				df.colSlice(aIdx, i),
				df.colSlice(aIdx, j),
			))
		}
	}

	if len(pairMetrics) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	return mustNewDataFrame(lastTime, []asset.Asset{a}, pairMetrics, pairData)
}

func (df *DataFrame) crossAssetCovariance(assets []asset.Asset, lastTime []time.Time) *DataFrame {
	metricLen := len(df.metrics)

	var pairAssets []asset.Asset
	var pairData []float64

	for i := 0; i < len(assets); i++ {
		aIdxI, okI := df.assetIndex[assets[i].CompositeFigi]
		for j := i + 1; j < len(assets); j++ {
			aIdxJ, okJ := df.assetIndex[assets[j].CompositeFigi]

			if !okI || !okJ {
				continue
			}

			pairAssets = append(pairAssets, CompositeAsset(assets[i], assets[j]))
			for mIdx := 0; mIdx < metricLen; mIdx++ {
				pairData = append(pairData, sampleCov(
					df.colSlice(aIdxI, mIdx),
					df.colSlice(aIdxJ, mIdx),
				))
			}
		}
	}

	if len(pairAssets) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(lastTime, pairAssets, metrics, pairData)
}

func sampleCov(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	if n < 2 {
		return 0
	}
	mx := stat.Mean(x[:n], nil)
	my := stat.Mean(y[:n], nil)
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += (x[i] - mx) * (y[i] - my)
	}
	return sum / float64(n-1)
}

// -- Common transforms -------------------------------------------------------

// Pct returns the percent change over n periods. If n is omitted it
// defaults to 1.
func (df *DataFrame) Pct(n ...int) *DataFrame {
	period := 1
	if len(n) > 0 {
		period = n[0]
	}

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		for i := 0; i < period && i < len(col); i++ {
			out[i] = math.NaN()
		}

		for i := period; i < len(col); i++ {
			out[i] = (col[i] - col[i-period]) / col[i-period]
		}

		return out
	})
}

// Diff returns the first difference between consecutive values.
func (df *DataFrame) Diff() *DataFrame {
	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		if len(col) == 0 {
			return out
		}

		out[0] = math.NaN()
		floats.SubTo(out[1:], col[1:], col[:len(col)-1])

		return out
	})
}

// Log returns the natural logarithm of every value.
func (df *DataFrame) Log() *DataFrame {
	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		for i, v := range col {
			out[i] = math.Log(v)
		}

		return out
	})
}

// CumSum returns the cumulative sum along the time axis for each column.
func (df *DataFrame) CumSum() *DataFrame {
	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		floats.CumSum(out, col)
		return out
	})
}

// Shift shifts every column forward by n periods, filling leading values
// with NaN. Negative n shifts backward.
func (df *DataFrame) Shift(n int) *DataFrame {
	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		for i := range out {
			out[i] = math.NaN()
		}

		if n >= 0 {
			if n < len(col) {
				copy(out[n:], col[:len(col)-n])
			}
		} else {
			abs := -n
			if abs < len(col) {
				copy(out, col[abs:])
			}
		}

		return out
	})
}

// -- Resampling --------------------------------------------------------------

// Downsample returns a DownsampledDataFrame that aggregates values when
// converting to a lower frequency.
func (df *DataFrame) Downsample(freq Frequency) *DownsampledDataFrame {
	return &DownsampledDataFrame{df: df, freq: freq}
}

// Upsample returns an UpsampledDataFrame that fills gaps when converting
// to a higher frequency.
func (df *DataFrame) Upsample(freq Frequency) *UpsampledDataFrame {
	return &UpsampledDataFrame{df: df, freq: freq}
}

func periodChanged(prev, curr time.Time, freq Frequency) bool {
	switch freq {
	case Weekly:
		_, pw := prev.ISOWeek()
		_, cw := curr.ISOWeek()
		return pw != cw
	case Monthly:
		return prev.Month() != curr.Month() || prev.Year() != curr.Year()
	case Quarterly:
		return (prev.Month()-1)/3 != (curr.Month()-1)/3 || prev.Year() != curr.Year()
	case Yearly:
		return prev.Year() != curr.Year()
	default:
		return true
	}
}

// -- Windowed operations -----------------------------------------------------

// Rolling returns a RollingDataFrame that applies rolling-window operations
// with a window of n periods.
func (df *DataFrame) Rolling(n int) *RollingDataFrame {
	return &RollingDataFrame{df: df, window: n}
}

// -- Extensibility -----------------------------------------------------------

// Apply runs fn on each column and returns a new DataFrame with the
// transformed values. The function receives a contiguous []float64 column
// and must return a slice of the same length.
func (df *DataFrame) Apply(fn func([]float64) []float64) *DataFrame {
	timeLen := len(df.times)
	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen*timeLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			col := df.colSlice(aIdx, mIdx)
			transformed := fn(col)
			dstOff := (aIdx*metricLen + mIdx) * timeLen
			copy(newData[dstOff:dstOff+timeLen], transformed)
		}
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	assets := make([]asset.Asset, assetLen)
	copy(assets, df.assets)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(times, assets, metrics, newData)
}

// Reduce runs fn on each column, collapsing it to a single value. The
// result is a single-row DataFrame with the same assets and metrics.
func (df *DataFrame) Reduce(fn func([]float64) float64) *DataFrame {
	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			col := df.colSlice(aIdx, mIdx)
			dstOff := aIdx*metricLen + mIdx
			newData[dstOff] = fn(col)
		}
	}

	var lastTime []time.Time
	if len(df.times) > 0 {
		lastTime = []time.Time{df.times[len(df.times)-1]}
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, df.assets)

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(lastTime, assets, metrics, newData)
}
