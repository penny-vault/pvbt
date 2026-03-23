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

package study_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
)

var _ = Describe("ParamSweep", func() {
	Describe("SweepRange", func() {
		It("generates float values from min to max with step", func() {
			sweep := study.SweepRange("lookback", 1.0, 3.0, 1.0)
			Expect(sweep.Field()).To(Equal("lookback"))
			Expect(sweep.Values()).To(Equal([]string{"1", "2", "3"}))
		})

		It("generates int values", func() {
			sweep := study.SweepRange("window", 5, 15, 5)
			Expect(sweep.Values()).To(Equal([]string{"5", "10", "15"}))
		})

		It("returns single value when min equals max", func() {
			sweep := study.SweepRange("x", 5.0, 5.0, 1.0)
			Expect(sweep.Values()).To(Equal([]string{"5"}))
		})

		It("returns empty when min exceeds max", func() {
			sweep := study.SweepRange("x", 10.0, 5.0, 1.0)
			Expect(sweep.Values()).To(BeEmpty())
		})
	})

	Describe("SweepDuration", func() {
		It("generates duration values", func() {
			sweep := study.SweepDuration("rebalance", 24*time.Hour, 72*time.Hour, 24*time.Hour)
			Expect(sweep.Values()).To(Equal([]string{"24h0m0s", "48h0m0s", "72h0m0s"}))
		})
	})

	Describe("SweepValues", func() {
		It("stores explicit string values", func() {
			sweep := study.SweepValues("universe", "SPY,TLT", "QQQ,SHY")
			Expect(sweep.Field()).To(Equal("universe"))
			Expect(sweep.Values()).To(Equal([]string{"SPY,TLT", "QQQ,SHY"}))
		})
	})

	Describe("SweepPresets", func() {
		It("stores preset names and marks as preset", func() {
			sweep := study.SweepPresets("Classic", "Modern")
			Expect(sweep.Field()).To(Equal(""))
			Expect(sweep.Values()).To(Equal([]string{"Classic", "Modern"}))
			Expect(sweep.IsPreset()).To(BeTrue())
		})
	})
})

var _ = Describe("ParamSweep Min/Max", func() {
	It("returns min and max for SweepRange", func() {
		sweep := study.SweepRange("lookback", 3.0, 24.0, 1.0)
		Expect(sweep.Min()).To(Equal("3"))
		Expect(sweep.Max()).To(Equal("24"))
	})
	It("returns min and max for SweepDuration", func() {
		sweep := study.SweepDuration("hold", time.Hour, 24*time.Hour, time.Hour)
		Expect(sweep.Min()).To(Equal(time.Hour.String()))
		Expect(sweep.Max()).To(Equal((24 * time.Hour).String()))
	})
	It("returns empty for SweepValues", func() {
		sweep := study.SweepValues("universe", "SPY,TLT", "QQQ,SHY")
		Expect(sweep.Min()).To(BeEmpty())
		Expect(sweep.Max()).To(BeEmpty())
	})
	It("returns empty for SweepPresets", func() {
		sweep := study.SweepPresets("Classic", "Modern")
		Expect(sweep.Min()).To(BeEmpty())
		Expect(sweep.Max()).To(BeEmpty())
	})
})

var _ = Describe("CrossProduct", func() {
	It("cross-products base configs with sweeps", func() {
		base := []study.RunConfig{
			{Name: "Base", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)},
		}

		sweeps := []study.ParamSweep{
			study.SweepRange("lookback", 5.0, 10.0, 5.0),
			study.SweepValues("universe", "SPY", "QQQ"),
		}

		result := study.CrossProduct(base, sweeps)
		Expect(result).To(HaveLen(4)) // 1 base * 2 lookback * 2 universe
	})

	It("handles preset sweeps by setting RunConfig.Preset", func() {
		base := []study.RunConfig{{Name: "Run"}}
		sweeps := []study.ParamSweep{study.SweepPresets("Classic", "Modern")}

		result := study.CrossProduct(base, sweeps)
		Expect(result).To(HaveLen(2))
		Expect(result[0].Preset).To(Equal("Classic"))
		Expect(result[1].Preset).To(Equal("Modern"))
	})

	It("returns base configs unchanged when no sweeps provided", func() {
		base := []study.RunConfig{{Name: "Only"}}
		result := study.CrossProduct(base, nil)
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal("Only"))
	})

	It("sweep preset overrides study preset", func() {
		base := []study.RunConfig{{Name: "Run", Preset: "Original"}}
		sweeps := []study.ParamSweep{study.SweepPresets("Override")}

		result := study.CrossProduct(base, sweeps)
		Expect(result[0].Preset).To(Equal("Override"))
	})

	It("deep copies maps so mutations don't affect other configs", func() {
		base := []study.RunConfig{{Name: "Base", Params: map[string]string{"a": "1"}}}
		sweeps := []study.ParamSweep{study.SweepValues("b", "x", "y")}

		result := study.CrossProduct(base, sweeps)
		result[0].Params["c"] = "mutated"
		Expect(result[1].Params).NotTo(HaveKey("c"))
	})
})
