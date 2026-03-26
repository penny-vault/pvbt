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

package signal

import (
	"fmt"
	"math"
)

// zScore computes the z-score of the last element in values relative to the
// mean and standard deviation of the full slice. Returns an error if the
// standard deviation is zero (constant series) or the slice has fewer than
// 2 elements.
func zScore(values []float64) (float64, error) {
	nn := len(values)
	if nn < 2 {
		return 0, fmt.Errorf("zScore: need at least 2 values, got %d", nn)
	}

	sum := 0.0
	for _, vv := range values {
		sum += vv
	}

	mean := sum / float64(nn)

	sumSq := 0.0

	for _, vv := range values {
		diff := vv - mean
		sumSq += diff * diff
	}

	stddev := math.Sqrt(sumSq / float64(nn))
	if stddev == 0 {
		return 0, fmt.Errorf("zScore: standard deviation is zero (constant series)")
	}

	return (values[nn-1] - mean) / stddev, nil
}

// linRegress performs simple linear regression of yy on xx, returning the
// slope and intercept. Both slices must have the same length (>= 2).
func linRegress(xx, yy []float64) (slope, intercept float64, err error) {
	nn := len(xx)
	if nn < 2 {
		return 0, 0, fmt.Errorf("linRegress: need at least 2 points, got %d", nn)
	}

	if len(yy) != nn {
		return 0, 0, fmt.Errorf("linRegress: x and y lengths differ (%d vs %d)", nn, len(yy))
	}

	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for ii := range nn {
		sumX += xx[ii]
		sumY += yy[ii]
		sumXY += xx[ii] * yy[ii]
		sumX2 += xx[ii] * xx[ii]
	}

	nf := float64(nn)
	denom := nf*sumX2 - sumX*sumX

	if denom == 0 {
		return 0, 0, fmt.Errorf("linRegress: all x values are identical")
	}

	slope = (nf*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / nf

	return slope, intercept, nil
}
