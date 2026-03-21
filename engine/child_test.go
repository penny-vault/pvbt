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

// --- Stubs for child discovery tests ---

// simpleChild is a minimal strategy with configurable parameters.
type simpleChild struct {
	Lookback int     `pvbt:"lookback" desc:"Lookback" default:"6" suggest:"Fast=3|Slow=12"`
	Ratio    float64 `pvbt:"ratio" desc:"Ratio" default:"0.5" suggest:"Fast=0.8|Slow=0.2"`
	Label    string  `pvbt:"label" desc:"Label" default:"default"`
	Enabled  bool    `pvbt:"enabled" desc:"Enabled" default:"true"`
}

func (sc *simpleChild) Name() string           { return "SimpleChild" }
func (sc *simpleChild) Setup(_ *engine.Engine)  {}
func (sc *simpleChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (sc *simpleChild) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{ShortCode: "sc"}
}

// parentSingleChild has one weighted child.
type parentSingleChild struct {
	Child *simpleChild `pvbt:"child" weight:"0.60"`
}

func (p *parentSingleChild) Name() string           { return "ParentSingle" }
func (p *parentSingleChild) Setup(_ *engine.Engine)  {}
func (p *parentSingleChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentNoWeight has a Strategy field without a weight tag -- should be skipped.
type parentNoWeight struct {
	Child *simpleChild `pvbt:"child"`
}

func (p *parentNoWeight) Name() string           { return "ParentNoWeight" }
func (p *parentNoWeight) Setup(_ *engine.Engine)  {}
func (p *parentNoWeight) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentNilChild has a nil pointer child that needs allocation.
type parentNilChild struct {
	Child *simpleChild `pvbt:"child" weight:"0.40"`
}

func (p *parentNilChild) Name() string           { return "ParentNil" }
func (p *parentNilChild) Setup(_ *engine.Engine)  {}
func (p *parentNilChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentOverweight has children whose weights exceed 1.0.
type parentOverweight struct {
	ChildA *simpleChild `pvbt:"child-a" weight:"0.70"`
	ChildB *simpleChild `pvbt:"child-b" weight:"0.40"`
}

func (p *parentOverweight) Name() string           { return "ParentOverweight" }
func (p *parentOverweight) Setup(_ *engine.Engine)  {}
func (p *parentOverweight) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentValidWeight has children whose weights sum to 0.90.
type parentValidWeight struct {
	ChildA *simpleChild `pvbt:"child-a" weight:"0.50"`
	ChildB *simpleChild `pvbt:"child-b" weight:"0.40"`
}

func (p *parentValidWeight) Name() string           { return "ParentValid" }
func (p *parentValidWeight) Setup(_ *engine.Engine)  {}
func (p *parentValidWeight) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentWithPreset applies a preset to a child.
type parentWithPreset struct {
	Child *simpleChild `pvbt:"child" weight:"0.50" preset:"Fast"`
}

func (p *parentWithPreset) Name() string           { return "ParentPreset" }
func (p *parentWithPreset) Setup(_ *engine.Engine)  {}
func (p *parentWithPreset) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentWithParams applies params overrides.
type parentWithParams struct {
	Child *simpleChild `pvbt:"child" weight:"0.50" params:"lookback=20 ratio=0.99 label=custom"`
}

func (p *parentWithParams) Name() string           { return "ParentParams" }
func (p *parentWithParams) Setup(_ *engine.Engine)  {}
func (p *parentWithParams) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentPresetThenParams applies a preset and then overrides with params.
type parentPresetThenParams struct {
	Child *simpleChild `pvbt:"child" weight:"0.50" preset:"Slow" params:"lookback=99"`
}

func (p *parentPresetThenParams) Name() string           { return "ParentPresetParams" }
func (p *parentPresetThenParams) Setup(_ *engine.Engine)  {}
func (p *parentPresetThenParams) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentBadPreset references a non-existent preset.
type parentBadPreset struct {
	Child *simpleChild `pvbt:"child" weight:"0.30" preset:"NonExistent"`
}

func (p *parentBadPreset) Name() string           { return "ParentBadPreset" }
func (p *parentBadPreset) Setup(_ *engine.Engine)  {}
func (p *parentBadPreset) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// cyclicParent holds a Strategy interface field that points back to itself.
type cyclicParent struct {
	Loop engine.Strategy `weight:"0.50"`
}

func (cp *cyclicParent) Name() string           { return "CyclicParent" }
func (cp *cyclicParent) Setup(_ *engine.Engine)  {}
func (cp *cyclicParent) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// innerChild is used as a grandchild in recursive discovery.
type innerChild struct {
	Speed int `pvbt:"speed" default:"5"`
}

func (ic *innerChild) Name() string           { return "InnerChild" }
func (ic *innerChild) Setup(_ *engine.Engine)  {}
func (ic *innerChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// middleChild contains its own weighted child.
type middleChild struct {
	Inner *innerChild `pvbt:"inner" weight:"0.30"`
}

func (mc *middleChild) Name() string           { return "MiddleChild" }
func (mc *middleChild) Setup(_ *engine.Engine)  {}
func (mc *middleChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentRecursive has a child that itself has a weighted child.
type parentRecursive struct {
	Mid *middleChild `pvbt:"mid" weight:"0.60"`
}

func (p *parentRecursive) Name() string           { return "ParentRecursive" }
func (p *parentRecursive) Setup(_ *engine.Engine)  {}
func (p *parentRecursive) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// parentNoTag has a child field without a pvbt tag -- name should be lowercased field name.
type parentNoTag struct {
	MyChild *simpleChild `weight:"0.25"`
}

func (p *parentNoTag) Name() string           { return "ParentNoTag" }
func (p *parentNoTag) Setup(_ *engine.Engine)  {}
func (p *parentNoTag) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// --- Test suite ---

var _ = Describe("discoverChildren", func() {
	var eng *engine.Engine

	Context("basic discovery", func() {
		It("discovers a field with a weight tag and parses weight correctly", func() {
			parent := &parentSingleChild{Child: &simpleChild{}}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			children := engine.EngineChildrenForTest(eng)
			Expect(children).To(HaveLen(1))
			Expect(engine.ChildEntryName(children[0])).To(Equal("child"))
			Expect(engine.ChildEntryWeight(children[0])).To(BeNumerically("~", 0.60, 0.001))
		})

		It("ignores a Strategy field without a weight tag", func() {
			parent := &parentNoWeight{Child: &simpleChild{}}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			children := engine.EngineChildrenForTest(eng)
			Expect(children).To(BeEmpty())
		})

		It("allocates a nil pointer child", func() {
			parent := &parentNilChild{} // Child is nil
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			Expect(parent.Child).NotTo(BeNil())
			children := engine.EngineChildrenForTest(eng)
			Expect(children).To(HaveLen(1))
		})

		It("uses lowercased field name when pvbt tag is absent", func() {
			parent := &parentNoTag{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			children := engine.EngineChildrenForTest(eng)
			Expect(children).To(HaveLen(1))
			Expect(engine.ChildEntryName(children[0])).To(Equal("mychild"))
		})
	})

	Context("weight validation", func() {
		It("returns error when weights sum exceeds 1.0", func() {
			parent := &parentOverweight{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds 1.0"))
		})

		It("succeeds when weights sum to 0.90", func() {
			parent := &parentValidWeight{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			children := engine.EngineChildrenForTest(eng)
			Expect(children).To(HaveLen(2))
		})
	})

	Context("preset application", func() {
		It("applies suggestion values from a preset tag", func() {
			parent := &parentWithPreset{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			// The "Fast" preset sets lookback=3, ratio=0.8
			Expect(parent.Child.Lookback).To(Equal(3))
			Expect(parent.Child.Ratio).To(BeNumerically("~", 0.8, 0.001))
		})

		It("returns error for a missing preset name", func() {
			parent := &parentBadPreset{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("preset"))
			Expect(err.Error()).To(ContainSubstring("NonExistent"))
		})
	})

	Context("params application", func() {
		It("applies space-separated key=value overrides from params tag", func() {
			parent := &parentWithParams{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			Expect(parent.Child.Lookback).To(Equal(20))
			Expect(parent.Child.Ratio).To(BeNumerically("~", 0.99, 0.001))
			Expect(parent.Child.Label).To(Equal("custom"))
		})

		It("params override preset values", func() {
			parent := &parentPresetThenParams{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			// Preset "Slow" sets lookback=12, ratio=0.2
			// Params override lookback=99
			Expect(parent.Child.Lookback).To(Equal(99))
			Expect(parent.Child.Ratio).To(BeNumerically("~", 0.2, 0.001))
		})
	})

	Context("cycle detection", func() {
		It("returns error when a strategy references itself", func() {
			parent := &cyclicParent{}
			parent.Loop = parent // self-referential cycle
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle"))
		})
	})

	Context("recursive discovery", func() {
		It("discovers grandchildren through nested weighted Strategy fields", func() {
			parent := &parentRecursive{}
			eng = engine.New(parent)

			err := engine.DiscoverChildrenForTest(eng, parent, make(map[uintptr]bool))
			Expect(err).NotTo(HaveOccurred())

			children := engine.EngineChildrenForTest(eng)
			// mid (0.60) + inner (0.30) = 2 entries total
			Expect(children).To(HaveLen(2))

			byName := engine.EngineChildrenByNameForTest(eng)
			Expect(byName).To(HaveKey("mid"))
			Expect(byName).To(HaveKey("inner"))
		})
	})
})
