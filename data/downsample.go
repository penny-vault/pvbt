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
	"math"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
)

// DownsampledDataFrame groups timestamps by the target frequency and
// aggregates values within each period. Created by DataFrame.Downsample(freq).
type DownsampledDataFrame struct {
	df   *DataFrame
	freq Frequency
}

func (d *DownsampledDataFrame) aggregate(reducer func([]float64) float64) *DataFrame {
	if len(d.df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, 0, nil)
	}

	type group struct {
		start int
		end   int
	}

	var groups []group

	groupStart := 0

	for i := 1; i < len(d.df.times); i++ {
		if periodChanged(d.df.times[i-1], d.df.times[i], d.freq) {
			groups = append(groups, group{groupStart, i})
			groupStart = i
		}
	}

	groups = append(groups, group{groupStart, len(d.df.times)})

	newTimeLen := len(groups)
	assetLen := len(d.df.assets)
	metricLen := len(d.df.metrics)
	newData := make([]float64, assetLen*metricLen*newTimeLen)
	newTimes := make([]time.Time, newTimeLen)

	for gIdx, timeGroup := range groups {
		newTimes[gIdx] = d.df.times[timeGroup.end-1]

		for aIdx := 0; aIdx < assetLen; aIdx++ {
			for mIdx := 0; mIdx < metricLen; mIdx++ {
				srcOff := d.df.colOffset(aIdx, mIdx)
				vals := d.df.data[srcOff+timeGroup.start : srcOff+timeGroup.end]
				dstOff := (aIdx*metricLen + mIdx) * newTimeLen
				newData[dstOff+gIdx] = reducer(vals)
			}
		}
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, d.df.assets)

	metrics := make([]Metric, metricLen)
	copy(metrics, d.df.metrics)

	result := mustNewDataFrame(newTimes, assets, metrics, d.freq, newData)

	if d.df.riskFreeRates != nil {
		rfRates := make([]float64, newTimeLen)
		for gIdx, timeGroup := range groups {
			// Use the last cumulative risk-free value in each period.
			rfRates[gIdx] = d.df.riskFreeRates[timeGroup.end-1]
		}

		result.riskFreeRates = rfRates
	}

	return result
}

// Mean returns a DataFrame with the mean of each group.
func (d *DownsampledDataFrame) Mean() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return stat.Mean(vals, nil)
	})
}

// Sum returns a DataFrame with the sum of each group.
func (d *DownsampledDataFrame) Sum() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return floats.Sum(vals)
	})
}

// Max returns a DataFrame with the maximum value of each group.
func (d *DownsampledDataFrame) Max() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return floats.Max(vals)
	})
}

// Min returns a DataFrame with the minimum value of each group.
func (d *DownsampledDataFrame) Min() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return floats.Min(vals)
	})
}

// First returns a DataFrame with the first value of each group.
func (d *DownsampledDataFrame) First() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return vals[0]
	})
}

// Last returns a DataFrame with the last value of each group.
func (d *DownsampledDataFrame) Last() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		return vals[len(vals)-1]
	})
}

// Std returns a DataFrame with the sample standard deviation (N-1 denominator)
// of each group.
func (d *DownsampledDataFrame) Std() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		count := len(vals)
		if count < 2 {
			return 0
		}

		mean := stat.Mean(vals, nil)
		sum := 0.0

		for _, v := range vals {
			diff := v - mean
			sum += diff * diff
		}

		return math.Sqrt(sum / float64(count-1))
	})
}

// Variance returns a DataFrame with the sample variance (N-1 denominator)
// of each group.
func (d *DownsampledDataFrame) Variance() *DataFrame {
	return d.aggregate(func(vals []float64) float64 {
		count := len(vals)
		if count < 2 {
			return 0
		}

		mean := stat.Mean(vals, nil)
		sum := 0.0

		for _, v := range vals {
			diff := v - mean
			sum += diff * diff
		}

		return sum / float64(count-1)
	})
}
