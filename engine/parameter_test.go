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
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

type paramTestStrategy struct {
	Lookback    float64           `pvbt:"lookback" desc:"Lookback in months" default:"6.0"`
	Ticker      asset.Asset       `pvbt:"ticker" desc:"Primary ticker" default:"SPY"`
	RiskOn      universe.Universe `pvbt:"riskOn" desc:"Risk-on universe" default:"VFINX,PRIDX" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
	Name_       string            // exported, no pvbt tag -- name derived from field name
	RebalanceAt string            // exported, no pvbt tag, multi-word -- exercises kebab-case derivation
	hidden      int               // unexported, should be skipped
	Duration    time.Duration     `pvbt:"dur" desc:"Interval" default:"5m"`
	Enabled     bool              `pvbt:"enabled" desc:"Enable feature" default:"true"`
	Count       int               `pvbt:"count" desc:"Number of items" default:"10"`
	Label       string            `pvbt:"label" desc:"Display label" default:"hello"`
}

func (s *paramTestStrategy) Name() string           { return "test" }
func (s *paramTestStrategy) Setup(_ *engine.Engine) {}
func (s *paramTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

func findParam(params []engine.Parameter, name string) *engine.Parameter {
	for idx := range params {
		if params[idx].Name == name {
			return &params[idx]
		}
	}
	return nil
}

var _ = Describe("StrategyParameters", func() {
	It("extracts exported fields with correct metadata", func() {
		strategy := &paramTestStrategy{}
		params := engine.StrategyParameters(strategy)

		// Should include exported fields only: Lookback, Ticker, RiskOn, Name_, RebalanceAt, Duration, Enabled, Count, Label = 9
		Expect(params).To(HaveLen(9))

		lookback := findParam(params, "lookback")
		Expect(lookback).NotTo(BeNil())
		Expect(lookback.Description).To(Equal("Lookback in months"))
		Expect(lookback.Default).To(Equal("6.0"))
		Expect(lookback.GoType).To(Equal(reflect.TypeOf(float64(0))))
	})

	It("derives a single-word name from the field name when the pvbt tag is missing", func() {
		strategy := &paramTestStrategy{}
		params := engine.StrategyParameters(strategy)

		nameParam := findParam(params, "name_")
		Expect(nameParam).NotTo(BeNil())
		Expect(nameParam.FieldName).To(Equal("Name_"))
	})

	It("derives a kebab-case name for multi-word fields when the pvbt tag is missing", func() {
		strategy := &paramTestStrategy{}
		params := engine.StrategyParameters(strategy)

		rebalanceAt := findParam(params, "rebalance-at")
		Expect(rebalanceAt).NotTo(BeNil())
		Expect(rebalanceAt.FieldName).To(Equal("RebalanceAt"))
	})
})

var _ = Describe("ParameterName", func() {
	fieldByName := func(name string) reflect.StructField {
		t := reflect.TypeOf(paramTestStrategy{})
		field, ok := t.FieldByName(name)
		Expect(ok).To(BeTrue())
		return field
	}

	It("returns the pvbt tag verbatim when present", func() {
		Expect(engine.ParameterName(fieldByName("Lookback"))).To(Equal("lookback"))
		Expect(engine.ParameterName(fieldByName("RiskOn"))).To(Equal("riskOn"))
	})

	It("derives kebab-case from a PascalCase field name when the tag is missing", func() {
		Expect(engine.ParameterName(fieldByName("RebalanceAt"))).To(Equal("rebalance-at"))
	})

	It("leaves a single-word exported field lowercased", func() {
		Expect(engine.ParameterName(fieldByName("Name_"))).To(Equal("name_"))
	})

	It("parses suggest tags into a suggestions map", func() {
		strategy := &paramTestStrategy{}
		params := engine.StrategyParameters(strategy)

		riskOn := findParam(params, "riskOn")
		Expect(riskOn).NotTo(BeNil())
		Expect(riskOn.Suggestions).To(HaveLen(2))
		Expect(riskOn.Suggestions["Classic"]).To(Equal("VFINX,PRIDX"))
		Expect(riskOn.Suggestions["Modern"]).To(Equal("SPY,QQQ"))
	})

	It("leaves suggestions nil when no suggest tag is present", func() {
		strategy := &paramTestStrategy{}
		params := engine.StrategyParameters(strategy)

		lookback := findParam(params, "lookback")
		Expect(lookback).NotTo(BeNil())
		Expect(lookback.Suggestions).To(BeNil())
	})
})

type testOnlyTagStrategy struct {
	Visible     int `pvbt:"visible" desc:"v" default:"1"`
	HiddenTrue  int `pvbt:"hidden-true" testonly:"true"`
	HiddenFalse int `pvbt:"hidden-false" testonly:"false"`
}

func (s *testOnlyTagStrategy) Name() string           { return "testOnlyTag" }
func (s *testOnlyTagStrategy) Setup(_ *engine.Engine) {}
func (s *testOnlyTagStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("IsTestOnlyField", func() {
	fieldByName := func(name string) reflect.StructField {
		t := reflect.TypeOf(testOnlyTagStrategy{})
		field, ok := t.FieldByName(name)
		Expect(ok).To(BeTrue())
		return field
	}

	It("returns false when the testonly tag is absent", func() {
		Expect(engine.IsTestOnlyField(fieldByName("Visible"))).To(BeFalse())
	})

	It("returns true when the testonly tag is \"true\"", func() {
		Expect(engine.IsTestOnlyField(fieldByName("HiddenTrue"))).To(BeTrue())
	})

	It("returns false when the testonly tag is \"false\"", func() {
		Expect(engine.IsTestOnlyField(fieldByName("HiddenFalse"))).To(BeFalse())
	})

	It("panics when the testonly tag has an unparseable value", func() {
		type bad struct {
			Field int `pvbt:"x" testonly:"banana"`
		}
		field, ok := reflect.TypeOf(bad{}).FieldByName("Field")
		Expect(ok).To(BeTrue())
		Expect(func() { engine.IsTestOnlyField(field) }).To(PanicWith(ContainSubstring("invalid testonly tag")))
	})
})
