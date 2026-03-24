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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// childStub is a minimal Strategy used to test that Strategy-typed fields are
// skipped by StrategyParameters and hydrateFields.
type childStub struct{}

func (stub *childStub) Name() string           { return "child" }
func (stub *childStub) Setup(_ *engine.Engine) {}
func (stub *childStub) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (stub *childStub) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{Schedule: "0 16 * * 1-5"}
}

// parentWithChild has one regular parameter alongside a Strategy-typed field.
type parentWithChild struct {
	Lookback int        `pvbt:"lookback" desc:"lookback" default:"6"`
	Child    *childStub `pvbt:"child" weight:"0.50"`
}

func (parent *parentWithChild) Name() string           { return "parentWithChild" }
func (parent *parentWithChild) Setup(_ *engine.Engine) {}
func (parent *parentWithChild) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// descriptorStrategy implements both Strategy and Descriptor.
type descriptorStrategy struct {
	Lookback int    `pvbt:"lookback" desc:"Lookback period" default:"6"`
	Tickers  string `pvbt:"tickers" desc:"Asset tickers" default:"SPY,QQQ" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
}

func (s *descriptorStrategy) Name() string           { return "DescriptorTest" }
func (s *descriptorStrategy) Setup(_ *engine.Engine) {}
func (s *descriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}
func (s *descriptorStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode:   "dt",
		Description: "A test strategy with descriptor",
		Source:      "unit test",
		Version:     "1.0.0",
		Schedule:    "0 16 * * 1-5",
		Benchmark:   "SPY",
		Warmup:      14,
	}
}

// plainStrategy implements only Strategy (no Descriptor).
type plainStrategy struct {
	Window int `pvbt:"window" desc:"Rolling window" default:"12"`
}

func (s *plainStrategy) Name() string           { return "PlainTest" }
func (s *plainStrategy) Setup(_ *engine.Engine) {}
func (s *plainStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("Strategy child-field skipping", func() {
	It("excludes Strategy-typed fields from StrategyParameters", func() {
		params := engine.StrategyParameters(&parentWithChild{})
		Expect(params).To(HaveLen(1))
		Expect(params[0].Name).To(Equal("lookback"))
	})

	It("does not error when hydrateFields encounters a Strategy-typed field", func() {
		target := &parentWithChild{}
		err := engine.HydrateFieldsForTest(nil, target)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Lookback).To(Equal(6))
	})
})

var _ = Describe("DescribeStrategy", func() {
	Context("with a Descriptor implementation", func() {
		It("populates all fields from the descriptor and strategy", func() {
			strategy := &descriptorStrategy{}
			info := engine.DescribeStrategy(strategy)

			Expect(info.Name).To(Equal("DescriptorTest"))
			Expect(info.ShortCode).To(Equal("dt"))
			Expect(info.Description).To(Equal("A test strategy with descriptor"))
			Expect(info.Source).To(Equal("unit test"))
			Expect(info.Version).To(Equal("1.0.0"))
			Expect(info.Schedule).To(Equal("0 16 * * 1-5"))
			Expect(info.Benchmark).To(Equal("SPY"))
			Expect(info.Warmup).To(Equal(14))
			Expect(info.RiskFree).To(Equal("DGS3MO"))
			Expect(info.Parameters).To(HaveLen(2))
			Expect(info.Suggestions).To(HaveLen(2))
			Expect(info.Suggestions["Classic"]["tickers"]).To(Equal("VFINX,PRIDX"))
		})

		It("round-trips through JSON", func() {
			strategy := &descriptorStrategy{}
			info := engine.DescribeStrategy(strategy)

			encoded, err := json.Marshal(info)
			Expect(err).NotTo(HaveOccurred())

			var decoded engine.StrategyInfo
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded.Name).To(Equal(info.Name))
			Expect(decoded.ShortCode).To(Equal(info.ShortCode))
		})

		It("includes warmup in JSON output", func() {
			strategy := &descriptorStrategy{}
			info := engine.DescribeStrategy(strategy)
			encoded, err := json.Marshal(info)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(encoded)).To(ContainSubstring(`"warmup":14`))
		})
	})

	It("serializes Schedule and Benchmark fields", func() {
		desc := engine.StrategyDescription{
			ShortCode:   "test",
			Description: "test strategy",
			Schedule:    "@monthend",
			Benchmark:   "SPY",
		}

		data, err := json.Marshal(desc)
		Expect(err).NotTo(HaveOccurred())

		var parsed map[string]interface{}
		Expect(json.Unmarshal(data, &parsed)).To(Succeed())
		Expect(parsed["schedule"]).To(Equal("@monthend"))
		Expect(parsed["benchmark"]).To(Equal("SPY"))
	})

	Context("without a Descriptor implementation", func() {
		It("uses defaults for missing descriptor fields", func() {
			strategy := &plainStrategy{}
			info := engine.DescribeStrategy(strategy)

			Expect(info.Name).To(Equal("PlainTest"))
			Expect(info.ShortCode).To(BeEmpty())
			Expect(info.Schedule).To(BeEmpty())
			Expect(info.Benchmark).To(BeEmpty())
			Expect(info.Warmup).To(Equal(0))
			Expect(info.RiskFree).To(Equal("DGS3MO"))
			Expect(info.Suggestions).To(BeNil())
			Expect(info.Parameters).To(HaveLen(1))
			Expect(info.Parameters[0].Name).To(Equal("window"))
			Expect(info.Parameters[0].Type).To(Equal("int"))
		})

		It("omits empty fields in JSON", func() {
			strategy := &plainStrategy{}
			info := engine.DescribeStrategy(strategy)

			encoded, err := json.Marshal(info)
			Expect(err).NotTo(HaveOccurred())

			jsonStr := string(encoded)
			Expect(jsonStr).NotTo(ContainSubstring(`"shortcode"`))
			Expect(jsonStr).NotTo(ContainSubstring(`"source"`))
			Expect(jsonStr).NotTo(ContainSubstring(`"version"`))
		})
	})
})
