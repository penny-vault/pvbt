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

import "math/rand/v2"

// Resampler produces a synthetic return series from historical returns.
// Input is a 2D slice (assets x time steps) of daily returns.
// Output is a 2D slice (assets x targetLen) of resampled returns.
// All methods synchronize cross-asset indices to preserve correlations.
type Resampler interface {
	Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64
}

// BlockBootstrap resamples by picking random contiguous blocks of returns
// across all assets simultaneously, preserving short-term autocorrelation
// and cross-asset correlations within blocks.
type BlockBootstrap struct {
	BlockSize int // number of trading days per block
}

// Resample draws contiguous blocks of length BlockSize (default 20) from the
// historical return series and concatenates them until targetLen is reached.
// All assets use the same block start index at each step, preserving
// cross-asset correlations within each block.
func (bb *BlockBootstrap) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	blockSize := bb.BlockSize
	if blockSize <= 0 {
		blockSize = 20
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, 0, targetLen)
	}

	filled := 0
	for filled < targetLen {
		startIdx := rng.IntN(histLen)
		blockEnd := startIdx + blockSize

		if blockEnd > histLen {
			blockEnd = histLen
		}

		copyLen := blockEnd - startIdx
		if filled+copyLen > targetLen {
			copyLen = targetLen - filled
		}

		for assetIdx := range numAssets {
			result[assetIdx] = append(result[assetIdx], returns[assetIdx][startIdx:startIdx+copyLen]...)
		}

		filled += copyLen
	}

	return result
}

// ReturnBootstrap resamples individual time steps with replacement.
// For each output time step, picks a random historical time step and copies
// returns for all assets at that step. Preserves cross-asset correlations
// at each point but destroys all temporal structure.
type ReturnBootstrap struct{}

// Resample draws targetLen individual time steps at random (with replacement)
// from the historical series. All assets use the same drawn index at each
// step, preserving cross-asset correlations.
func (rb *ReturnBootstrap) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, targetLen)
	}

	for timeIdx := range targetLen {
		srcIdx := rng.IntN(histLen)
		for assetIdx := range numAssets {
			result[assetIdx][timeIdx] = returns[assetIdx][srcIdx]
		}
	}

	return result
}

// Permutation randomly shuffles the time indices of the historical return
// series without replacement. All assets are permuted with the same index
// mapping. The marginal distribution is exactly preserved.
type Permutation struct{}

// Resample returns a permuted view of the historical returns. When targetLen
// exceeds the history length, output is capped at the history length. When
// targetLen is shorter, only the first targetLen permuted indices are used.
func (pp *Permutation) Resample(returns [][]float64, targetLen int, rng *rand.Rand) [][]float64 {
	if len(returns) == 0 || len(returns[0]) == 0 {
		return returns
	}

	numAssets := len(returns)
	histLen := len(returns[0])

	indices := rng.Perm(histLen)

	actualLen := targetLen
	if actualLen > histLen {
		actualLen = histLen
	}

	result := make([][]float64, numAssets)
	for assetIdx := range result {
		result[assetIdx] = make([]float64, actualLen)
		for timeIdx := range actualLen {
			result[assetIdx][timeIdx] = returns[assetIdx][indices[timeIdx]]
		}
	}

	return result
}
