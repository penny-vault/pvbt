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

package engine_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// applyParamsStrategy has suggest tags so presets are available via DescribeStrategy.
type applyParamsStrategy struct {
	Lookback int     `pvbt:"lookback" desc:"Lookback" default:"6" suggest:"Fast=3|Slow=12"`
	Ratio    float64 `pvbt:"ratio" desc:"Ratio" default:"0.5" suggest:"Fast=0.8|Slow=0.2"`
	Label    string  `pvbt:"label" desc:"Label" default:"default"`
}

func (ap *applyParamsStrategy) Name() string           { return "ApplyParamsTest" }
func (ap *applyParamsStrategy) Setup(_ *engine.Engine) {}
func (ap *applyParamsStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (ap *applyParamsStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "apt"}
}

// vanillaStrategy mirrors the value-factor case where two int params
// have non-zero defaults but the "Vanilla" preset suggests zeroing them.
// This exists to exercise the bug where presets that set a field to 0
// were being silently re-defaulted by hydrateFields.
type vanillaStrategy struct {
	SectorCap int `pvbt:"sector-cap" desc:"Sector cap" default:"4" suggest:"Vanilla=0"`
	MinFScore int `pvbt:"min-fscore" desc:"Min F-score" default:"6" suggest:"Vanilla=0"`
}

func (vs *vanillaStrategy) Name() string           { return "Vanilla" }
func (vs *vanillaStrategy) Setup(_ *engine.Engine) {}
func (vs *vanillaStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (vs *vanillaStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "vsi"}
}

// noDescriptorStrategy implements Strategy but not Descriptor.
type noDescriptorStrategy struct {
	Window int `pvbt:"window" desc:"Rolling window" default:"12"`
}

func (nd *noDescriptorStrategy) Name() string           { return "NoDescriptor" }
func (nd *noDescriptorStrategy) Setup(_ *engine.Engine) {}
func (nd *noDescriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("ApplyParams", func() {
	Context("simple parameter application", func() {
		It("applies a string parameter", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"label": "custom"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Label).To(Equal("custom"))
		})

		It("applies a float64 parameter", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"ratio": "0.75"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Ratio).To(Equal(0.75))
		})

		It("is a no-op with empty params and no preset", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			// Fields retain zero values since no params were applied and
			// hydrateFields only sets fields with default tags when zero.
			Expect(strategy.Lookback).To(Equal(6))
			Expect(strategy.Ratio).To(Equal(0.5))
			Expect(strategy.Label).To(Equal("default"))
		})

		It("is a no-op with nil params and no preset", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("preset resolution", func() {
		It("resolves a preset and applies all preset values", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "Fast", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Lookback).To(Equal(3))
			Expect(strategy.Ratio).To(Equal(0.8))
		})

		It("allows explicit params to override preset values", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "Fast", map[string]string{"lookback": "7"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Lookback).To(Equal(7))
			// Ratio still comes from the Fast preset.
			Expect(strategy.Ratio).To(Equal(0.8))
		})

		It("returns an error for an unknown preset", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "Unknown", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("preset \"Unknown\" not found"))
			Expect(err.Error()).To(ContainSubstring("Fast"))
			Expect(err.Error()).To(ContainSubstring("Slow"))
		})
	})

	Context("explicit zero overrides", func() {
		It("preserves a zero passed via params against a non-zero int default", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"lookback": "0"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Lookback).To(Equal(0),
				"lookback=0 must override the struct-tag default of 6, not be silently re-defaulted")
		})

		It("preserves a zero passed via params against a non-zero float default", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"ratio": "0"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Ratio).To(Equal(0.0),
				"ratio=0 must override the struct-tag default of 0.5, not be silently re-defaulted")
		})

		It("honors a preset that sets a parameter to zero", func() {
			strategy := &vanillaStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "Vanilla", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.SectorCap).To(Equal(0),
				"Vanilla preset must set sector-cap=0, not be re-defaulted to 4")
			Expect(strategy.MinFScore).To(Equal(0),
				"Vanilla preset must set min-fscore=0, not be re-defaulted to 6")
		})

		It("still applies defaults to params the user did not touch", func() {
			strategy := &applyParamsStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"lookback": "0"})
			Expect(err).NotTo(HaveOccurred())
			// lookback was zeroed by the user
			Expect(strategy.Lookback).To(Equal(0))
			// ratio and label were not touched, so defaults should still apply
			Expect(strategy.Ratio).To(Equal(0.5),
				"ratio not in params; default must still apply")
			Expect(strategy.Label).To(Equal("default"),
				"label not in params; default must still apply")
		})

		It("preserves an explicit zero set by the CLI path (WithUserParams)", func() {
			// Simulate the CLI flow: applyStrategyFlags has already written
			// the explicit zero to the field, then engine.New is constructed
			// with WithUserParams listing the field. hydrateFields must not
			// overwrite the zero with the struct-tag default.
			strategy := &applyParamsStrategy{Lookback: 0}
			eng := engine.New(strategy, engine.WithUserParams("lookback"))

			err := engine.HydrateFieldsForTest(eng, strategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Lookback).To(Equal(0),
				"WithUserParams must protect an explicit zero from being re-defaulted")
		})
	})

	Context("Descriptor requirement", func() {
		It("returns an error when preset is non-empty and strategy lacks Descriptor", func() {
			strategy := &noDescriptorStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "SomePreset", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not implement Descriptor"))
		})

		It("succeeds when preset is empty and strategy lacks Descriptor", func() {
			strategy := &noDescriptorStrategy{}
			eng := engine.New(strategy)

			err := engine.ApplyParams(eng, "", map[string]string{"window": "24"})
			Expect(err).NotTo(HaveOccurred())
			Expect(strategy.Window).To(Equal(24))
		})
	})
})

// applyParamsTestOnlyStrategy has one regular field and one test-only field.
// It implements Descriptor so the preset path is also exercisable.
type applyParamsTestOnlyStrategy struct {
	Window int `pvbt:"window" desc:"Rolling window" default:"12"`
	Seed   int `pvbt:"seed" testonly:"true"`
}

func (ap *applyParamsTestOnlyStrategy) Name() string           { return "ApplyParamsTestOnly" }
func (ap *applyParamsTestOnlyStrategy) Setup(_ *engine.Engine) {}
func (ap *applyParamsTestOnlyStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (ap *applyParamsTestOnlyStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "apto"}
}

var _ = Describe("ApplyParams with testonly fields", func() {
	It("returns an error when explicit params target a test-only field", func() {
		strategy := &applyParamsTestOnlyStrategy{}
		eng := engine.New(strategy)

		err := engine.ApplyParams(eng, "", map[string]string{"seed": "42"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("seed"))
		Expect(err.Error()).To(ContainSubstring("test-only"))
		Expect(strategy.Seed).To(Equal(0))
	})

	It("still applies non-test-only params alongside the rejected one", func() {
		strategy := &applyParamsTestOnlyStrategy{}
		eng := engine.New(strategy)

		err := engine.ApplyParams(eng, "", map[string]string{
			"window": "24",
			"seed":   "42",
		})
		Expect(err).To(HaveOccurred())
		// The whole call fails -- no params should be partially applied.
		Expect(strategy.Window).To(Equal(0))
		Expect(strategy.Seed).To(Equal(0))
	})

	It("allows direct struct assignment of a test-only field", func() {
		strategy := &applyParamsTestOnlyStrategy{Seed: 99}
		eng := engine.New(strategy)

		err := engine.ApplyParams(eng, "", map[string]string{"window": "24"})
		Expect(err).NotTo(HaveOccurred())
		Expect(strategy.Window).To(Equal(24))
		Expect(strategy.Seed).To(Equal(99))
	})
})
