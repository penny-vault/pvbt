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
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
)

func TestMergeColumns(t *testing.T) {
	times := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
	}

	a1 := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "IBM"}
	a2 := asset.Asset{CompositeFigi: "BBG000BVPV84", Ticker: "AMZN"}

	// Frame 1: Close metric for both assets.
	df1, err := NewDataFrame(times, []asset.Asset{a1, a2}, []Metric{MetricClose}, []float64{
		100, 101, 102, // a1 Close
		200, 201, 202, // a2 Close
	})
	if err != nil {
		t.Fatalf("NewDataFrame df1: %v", err)
	}

	// Frame 2: Open metric for both assets.
	df2, err := NewDataFrame(times, []asset.Asset{a1, a2}, []Metric{MetricOpen}, []float64{
		99, 100, 101, // a1 Open
		199, 200, 201, // a2 Open
	})
	if err != nil {
		t.Fatalf("NewDataFrame df2: %v", err)
	}

	merged, err := MergeColumns(df1, df2)
	if err != nil {
		t.Fatalf("MergeColumns: %v", err)
	}

	// Verify merged DataFrame has both metrics.
	closeCol := merged.Column(a1, MetricClose)
	if closeCol == nil {
		t.Fatal("expected Close column for a1, got nil")
	}
	if closeCol[0] != 100 || closeCol[1] != 101 || closeCol[2] != 102 {
		t.Errorf("a1 Close = %v, want [100 101 102]", closeCol)
	}

	openCol := merged.Column(a1, MetricOpen)
	if openCol == nil {
		t.Fatal("expected Open column for a1, got nil")
	}
	if openCol[0] != 99 || openCol[1] != 100 || openCol[2] != 101 {
		t.Errorf("a1 Open = %v, want [99 100 101]", openCol)
	}

	closeCol2 := merged.Column(a2, MetricClose)
	if closeCol2 == nil {
		t.Fatal("expected Close column for a2, got nil")
	}
	if closeCol2[0] != 200 || closeCol2[1] != 201 || closeCol2[2] != 202 {
		t.Errorf("a2 Close = %v, want [200 201 202]", closeCol2)
	}

	openCol2 := merged.Column(a2, MetricOpen)
	if openCol2 == nil {
		t.Fatal("expected Open column for a2, got nil")
	}
	if openCol2[0] != 199 || openCol2[1] != 200 || openCol2[2] != 201 {
		t.Errorf("a2 Open = %v, want [199 200 201]", openCol2)
	}
}

func TestMergeColumnsEmpty(t *testing.T) {
	merged, err := MergeColumns()
	if err != nil {
		t.Fatalf("MergeColumns empty: %v", err)
	}
	if merged.Len() != 0 {
		t.Errorf("expected Len()=0, got %d", merged.Len())
	}
}

func TestMergeTimes(t *testing.T) {
	a1 := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "IBM"}

	times1 := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	times2 := []time.Time{
		time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
	}

	df1, err := NewDataFrame(times1, []asset.Asset{a1}, []Metric{MetricClose}, []float64{
		100, 101,
	})
	if err != nil {
		t.Fatalf("NewDataFrame df1: %v", err)
	}

	df2, err := NewDataFrame(times2, []asset.Asset{a1}, []Metric{MetricClose}, []float64{
		102, 103,
	})
	if err != nil {
		t.Fatalf("NewDataFrame df2: %v", err)
	}

	merged, err := MergeTimes(df1, df2)
	if err != nil {
		t.Fatalf("MergeTimes: %v", err)
	}

	if merged.Len() != 4 {
		t.Fatalf("expected Len()=4, got %d", merged.Len())
	}

	col := merged.Column(a1, MetricClose)
	if col == nil {
		t.Fatal("expected Close column, got nil")
	}

	want := []float64{100, 101, 102, 103}
	for i, v := range want {
		if col[i] != v {
			t.Errorf("col[%d] = %v, want %v", i, col[i], v)
		}
	}

	// Verify timestamps are in order.
	mergedTimes := merged.Times()
	expectedTimes := append(times1, times2...)
	for i, tm := range expectedTimes {
		if !mergedTimes[i].Equal(tm) {
			t.Errorf("time[%d] = %v, want %v", i, mergedTimes[i], tm)
		}
	}
}

func TestMergeTimesOverlap(t *testing.T) {
	a1 := asset.Asset{CompositeFigi: "BBG000BLNNH6", Ticker: "IBM"}

	times1 := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	times2 := []time.Time{
		time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
	}

	df1, err := NewDataFrame(times1, []asset.Asset{a1}, []Metric{MetricClose}, []float64{
		100, 101,
	})
	if err != nil {
		t.Fatalf("NewDataFrame df1: %v", err)
	}

	df2, err := NewDataFrame(times2, []asset.Asset{a1}, []Metric{MetricClose}, []float64{
		101, 102,
	})
	if err != nil {
		t.Fatalf("NewDataFrame df2: %v", err)
	}

	_, err = MergeTimes(df1, df2)
	if err == nil {
		t.Fatal("expected error for overlapping time ranges, got nil")
	}
}
