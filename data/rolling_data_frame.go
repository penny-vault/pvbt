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
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			out[i] = stat.Mean(col[i-n+1:i+1], nil)
		}

		return out
	})
}

// Sum returns a DataFrame with the rolling sum over the window.
func (r *RollingDataFrame) Sum() *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			out[i] = floats.Sum(col[i-n+1 : i+1])
		}

		return out
	})
}

// Max returns a DataFrame with the rolling maximum over the window.
func (r *RollingDataFrame) Max() *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			out[i] = floats.Max(col[i-n+1 : i+1])
		}

		return out
	})
}

// Min returns a DataFrame with the rolling minimum over the window.
func (r *RollingDataFrame) Min() *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			out[i] = floats.Min(col[i-n+1 : i+1])
		}

		return out
	})
}

// Std returns a DataFrame with the rolling sample standard deviation
// (N-1 denominator) over the window.
func (r *RollingDataFrame) Std() *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			window := col[i-n+1 : i+1]
			mean := stat.Mean(window, nil)

			variance := 0.0

			for _, v := range window {
				d := v - mean
				variance += d * d
			}

			if n <= 1 {
				out[i] = 0.0
			} else {
				out[i] = math.Sqrt(variance / float64(n-1))
			}
		}

		return out
	})
}

// Variance returns a DataFrame with the rolling sample variance (N-1
// denominator) over the window.
func (r *RollingDataFrame) Variance() *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			window := col[i-n+1 : i+1]
			mean := stat.Mean(window, nil)

			variance := 0.0

			for _, v := range window {
				d := v - mean
				variance += d * d
			}

			out[i] = variance / float64(n-1)
		}

		return out
	})
}

// Percentile returns a DataFrame with the rolling p-th percentile over
// the window. p should be in the range [0, 1].
func (r *RollingDataFrame) Percentile(p float64) *DataFrame {
	return r.df.Apply(func(col []float64) []float64 {
		out := make([]float64, len(col))
		n := r.window

		for i := range col {
			if i < n-1 {
				out[i] = math.NaN()

				continue
			}

			sorted := make([]float64, n)
			copy(sorted, col[i-n+1:i+1])
			sort.Float64s(sorted)

			out[i] = stat.Quantile(p, stat.LinInterp, sorted, nil)
		}

		return out
	})
}
