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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
)

var _ = Describe("Grid", func() {
	It("returns all combinations on first call with done=true", func() {
		gs := study.NewGrid(
			study.SweepRange("lookback", 1, 3, 1),
			study.SweepValues("universe", "SPY", "QQQ"),
		)

		configs, done := gs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(HaveLen(6)) // 3 * 2
	})

	It("configs have the correct Params", func() {
		gs := study.NewGrid(
			study.SweepValues("alpha", "a", "b"),
		)

		configs, _ := gs.Next(nil)
		Expect(configs).To(HaveLen(2))
		Expect(configs[0].Params["alpha"]).To(Equal("a"))
		Expect(configs[1].Params["alpha"]).To(Equal("b"))
	})

	It("handles preset sweeps by setting Preset field", func() {
		gs := study.NewGrid(
			study.SweepPresets("Classic", "Modern"),
		)

		configs, done := gs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(HaveLen(2))
		Expect(configs[0].Preset).To(Equal("Classic"))
		Expect(configs[1].Preset).To(Equal("Modern"))
	})

	It("returns nothing on subsequent calls with done=true", func() {
		gs := study.NewGrid(study.SweepValues("x", "1", "2"))

		_, _ = gs.Next(nil)
		configs, done := gs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(BeNil())
	})
})

var _ = Describe("Random", func() {
	It("returns the requested number of samples with done=true", func() {
		rs := study.NewRandom([]study.ParamSweep{
			study.SweepValues("alpha", "a", "b", "c"),
		}, 10, 42)

		configs, done := rs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(HaveLen(10))
	})

	It("samples within range bounds for continuous params", func() {
		rs := study.NewRandom([]study.ParamSweep{
			study.SweepRange("ratio", 0.0, 1.0, 0.1),
		}, 100, 99)

		configs, _ := rs.Next(nil)
		Expect(configs).To(HaveLen(100))

		for _, cfg := range configs {
			raw := cfg.Params["ratio"]
			val, err := strconv.ParseFloat(raw, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically(">=", 0.0))
			Expect(val).To(BeNumerically("<=", 1.0))
		}
	})

	It("is deterministic with the same seed", func() {
		sweeps := []study.ParamSweep{
			study.SweepRange("lookback", 5.0, 30.0, 1.0),
			study.SweepValues("universe", "SPY", "QQQ", "TLT"),
		}

		rs1 := study.NewRandom(sweeps, 20, 7)
		rs2 := study.NewRandom(sweeps, 20, 7)

		configs1, _ := rs1.Next(nil)
		configs2, _ := rs2.Next(nil)

		Expect(configs1).To(HaveLen(20))
		Expect(configs2).To(HaveLen(20))

		for ii := range configs1 {
			Expect(configs1[ii].Params).To(Equal(configs2[ii].Params))
			Expect(configs1[ii].Preset).To(Equal(configs2[ii].Preset))
		}
	})

	It("produces different results with a different seed", func() {
		sweeps := []study.ParamSweep{
			study.SweepValues("x", "a", "b", "c", "d"),
		}

		rs1 := study.NewRandom(sweeps, 50, 1)
		rs2 := study.NewRandom(sweeps, 50, 2)

		configs1, _ := rs1.Next(nil)
		configs2, _ := rs2.Next(nil)

		// Collect values from each run
		vals1 := make([]string, len(configs1))
		vals2 := make([]string, len(configs2))

		for ii := range configs1 {
			vals1[ii] = configs1[ii].Params["x"]
			vals2[ii] = configs2[ii].Params["x"]
		}

		Expect(vals1).NotTo(Equal(vals2))
	})

	It("samples from value lists for discrete params", func() {
		allowed := map[string]bool{"a": true, "b": true, "c": true}
		rs := study.NewRandom([]study.ParamSweep{
			study.SweepValues("key", "a", "b", "c"),
		}, 50, 13)

		configs, _ := rs.Next(nil)
		for _, cfg := range configs {
			Expect(allowed).To(HaveKey(cfg.Params["key"]))
		}
	})

	It("handles preset sweeps by setting Preset field", func() {
		rs := study.NewRandom([]study.ParamSweep{
			study.SweepPresets("Classic", "Modern", "Aggressive"),
		}, 30, 55)

		configs, done := rs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(HaveLen(30))

		allowed := map[string]bool{"Classic": true, "Modern": true, "Aggressive": true}

		for _, cfg := range configs {
			Expect(allowed).To(HaveKey(cfg.Preset))
			Expect(cfg.Params).To(BeNil())
		}
	})

	It("returns nothing on subsequent calls with done=true", func() {
		rs := study.NewRandom([]study.ParamSweep{
			study.SweepValues("x", "1"),
		}, 3, 1)

		_, _ = rs.Next(nil)
		configs, done := rs.Next(nil)
		Expect(done).To(BeTrue())
		Expect(configs).To(BeNil())
	})
})
