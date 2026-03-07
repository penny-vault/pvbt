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

// RollingDataFrame applies rolling-window operations to each column of
// the source DataFrame. Created by DataFrame.Rolling(n).
type RollingDataFrame struct {
	df     *DataFrame //nolint:unused // referenced once methods are implemented
	window int        //nolint:unused // referenced once methods are implemented
}

// Mean returns a DataFrame with the rolling mean over the window.
func (r *RollingDataFrame) Mean() *DataFrame { return nil }

// Sum returns a DataFrame with the rolling sum over the window.
func (r *RollingDataFrame) Sum() *DataFrame { return nil }

// Max returns a DataFrame with the rolling maximum over the window.
func (r *RollingDataFrame) Max() *DataFrame { return nil }

// Min returns a DataFrame with the rolling minimum over the window.
func (r *RollingDataFrame) Min() *DataFrame { return nil }

// Std returns a DataFrame with the rolling standard deviation over the window.
func (r *RollingDataFrame) Std() *DataFrame { return nil }

// Percentile returns a DataFrame with the rolling p-th percentile over
// the window. p should be in the range [0, 1].
func (r *RollingDataFrame) Percentile(p float64) *DataFrame { return nil }
