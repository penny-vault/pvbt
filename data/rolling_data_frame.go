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
	"sort"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat"
)

// RollingDataFrame applies rolling-window operations to each column of
// the source DataFrame. Created by DataFrame.Rolling(n).
type RollingDataFrame struct {
	df     *DataFrame
	window int
}

// Mean returns a DataFrame with the rolling mean over the window.
func (r *RollingDataFrame) Mean() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			out[idx] = stat.Mean(col[idx-windowSize+1:idx+1], nil)
		}

		return out
	})
}

// Sum returns a DataFrame with the rolling sum over the window.
func (r *RollingDataFrame) Sum() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			out[idx] = floats.Sum(col[idx-windowSize+1 : idx+1])
		}

		return out
	})
}

// Max returns a DataFrame with the rolling maximum over the window.
func (r *RollingDataFrame) Max() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			out[idx] = floats.Max(col[idx-windowSize+1 : idx+1])
		}

		return out
	})
}

// Min returns a DataFrame with the rolling minimum over the window.
func (r *RollingDataFrame) Min() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			out[idx] = floats.Min(col[idx-windowSize+1 : idx+1])
		}

		return out
	})
}

// Std returns a DataFrame with the rolling sample standard deviation
// (N-1 denominator) over the window.
func (r *RollingDataFrame) Std() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			window := col[idx-windowSize+1 : idx+1]
			mean := stat.Mean(window, nil)

			variance := 0.0

			for _, v := range window {
				diff := v - mean
				variance += diff * diff
			}

			if windowSize <= 1 {
				out[idx] = 0.0
			} else {
				out[idx] = math.Sqrt(variance / float64(windowSize-1))
			}
		}

		return out
	})
}

// Variance returns a DataFrame with the rolling sample variance (N-1
// denominator) over the window.
func (r *RollingDataFrame) Variance() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			window := col[idx-windowSize+1 : idx+1]
			mean := stat.Mean(window, nil)

			variance := 0.0

			for _, v := range window {
				diff := v - mean
				variance += diff * diff
			}

			if windowSize <= 1 {
				out[idx] = 0
			} else {
				out[idx] = variance / float64(windowSize-1)
			}
		}

		return out
	})
}

// Percentile returns a DataFrame with the rolling p-th percentile over
// the window. p should be in the range [0, 1].
func (r *RollingDataFrame) Percentile(percentile float64) *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			sorted := make([]float64, windowSize)
			copy(sorted, col[idx-windowSize+1:idx+1])
			sort.Float64s(sorted)

			out[idx] = stat.Quantile(percentile, stat.LinInterp, sorted, nil)
		}

		return out
	})
}

// EMA returns a DataFrame with the exponential moving average over the
// window. The smoothing factor is alpha = 2 / (n + 1) where n is the
// window size. The first n-1 rows are NaN. The EMA is seeded with the
// simple moving average of the first n values.
func (r *RollingDataFrame) EMA() *DataFrame {
	if r.df.err != nil {
		return WithErr(r.df.err)
	}

	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		windowSize := r.window
		alpha := 2.0 / float64(windowSize+1)

		for idx := range col {
			if idx < windowSize-1 {
				out[idx] = math.NaN()

				continue
			}

			if idx == windowSize-1 {
				out[idx] = stat.Mean(col[:windowSize], nil)

				continue
			}

			out[idx] = alpha*col[idx] + (1-alpha)*out[idx-1]
		}

		return out
	})
}
