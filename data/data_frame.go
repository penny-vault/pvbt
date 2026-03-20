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
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
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

	// err holds the first error encountered during chained operations.
	err error

	// freq holds the data resolution of this DataFrame.
	freq Frequency

	// riskFreeRates holds raw annualized risk-free yields (e.g. DGS3MO)
	// aligned with times. Used by RiskAdjustedPct to subtract the
	// risk-free return. Shared by pointer across same-time-axis transformations.
	riskFreeRates []float64

	// source is the DataSource that populated this DataFrame. Weighting
	// functions and other consumers use it to fetch additional data on demand.
	source DataSource
}

// Err returns the first error encountered during chained operations.
func (df *DataFrame) Err() error { return df.err }

// Frequency returns the data resolution of this DataFrame.
func (df *DataFrame) Frequency() Frequency { return df.freq }

// Source returns the DataSource that populated this DataFrame, or nil.
func (df *DataFrame) Source() DataSource { return df.source }

// SetSource sets the DataSource on this DataFrame.
func (df *DataFrame) SetSource(ds DataSource) { df.source = ds }

// WithErr returns a zero-value DataFrame carrying the given error.
// All accessor methods (Len, AssetList, etc.) return safe defaults on this form.
// Exported so that packages like signal can create error DataFrames.
func WithErr(err error) *DataFrame {
	return &DataFrame{err: err}
}

// SetRiskFreeRates attaches raw annualized risk-free yields to the DataFrame.
// Returns an error if len(rates) != df.Len().
func (df *DataFrame) SetRiskFreeRates(rates []float64) error {
	if len(rates) != len(df.times) {
		return fmt.Errorf("SetRiskFreeRates: length %d does not match time axis length %d", len(rates), len(df.times))
	}

	df.riskFreeRates = rates

	return nil
}

// RiskFreeRates returns the attached cumulative risk-free rate values, or nil
// if none have been set.
func (df *DataFrame) RiskFreeRates() []float64 {
	return df.riskFreeRates
}

// propagateAux copies auxiliary fields (e.g. risk-free rates) from the
// receiver to the target DataFrame. Returns target for chaining.
func (df *DataFrame) propagateAux(target *DataFrame) *DataFrame {
	target.riskFreeRates = df.riskFreeRates
	return target
}

// NewDataFrame constructs a DataFrame from the given dimensions and data.
// The data slice must have length len(times) * len(assets) * len(metrics),
// laid out in column-major order as described on DataFrame.
func NewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, freq Frequency, data []float64) (*DataFrame, error) {
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
		freq:       freq,
	}, nil
}

// mustNewDataFrame is an internal helper that calls NewDataFrame and panics
// on error. Use only when dimensions are guaranteed correct by construction.
func mustNewDataFrame(times []time.Time, assets []asset.Asset, metrics []Metric, freq Frequency, data []float64) *DataFrame {
	df, err := NewDataFrame(times, assets, metrics, freq, data)
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

func (df *DataFrame) timeIndex(timestamp time.Time) (int, bool) {
	if df.freq >= Daily {
		return df.timeIndexByDate(timestamp)
	}

	idx := sort.Search(len(df.times), func(idx int) bool {
		return !df.times[idx].Before(timestamp)
	})
	if idx < len(df.times) && df.times[idx].Equal(timestamp) {
		return idx, true
	}

	return 0, false
}

// timeIndexByDate finds the index whose calendar date matches t,
// ignoring the time-of-day component. Daily data has no meaningful
// time -- the stored hour is an artifact of eodTimestamp.
func (df *DataFrame) timeIndexByDate(t time.Time) (int, bool) {
	tY, tM, tD := t.Date()

	idx := sort.Search(len(df.times), func(idx int) bool {
		sY, sM, sD := df.times[idx].Date()
		// Compare year, then month, then day.
		if sY != tY {
			return sY > tY
		}

		if sM != tM {
			return sM > tM
		}

		return sD >= tD
	})
	if idx < len(df.times) {
		sY, sM, sD := df.times[idx].Date()
		if sY == tY && sM == tM && sD == tD {
			return idx, true
		}
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
func (df *DataFrame) Value(a asset.Asset, metric Metric) float64 {
	if df.err != nil {
		return math.NaN()
	}

	aIdx, found := df.assetIndex[a.CompositeFigi]
	if !found {
		return math.NaN()
	}

	mIdx, found := df.metricIndex(metric)
	if !found {
		return math.NaN()
	}

	off := df.colOffset(aIdx, mIdx)

	return df.data[off+len(df.times)-1]
}

// ValueAt returns the float64 for the given asset, metric, and timestamp.
func (df *DataFrame) ValueAt(a asset.Asset, metric Metric, timestamp time.Time) float64 {
	if df.err != nil {
		return math.NaN()
	}

	aIdx, found := df.assetIndex[a.CompositeFigi]
	if !found {
		return math.NaN()
	}

	mIdx, found := df.metricIndex(metric)
	if !found {
		return math.NaN()
	}

	tIdx, found := df.timeIndex(timestamp)
	if !found {
		return math.NaN()
	}

	off := df.colOffset(aIdx, mIdx)

	return df.data[off+tIdx]
}

// Column returns the contiguous []float64 slice for the given asset and
// metric. The returned slice shares the underlying Data array and is
// directly compatible with gonum.
func (df *DataFrame) Column(a asset.Asset, metric Metric) []float64 {
	if df.err != nil {
		return nil
	}

	aIdx, found := df.assetIndex[a.CompositeFigi]
	if !found {
		return nil
	}

	mIdx, found := df.metricIndex(metric)
	if !found {
		return nil
	}

	return df.colSlice(aIdx, mIdx)
}

// At returns a single-row DataFrame containing all assets and metrics at
// the given timestamp.
func (df *DataFrame) At(t time.Time) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	tIdx, ok := df.timeIndex(t)
	if !ok {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

	result := mustNewDataFrame(times, assets, metrics, df.freq, newData)
	if df.riskFreeRates != nil {
		result.riskFreeRates = []float64{df.riskFreeRates[tIdx]}
	}

	return result
}

// Last returns a single-row DataFrame containing the most recent timestamp.
func (df *DataFrame) Last() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if len(df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	return df.At(df.times[len(df.times)-1])
}

// Copy returns a deep copy of the DataFrame. The underlying Data slab is
// duplicated so modifications to the copy do not affect the original.
func (df *DataFrame) Copy() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	newData := make([]float64, len(df.data))
	copy(newData, df.data)

	times := make([]time.Time, len(df.times))
	copy(times, df.times)

	assets := make([]asset.Asset, len(df.assets))
	copy(assets, df.assets)

	metrics := make([]Metric, len(df.metrics))
	copy(metrics, df.metrics)

	result := mustNewDataFrame(times, assets, metrics, df.freq, newData)
	if df.riskFreeRates != nil {
		rfCopy := make([]float64, len(df.riskFreeRates))
		copy(rfCopy, df.riskFreeRates)
		result.riskFreeRates = rfCopy
	}

	return result
}

// Table returns an ASCII table representation of the DataFrame for
// debugging and interactive use.
func (df *DataFrame) Table() string {
	if df.err != nil {
		return "(error DataFrame)"
	}

	if len(df.times) == 0 {
		return "(empty DataFrame)"
	}

	var builder strings.Builder

	// Build header.
	fmt.Fprintf(&builder, "%-20s", "Time")

	for _, a := range df.assets {
		for _, m := range df.metrics {
			fmt.Fprintf(&builder, " %15s", a.Ticker+"/"+string(m))
		}
	}

	builder.WriteString("\n")

	// Build rows.
	for tIdx, t := range df.times {
		fmt.Fprintf(&builder, "%-20s", t.Format("2006-01-02"))

		for aIdx := range df.assets {
			for mIdx := range df.metrics {
				off := df.colOffset(aIdx, mIdx) + tIdx
				fmt.Fprintf(&builder, " %15.4f", df.data[off])
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// -- Narrowing and filtering -------------------------------------------------

// Assets returns a new DataFrame containing only the specified assets.
func (df *DataFrame) Assets(assets ...asset.Asset) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	timeLen := len(df.times)
	metricLen := len(df.metrics)

	var (
		matched    []asset.Asset
		matchedIdx []int
	)

	for _, a := range assets {
		if idx, ok := df.assetIndex[a.CompositeFigi]; ok {
			matched = append(matched, df.assets[idx])
			matchedIdx = append(matchedIdx, idx)
		}
	}

	if len(matched) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

	return df.propagateAux(mustNewDataFrame(times, matched, metrics, df.freq, newData))
}

// Metrics returns a new DataFrame containing only the specified metrics.
func (df *DataFrame) Metrics(metrics ...Metric) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	timeLen := len(df.times)
	assetLen := len(df.assets)

	var (
		matched    []Metric
		matchedIdx []int
	)

	for _, m := range metrics {
		if idx, ok := df.metricIndex(m); ok {
			matched = append(matched, m)
			matchedIdx = append(matchedIdx, idx)
		}
	}

	if len(matched) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

	return df.propagateAux(mustNewDataFrame(times, assetsCopy, matched, df.freq, newData))
}

// Between returns a new DataFrame containing only timestamps within the
// inclusive range [start, end]. For daily-or-coarser data the comparison
// uses calendar dates only, ignoring the time-of-day component.
func (df *DataFrame) Between(start, end time.Time) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	before, after := timeBefore, timeAfter
	if df.freq >= Daily {
		before, after = dateBefore, dateAfter
	}

	startIdx := sort.Search(len(df.times), func(i int) bool {
		return !before(df.times[i], start)
	})

	endIdx := sort.Search(len(df.times), func(i int) bool {
		return after(df.times[i], end)
	})

	if startIdx >= endIdx {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	return df.sliceByTimeIndices(startIdx, endIdx)
}

func timeBefore(a, b time.Time) bool { return a.Before(b) }
func timeAfter(a, b time.Time) bool  { return a.After(b) }

// dateBefore reports whether a's calendar date is strictly before b's.
func dateBefore(a, b time.Time) bool {
	aY, aM, aD := a.Date()

	bY, bM, bD := b.Date()
	if aY != bY {
		return aY < bY
	}

	if aM != bM {
		return aM < bM
	}

	return aD < bD
}

// dateAfter reports whether a's calendar date is strictly after b's.
func dateAfter(a, b time.Time) bool {
	return dateBefore(b, a)
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

	result := mustNewDataFrame(times, assets, metrics, df.freq, newData)
	if df.riskFreeRates != nil {
		result.riskFreeRates = df.riskFreeRates[startIdx:endIdx]
	}

	return result
}

// Filter returns a new DataFrame keeping only the timestamps for which fn
// returns true. The function receives the timestamp and a single-row
// DataFrame with all assets and metrics at that point.
func (df *DataFrame) Filter(predicate func(t time.Time, row *DataFrame) bool) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	var indices []int

	for tIdx, t := range df.times {
		row := df.At(t)
		if predicate(t, row) {
			indices = append(indices, tIdx)
		}
	}

	return df.sliceByIndices(indices)
}

func (df *DataFrame) sliceByIndices(indices []int) *DataFrame {
	if len(indices) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

	result := mustNewDataFrame(times, assets, metrics, df.freq, newData)
	if df.riskFreeRates != nil {
		rfSlice := make([]float64, newTimeLen)
		for tsIdx, idx := range indices {
			rfSlice[tsIdx] = df.riskFreeRates[idx]
		}

		result.riskFreeRates = rfSlice
	}

	return result
}

// Drop removes all timestamps where any value equals val (e.g. NaN).
func (df *DataFrame) Drop(val float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

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

// RenameMetric returns a new DataFrame with metric old replaced by new.
// Returns a DataFrame with Err set if old is not found or new already exists.
func (df *DataFrame) RenameMetric(old, new Metric) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	oldIdx := -1

	for i, m := range df.metrics {
		if m == old {
			oldIdx = i
		}

		if m == new {
			return WithErr(fmt.Errorf("RenameMetric: metric %q already exists", new))
		}
	}

	if oldIdx == -1 {
		return WithErr(fmt.Errorf("RenameMetric: metric %q not found", old))
	}

	result := df.Copy()
	result.metrics[oldIdx] = new

	return result
}

// -- Mutation ----------------------------------------------------------------

// Insert adds or overwrites a column in the DataFrame for the given asset
// and metric. The length of values must equal Len(). Returns an error if
// the values length does not match.
func (df *DataFrame) Insert(targetAsset asset.Asset, metric Metric, values []float64) error {
	if df.err != nil {
		return df.err
	}

	if len(values) != len(df.times) {
		return fmt.Errorf("Insert: values length %d does not match Len() %d", len(values), len(df.times))
	}

	timeLen := len(df.times)

	// Check if asset exists; if not, add it.
	_, assetExists := df.assetIndex[targetAsset.CompositeFigi]
	if !assetExists {
		df.assetIndex[targetAsset.CompositeFigi] = len(df.assets)
		df.assets = append(df.assets, targetAsset)
	}

	// Check if metric exists; if not, add it.
	_, metricExists := df.metricIndex(metric)
	if !metricExists {
		df.metrics = append(df.metrics, metric)
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
	aIdx := df.assetIndex[targetAsset.CompositeFigi]
	mIdx, _ := df.metricIndex(metric)
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
	var (
		pairs     []colPair
		resAssets []asset.Asset
	)

	assetSeen := make(map[string]bool)

	var resMetrics []Metric

	metricSeen := make(map[Metric]bool)

	for aIdx, currentAsset := range df.assets {
		otherAIdx, ok := other.assetIndex[currentAsset.CompositeFigi]
		if !ok {
			continue
		}

		for mIdx, currentMetric := range df.metrics {
			otherMIdx, ok := other.metricIndex(currentMetric)
			if !ok {
				continue
			}

			pairs = append(pairs, colPair{
				a:        currentAsset,
				m:        currentMetric,
				selfOff:  df.colOffset(aIdx, mIdx),
				otherOff: other.colOffset(otherAIdx, otherMIdx),
			})

			if !assetSeen[currentAsset.CompositeFigi] {
				resAssets = append(resAssets, currentAsset)
				assetSeen[currentAsset.CompositeFigi] = true
			}

			if !metricSeen[currentMetric] {
				resMetrics = append(resMetrics, currentMetric)
				metricSeen[currentMetric] = true
			}
		}
	}

	return pairs, resAssets, resMetrics
}

func (df *DataFrame) elemWiseOp(other *DataFrame, apply func(dst, s, t []float64) []float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if other.err != nil {
		return WithErr(other.err)
	}

	timeLen := len(df.times)
	if len(other.times) != timeLen {
		return WithErr(fmt.Errorf("elemWiseOp: timestamp count mismatch: %d vs %d", timeLen, len(other.times)))
	}

	// Validate that timestamps match, not just count.
	for i := 0; i < timeLen; i++ {
		if !df.times[i].Equal(other.times[i]) {
			return WithErr(fmt.Errorf("elemWiseOp: timestamp mismatch at index %d: %s vs %s",
				i, df.times[i].Format(time.RFC3339), other.times[i].Format(time.RFC3339)))
		}
	}

	pairs, resAssets, resMetrics := df.findIntersection(other)
	if len(pairs) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

	for _, pair := range pairs {
		raIdx := resIdx[pair.a.CompositeFigi]
		rmIdx := resMIdx[pair.m]
		dstOff := (raIdx*resMetricLen + rmIdx) * timeLen
		dst := newData[dstOff : dstOff+timeLen]
		s := df.data[pair.selfOff : pair.selfOff+timeLen]
		t := other.data[pair.otherOff : pair.otherOff+timeLen]
		apply(dst, s, t)
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	return df.propagateAux(mustNewDataFrame(times, resAssets, resMetrics, df.freq, newData))
}

// Add returns a new DataFrame with element-wise addition of two DataFrames
// aligned by asset and metric. If metrics are provided, each named metric
// column from other is broadcast across all columns of df.
func (df *DataFrame) Add(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.AddTo)
	}

	return df.broadcastOp(other, metrics, floats.AddTo)
}

// Sub returns a new DataFrame with element-wise subtraction. If metrics are
// provided, each named metric column from other is broadcast across all
// columns of df.
func (df *DataFrame) Sub(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.SubTo)
	}

	return df.broadcastOp(other, metrics, floats.SubTo)
}

// Mul returns a new DataFrame with element-wise multiplication. If metrics
// are provided, each named metric column from other is broadcast across all
// columns of df.
func (df *DataFrame) Mul(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.MulTo)
	}

	return df.broadcastOp(other, metrics, floats.MulTo)
}

// Div returns a new DataFrame with element-wise division. If metrics are
// provided, each named metric column from other is broadcast across all
// columns of df.
func (df *DataFrame) Div(other *DataFrame, metrics ...Metric) *DataFrame {
	if len(metrics) == 0 {
		return df.elemWiseOp(other, floats.DivTo)
	}

	return df.broadcastOp(other, metrics, floats.DivTo)
}

// broadcastOp applies apply(dst, dst, otherCol) for each named metric column
// in other against every column in df. The result is a copy of df with the
// operation applied in-place. Metrics are applied sequentially so chaining
// multiple metrics is equivalent to calling the operation once per metric.
func (df *DataFrame) broadcastOp(other *DataFrame, metrics []Metric, apply func(dst, s, t []float64) []float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if other.err != nil {
		return WithErr(other.err)
	}

	timeLen := len(df.times)
	if len(other.times) != timeLen {
		return WithErr(fmt.Errorf("broadcastOp: timestamp count mismatch: %d vs %d", timeLen, len(other.times)))
	}

	for i := 0; i < timeLen; i++ {
		if !df.times[i].Equal(other.times[i]) {
			return WithErr(fmt.Errorf("broadcastOp: timestamp mismatch at index %d", i))
		}
	}

	result := df.Copy()

	for _, m := range metrics {
		mIdx, ok := other.metricIndex(m)
		if !ok {
			continue
		}

		for aIdx := 0; aIdx < len(result.assets); aIdx++ {
			otherAIdx, ok := other.assetIndex[result.assets[aIdx].CompositeFigi]
			if !ok {
				continue
			}

			otherCol := other.colSlice(otherAIdx, mIdx)

			for rMIdx := 0; rMIdx < len(result.metrics); rMIdx++ {
				off := result.colOffset(aIdx, rMIdx)
				dst := result.data[off : off+timeLen]
				apply(dst, dst, otherCol)
			}
		}
	}

	return result
}

// -- Scalar arithmetic -------------------------------------------------------

// AddScalar adds a constant to every value in the DataFrame.
func (df *DataFrame) AddScalar(scalar float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	result := df.Copy()
	floats.AddConst(scalar, result.data)

	return result
}

// SubScalar subtracts a constant from every value in the DataFrame.
func (df *DataFrame) SubScalar(scalar float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.AddScalar(-scalar)
}

// MulScalar multiplies every value in the DataFrame by a constant.
func (df *DataFrame) MulScalar(scalar float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	result := df.Copy()
	floats.Scale(scalar, result.data)

	return result
}

// DivScalar divides every value in the DataFrame by a constant.
func (df *DataFrame) DivScalar(scalar float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.MulScalar(1.0 / scalar)
}

// -- Aggregation across assets per timestamp ---------------------------------

// MaxAcrossAssets returns a new DataFrame with the maximum value across all
// assets for each timestamp and metric. The result has a single synthetic
// asset with Ticker "MAX".
func (df *DataFrame) MaxAcrossAssets() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

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

	return df.propagateAux(mustNewDataFrame(times, []asset.Asset{synth}, metrics, df.freq, newData))
}

// MinAcrossAssets returns a new DataFrame with the minimum value across all
// assets for each timestamp and metric. The result has a single synthetic
// asset with Ticker "MIN".
func (df *DataFrame) MinAcrossAssets() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

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

	return df.propagateAux(mustNewDataFrame(times, []asset.Asset{synth}, metrics, df.freq, newData))
}

// CountWhere returns a new single-asset DataFrame with one metric (Count)
// whose value at each timestep is the number of assets where the predicate
// returns true for the given metric. The synthetic asset has Ticker "COUNT".
func (df *DataFrame) CountWhere(metric Metric, predicate func(float64) bool) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	mIdx, found := df.metricIndex(metric)
	if !found {
		return WithErr(fmt.Errorf("CountWhere: metric %q not found", metric))
	}

	timeLen := len(df.times)
	assetLen := len(df.assets)
	newData := make([]float64, timeLen)

	for tIdx := 0; tIdx < timeLen; tIdx++ {
		count := 0.0

		for aIdx := 0; aIdx < assetLen; aIdx++ {
			v := df.data[df.colOffset(aIdx, mIdx)+tIdx]
			if predicate(v) {
				count++
			}
		}

		newData[tIdx] = count
	}

	times := make([]time.Time, timeLen)
	copy(times, df.times)

	synth := asset.Asset{Ticker: "COUNT"}

	return df.propagateAux(mustNewDataFrame(times, []asset.Asset{synth}, []Metric{Count}, df.freq, newData))
}

// IdxMaxAcrossAssets returns, for each timestamp, the asset that holds the
// maximum value for the first metric across all assets.
func (df *DataFrame) IdxMaxAcrossAssets() []asset.Asset {
	if df.err != nil {
		return nil
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.Reduce(func(col []float64) float64 {
		return floats.Sum(col)
	})
}

// Max returns a single-row DataFrame with the maximum value of each
// column over the time dimension.
func (df *DataFrame) Max() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

	if len(assets) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
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

func (df *DataFrame) crossMetricCovariance(targetAsset asset.Asset, lastTime []time.Time) *DataFrame {
	aIdx, ok := df.assetIndex[targetAsset.CompositeFigi]
	if !ok {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	metricLen := len(df.metrics)

	var (
		pairMetrics []Metric
		pairData    []float64
	)

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
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	return mustNewDataFrame(lastTime, []asset.Asset{targetAsset}, pairMetrics, df.freq, pairData)
}

func (df *DataFrame) crossAssetCovariance(assets []asset.Asset, lastTime []time.Time) *DataFrame {
	metricLen := len(df.metrics)

	var (
		pairAssets []asset.Asset
		pairData   []float64
	)

	for assetIdx := 0; assetIdx < len(assets); assetIdx++ {
		aIdxI, okI := df.assetIndex[assets[assetIdx].CompositeFigi]
		for innerIdx := assetIdx + 1; innerIdx < len(assets); innerIdx++ {
			aIdxJ, okJ := df.assetIndex[assets[innerIdx].CompositeFigi]

			if !okI || !okJ {
				continue
			}

			pairAssets = append(pairAssets, CompositeAsset(assets[assetIdx], assets[innerIdx]))
			for mIdx := 0; mIdx < metricLen; mIdx++ {
				pairData = append(pairData, sampleCov(
					df.colSlice(aIdxI, mIdx),
					df.colSlice(aIdxJ, mIdx),
				))
			}
		}
	}

	if len(pairAssets) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	metrics := make([]Metric, metricLen)
	copy(metrics, df.metrics)

	return mustNewDataFrame(lastTime, pairAssets, metrics, df.freq, pairData)
}

func sampleCov(xValues, yValues []float64) float64 {
	count := len(xValues)
	if len(yValues) < count {
		count = len(yValues)
	}

	if count < 2 {
		return 0
	}

	mx := stat.Mean(xValues[:count], nil)
	my := stat.Mean(yValues[:count], nil)

	sum := 0.0
	for idx := 0; idx < count; idx++ {
		sum += (xValues[idx] - mx) * (yValues[idx] - my)
	}

	return sum / float64(count-1)
}

// -- Common transforms -------------------------------------------------------

// Pct returns the percent change over n periods. If n is omitted it
// defaults to 1.
func (df *DataFrame) Pct(periods ...int) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	period := 1
	if len(periods) > 0 {
		period = periods[0]
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

// RiskAdjustedPct returns the risk-adjusted percent change over n periods.
// It subtracts the risk-free return over the same period from each column's
// percent change. If no risk-free rates are attached, sets an error on the
// returned DataFrame. Default period is 1.
func (df *DataFrame) RiskAdjustedPct(periods ...int) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if df.riskFreeRates == nil {
		return WithErr(errors.New("RiskAdjustedPct: no risk-free rates attached"))
	}

	period := 1
	if len(periods) > 0 {
		period = periods[0]
	}

	rf := df.riskFreeRates
	periodsPerYear := df.freq.PeriodsPerYear()

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		for tsIdx := 0; tsIdx < period && tsIdx < len(col); tsIdx++ {
			out[tsIdx] = math.NaN()
		}

		for tsIdx := period; tsIdx < len(col); tsIdx++ {
			pctChange := (col[tsIdx] - col[tsIdx-period]) / col[tsIdx-period]

			// Sum the raw annualized yields over the n-period window,
			// then convert from annualized percent to per-period decimal.
			rfSum := 0.0
			for windowIdx := tsIdx - period + 1; windowIdx <= tsIdx; windowIdx++ {
				rfSum += rf[windowIdx]
			}

			out[tsIdx] = pctChange - rfSum/periodsPerYear/100.0
		}

		return out
	})
}

// Diff returns the first difference between consecutive values.
func (df *DataFrame) Diff() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

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
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		floats.CumSum(out, col)

		return out
	})
}

// CumMax returns the running maximum along the time axis for each column.
func (df *DataFrame) CumMax() *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		if len(col) == 0 {
			return out
		}

		out[0] = col[0]
		for i := 1; i < len(col); i++ {
			if col[i] > out[i-1] {
				out[i] = col[i]
			} else {
				out[i] = out[i-1]
			}
		}

		return out
	})
}

// Shift shifts every column forward by n periods, filling leading values
// with NaN. Negative n shifts backward.
func (df *DataFrame) Shift(positions int) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	return df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))

		for i := range out {
			out[i] = math.NaN()
		}

		if positions >= 0 {
			if positions < len(col) {
				copy(out[positions:], col[:len(col)-positions])
			}
		} else {
			abs := -positions
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

// Window returns a DataFrame containing only timestamps within the
// trailing window defined by p. When p is nil, returns the full DataFrame.
// When the window exceeds the available data, returns the full DataFrame.
func (df *DataFrame) Window(period *Period) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	if period == nil {
		return df.Copy()
	}

	if len(df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	start := period.Before(df.End())
	if !start.After(df.Start()) {
		return df.Copy()
	}

	return df.Between(start, df.End())
}

// Rolling returns a RollingDataFrame that applies rolling-window operations
// with a window of n periods.
func (df *DataFrame) Rolling(n int) *RollingDataFrame {
	return &RollingDataFrame{df: df, window: n}
}

// -- Extensibility -----------------------------------------------------------

// Apply runs fn on each column and returns a new DataFrame with the
// transformed values. The function receives a contiguous []float64 column
// and must return a slice of the same length.
func (df *DataFrame) Apply(transform func([]float64) []float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	timeLen := len(df.times)
	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen*timeLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			col := df.colSlice(aIdx, mIdx)
			transformed := transform(col)
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

	return df.propagateAux(mustNewDataFrame(times, assets, metrics, df.freq, newData))
}

// AppendRow appends a single timestamp and its column values to the
// DataFrame in place. The values slice must have length len(assets) *
// len(metrics), ordered as [asset0_metric0, asset0_metric1, ...,
// asset1_metric0, ...] (matching column-major layout). Returns an error
// if the timestamp is not after the current End() or if the values
// length is wrong.
//
// AppendRow is the only DataFrame method that mutates in place. This is
// safe because all read methods produce independent copies via make+copy.
func (df *DataFrame) AppendRow(timestamp time.Time, values []float64) error {
	if df.err != nil {
		return df.err
	}

	colCount := len(df.assets) * len(df.metrics)
	if len(values) != colCount {
		return fmt.Errorf("AppendRow: values length %d does not match column count %d", len(values), colCount)
	}

	if len(df.times) > 0 && !timestamp.After(df.End()) {
		return fmt.Errorf("AppendRow: timestamp %s is not after current End() %s (must be chronological)",
			timestamp.Format(time.RFC3339), df.End().Format(time.RFC3339))
	}

	// The data slab is column-major: each column is contiguous.
	// To append one row, we need to insert one value at the end of each
	// column. This requires rebuilding the slab because columns shift.
	oldT := len(df.times)
	newT := oldT + 1
	metricLen := len(df.metrics)

	newData := make([]float64, colCount*newT)

	for aIdx := 0; aIdx < len(df.assets); aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			oldOff := (aIdx*metricLen + mIdx) * oldT
			newOff := (aIdx*metricLen + mIdx) * newT
			copy(newData[newOff:newOff+oldT], df.data[oldOff:oldOff+oldT])
			newData[newOff+oldT] = values[aIdx*metricLen+mIdx]
		}
	}

	df.data = newData
	df.times = append(df.times, timestamp)
	df.riskFreeRates = nil

	return nil
}

// Reduce runs fn on each column, collapsing it to a single value. The
// result is a single-row DataFrame with the same assets and metrics.
func (df *DataFrame) Reduce(reducer func([]float64) float64) *DataFrame {
	if df.err != nil {
		return WithErr(df.err)
	}

	assetLen := len(df.assets)
	metricLen := len(df.metrics)
	newData := make([]float64, assetLen*metricLen)

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			col := df.colSlice(aIdx, mIdx)
			dstOff := aIdx*metricLen + mIdx
			newData[dstOff] = reducer(col)
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

	return mustNewDataFrame(lastTime, assets, metrics, df.freq, newData)
}

// Annotate pushes every non-NaN cell in the DataFrame as a key-value
// annotation to the destination. Keys are formatted as "TICKER/Metric".
// Values are the float formatted with strconv.FormatFloat(v, 'f', -1, 64).
// Returns the DataFrame for chaining. If the DataFrame has an error,
// this is a no-op.
func (df *DataFrame) Annotate(dest Annotator) *DataFrame {
	if df.err != nil {
		return df
	}

	times := df.Times()
	assets := df.AssetList()
	metrics := df.MetricList()

	for _, timestamp := range times {
		unixSeconds := timestamp.Unix()

		for _, assetItem := range assets {
			for _, metric := range metrics {
				value := df.ValueAt(assetItem, metric, timestamp)
				if !math.IsNaN(value) {
					dest.Annotate(unixSeconds, assetItem.Ticker+"/"+string(metric), strconv.FormatFloat(value, 'f', -1, 64))
				}
			}
		}
	}

	return df
}
