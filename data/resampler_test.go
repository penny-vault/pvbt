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

package data_test

import (
	"math/rand/v2"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/data"
)

// makeRng returns a deterministic RNG seeded with the given value.
func makeRng(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
}

// twoAssetReturns returns a simple 2-asset, histLen-step return series.
// Asset 0 values are 1.0, 2.0, ..., histLen.
// Asset 1 values are offset from asset 0 by a fixed constant (10.0) so that
// (asset1[t] - asset0[t]) == 10.0 for every t.
func twoAssetReturns(histLen int) [][]float64 {
	asset0 := make([]float64, histLen)
	asset1 := make([]float64, histLen)
	for idx := range histLen {
		asset0[idx] = float64(idx + 1)
		asset1[idx] = float64(idx+1) + 10.0
	}
	return [][]float64{asset0, asset1}
}

var _ = Describe("Resampler", func() {
	const histLen = 100
	const targetLen = 150

	Describe("BlockBootstrap", func() {
		It("satisfies the Resampler interface", func() {
			var _ data.Resampler = &data.BlockBootstrap{BlockSize: 20}
		})

		It("produces output of the requested length", func() {
			returns := twoAssetReturns(histLen)
			bb := &data.BlockBootstrap{BlockSize: 20}
			result := bb.Resample(returns, targetLen, makeRng(1))
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(HaveLen(targetLen))
			Expect(result[1]).To(HaveLen(targetLen))
		})

		It("preserves cross-asset correlation (same time indices across assets)", func() {
			// Asset 1 is always asset 0 + 10; if both use the same source index,
			// the difference is always exactly 10.0 at every output time step.
			returns := twoAssetReturns(histLen)
			bb := &data.BlockBootstrap{BlockSize: 20}
			result := bb.Resample(returns, targetLen, makeRng(2))
			for timeIdx := range targetLen {
				diff := result[1][timeIdx] - result[0][timeIdx]
				Expect(diff).To(BeNumerically("~", 10.0, 1e-9),
					"cross-asset difference should be 10.0 at every time step")
			}
		})

		It("is reproducible with the same seed", func() {
			returns := twoAssetReturns(histLen)
			bb := &data.BlockBootstrap{BlockSize: 20}
			first := bb.Resample(returns, targetLen, makeRng(42))
			second := bb.Resample(returns, targetLen, makeRng(42))
			Expect(first).To(Equal(second))
		})

		It("returns the input unchanged on empty input", func() {
			bb := &data.BlockBootstrap{BlockSize: 20}
			empty := [][]float64{}
			Expect(bb.Resample(empty, targetLen, makeRng(0))).To(Equal(empty))

			emptyInner := [][]float64{{}}
			Expect(bb.Resample(emptyInner, targetLen, makeRng(0))).To(Equal(emptyInner))
		})

		It("handles targetLen shorter than blockSize", func() {
			returns := twoAssetReturns(histLen)
			bb := &data.BlockBootstrap{BlockSize: 50}
			shortTarget := 10
			result := bb.Resample(returns, shortTarget, makeRng(3))
			Expect(result[0]).To(HaveLen(shortTarget))
			Expect(result[1]).To(HaveLen(shortTarget))
		})

		It("uses default block size of 20 when BlockSize is zero", func() {
			returns := twoAssetReturns(histLen)
			bb := &data.BlockBootstrap{BlockSize: 0}
			result := bb.Resample(returns, targetLen, makeRng(5))
			Expect(result[0]).To(HaveLen(targetLen))
		})
	})

	Describe("ReturnBootstrap", func() {
		It("satisfies the Resampler interface", func() {
			var _ data.Resampler = &data.ReturnBootstrap{}
		})

		It("produces output of the requested length", func() {
			returns := twoAssetReturns(histLen)
			rb := &data.ReturnBootstrap{}
			result := rb.Resample(returns, targetLen, makeRng(1))
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(HaveLen(targetLen))
			Expect(result[1]).To(HaveLen(targetLen))
		})

		It("preserves cross-asset correlation at each time step", func() {
			// Asset 1 is always asset 0 + 10; same src index means same offset.
			returns := twoAssetReturns(histLen)
			rb := &data.ReturnBootstrap{}
			result := rb.Resample(returns, targetLen, makeRng(2))
			for timeIdx := range targetLen {
				diff := result[1][timeIdx] - result[0][timeIdx]
				Expect(diff).To(BeNumerically("~", 10.0, 1e-9),
					"cross-asset difference should be 10.0 at every time step")
			}
		})

		It("only contains values present in the original series", func() {
			returns := twoAssetReturns(histLen)
			rb := &data.ReturnBootstrap{}
			result := rb.Resample(returns, targetLen, makeRng(7))

			// Build a set of valid asset-0 values (1.0 .. histLen).
			validValues := make(map[float64]bool, histLen)
			for _, val := range returns[0] {
				validValues[val] = true
			}
			for _, val := range result[0] {
				Expect(validValues[val]).To(BeTrue(),
					"output value %v not found in original series", val)
			}
		})

		It("returns the input unchanged on empty input", func() {
			rb := &data.ReturnBootstrap{}
			empty := [][]float64{}
			Expect(rb.Resample(empty, targetLen, makeRng(0))).To(Equal(empty))

			emptyInner := [][]float64{{}}
			Expect(rb.Resample(emptyInner, targetLen, makeRng(0))).To(Equal(emptyInner))
		})
	})

	Describe("Permutation", func() {
		It("satisfies the Resampler interface", func() {
			var _ data.Resampler = &data.Permutation{}
		})

		It("preserves the exact distribution of returns (same values, possibly different order)", func() {
			returns := twoAssetReturns(histLen)
			pp := &data.Permutation{}
			result := pp.Resample(returns, histLen, makeRng(1))

			// Sort both slices and compare.
			orig := make([]float64, histLen)
			copy(orig, returns[0])
			got := make([]float64, histLen)
			copy(got, result[0])
			sort.Float64s(orig)
			sort.Float64s(got)
			Expect(got).To(Equal(orig))
		})

		It("preserves cross-asset correlation", func() {
			returns := twoAssetReturns(histLen)
			pp := &data.Permutation{}
			result := pp.Resample(returns, histLen, makeRng(3))
			for timeIdx := range histLen {
				diff := result[1][timeIdx] - result[0][timeIdx]
				Expect(diff).To(BeNumerically("~", 10.0, 1e-9),
					"cross-asset difference should be 10.0 at every time step")
			}
		})

		It("truncates to history length when targetLen exceeds history length", func() {
			returns := twoAssetReturns(histLen)
			pp := &data.Permutation{}
			// Ask for more than histLen; expect exactly histLen back.
			result := pp.Resample(returns, histLen+50, makeRng(4))
			Expect(result[0]).To(HaveLen(histLen))
			Expect(result[1]).To(HaveLen(histLen))
		})

		It("returns only targetLen rows when targetLen is shorter than history", func() {
			returns := twoAssetReturns(histLen)
			pp := &data.Permutation{}
			shortTarget := 40
			result := pp.Resample(returns, shortTarget, makeRng(5))
			Expect(result[0]).To(HaveLen(shortTarget))
			Expect(result[1]).To(HaveLen(shortTarget))
		})

		It("returns the input unchanged on empty input", func() {
			pp := &data.Permutation{}
			empty := [][]float64{}
			Expect(pp.Resample(empty, histLen, makeRng(0))).To(Equal(empty))

			emptyInner := [][]float64{{}}
			Expect(pp.Resample(emptyInner, histLen, makeRng(0))).To(Equal(emptyInner))
		})
	})
})
