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

// UpsampledDataFrame fills gaps when converting to a higher frequency.
// Created by DataFrame.Upsample(freq).
type UpsampledDataFrame struct {
	df   *DataFrame
	freq Frequency
}

// generateTimes creates a time axis at the target frequency spanning
// from the first to the last timestamp of the source DataFrame.
func (u *UpsampledDataFrame) generateTimes() []time.Time {
	if len(u.df.times) < 2 {
		return u.df.times
	}

	start := u.df.times[0]
	end := u.df.times[len(u.df.times)-1]
	var times []time.Time

	for t := start; !t.After(end); {
		times = append(times, t)
		switch u.freq {
		case Daily:
			t = t.AddDate(0, 0, 1)
		case Weekly:
			t = t.AddDate(0, 0, 7)
		case Monthly:
			t = t.AddDate(0, 1, 0)
		case Quarterly:
			t = t.AddDate(0, 3, 0)
		case Yearly:
			t = t.AddDate(1, 0, 0)
		default:
			t = t.AddDate(0, 0, 1) // default to daily
		}
	}

	return times
}

// ForwardFill carries the last known value forward to fill gaps in the
// upsampled time axis.
func (u *UpsampledDataFrame) ForwardFill() *DataFrame {
	if len(u.df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newTimes := u.generateTimes()
	assetLen := len(u.df.assets)
	metricLen := len(u.df.metrics)
	newData := make([]float64, assetLen*metricLen*len(newTimes))

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcCol := u.df.colSlice(aIdx, mIdx)
			dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
			srcIdx := 0

			for i, t := range newTimes {
				// Advance source index to the latest timestamp <= t
				for srcIdx < len(u.df.times)-1 && !u.df.times[srcIdx+1].After(t) {
					srcIdx++
				}
				newData[dstOff+i] = srcCol[srcIdx]
			}
		}
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, u.df.assets)
	metrics := make([]Metric, metricLen)
	copy(metrics, u.df.metrics)

	return mustNewDataFrame(newTimes, assets, metrics, newData)
}

// BackFill uses the next known value to fill gaps in the upsampled time axis.
func (u *UpsampledDataFrame) BackFill() *DataFrame {
	if len(u.df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newTimes := u.generateTimes()
	assetLen := len(u.df.assets)
	metricLen := len(u.df.metrics)
	newData := make([]float64, assetLen*metricLen*len(newTimes))

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcCol := u.df.colSlice(aIdx, mIdx)
			dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
			srcIdx := 0

			for i, t := range newTimes {
				// Advance source index to the earliest timestamp >= t
				for srcIdx < len(u.df.times)-1 && u.df.times[srcIdx].Before(t) {
					srcIdx++
				}
				newData[dstOff+i] = srcCol[srcIdx]
			}
		}
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, u.df.assets)
	metrics := make([]Metric, metricLen)
	copy(metrics, u.df.metrics)

	return mustNewDataFrame(newTimes, assets, metrics, newData)
}

// Interpolate linearly interpolates between known values to fill gaps
// in the upsampled time axis.
func (u *UpsampledDataFrame) Interpolate() *DataFrame {
	if len(u.df.times) == 0 {
		return mustNewDataFrame(nil, nil, nil, nil)
	}

	newTimes := u.generateTimes()
	assetLen := len(u.df.assets)
	metricLen := len(u.df.metrics)
	newData := make([]float64, assetLen*metricLen*len(newTimes))

	for aIdx := 0; aIdx < assetLen; aIdx++ {
		for mIdx := 0; mIdx < metricLen; mIdx++ {
			srcCol := u.df.colSlice(aIdx, mIdx)
			dstOff := (aIdx*metricLen + mIdx) * len(newTimes)
			srcIdx := 0

			for i, t := range newTimes {
				// Find surrounding source timestamps.
				for srcIdx < len(u.df.times)-1 && u.df.times[srcIdx+1].Before(t) {
					srcIdx++
				}

				if srcIdx >= len(u.df.times)-1 || t.Equal(u.df.times[srcIdx]) {
					newData[dstOff+i] = srcCol[srcIdx]
				} else {
					// Linear interpolation.
					t0 := u.df.times[srcIdx]
					t1 := u.df.times[srcIdx+1]
					v0 := srcCol[srcIdx]
					v1 := srcCol[srcIdx+1]
					frac := float64(t.Sub(t0)) / float64(t1.Sub(t0))
					newData[dstOff+i] = v0 + frac*(v1-v0)
				}
			}
		}
	}

	assets := make([]asset.Asset, assetLen)
	copy(assets, u.df.assets)
	metrics := make([]Metric, metricLen)
	copy(metrics, u.df.metrics)

	return mustNewDataFrame(newTimes, assets, metrics, newData)
}
