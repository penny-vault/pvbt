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

func (ap *applyParamsStrategy) Name() string          { return "ApplyParamsTest" }
func (ap *applyParamsStrategy) Setup(_ *engine.Engine) {}
func (ap *applyParamsStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (ap *applyParamsStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "apt"}
}

// noDescriptorStrategy implements Strategy but not Descriptor.
type noDescriptorStrategy struct {
	Window int `pvbt:"window" desc:"Rolling window" default:"12"`
}

func (nd *noDescriptorStrategy) Name() string          { return "NoDescriptor" }
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
