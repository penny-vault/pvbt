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

package cli

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

type testStrategy struct {
	Lookback  int     `pvbt:"lookback" desc:"lookback period" default:"90"`
	Threshold float64 `pvbt:"threshold" desc:"signal threshold" default:"0.5"`
}

func (s *testStrategy) Name() string                                                       { return "test" }
func (s *testStrategy) Setup(e *engine.Engine)                                              {}
func (s *testStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}

var _ = Describe("runID", func() {
	It("returns a full UUID and a 5-char prefix", func() {
		fullID, shortID := runID()

		Expect(fullID).To(HaveLen(36))
		// UUID format: 8-4-4-4-12
		parts := strings.Split(fullID, "-")
		Expect(parts).To(HaveLen(5))
		Expect(parts[0]).To(HaveLen(8))
		Expect(parts[1]).To(HaveLen(4))
		Expect(parts[2]).To(HaveLen(4))
		Expect(parts[3]).To(HaveLen(4))
		Expect(parts[4]).To(HaveLen(12))

		Expect(shortID).To(HaveLen(5))
		Expect(shortID).To(Equal(fullID[:5]))
	})

	It("generates unique IDs on successive calls", func() {
		id1, _ := runID()
		id2, _ := runID()
		Expect(id1).NotTo(Equal(id2))
	})
})

var _ = Describe("defaultOutputPath", func() {
	It("generates the correct filename pattern", func() {
		start := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)
		end := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
		shortID := "ab12c"

		path := defaultOutputPath("MyStrategy", start, end, shortID)
		Expect(path).To(Equal("mystrategy-backtest-20200115-20250630-ab12c.db"))
	})
})

var _ = Describe("toKebabCase", func() {
	It("converts PascalCase to kebab-case", func() {
		Expect(toKebabCase("LookbackPeriod")).To(Equal("lookback-period"))
	})

	It("converts consecutive uppercase letters", func() {
		Expect(toKebabCase("URL")).To(Equal("u-r-l"))
	})

	It("leaves lowercase unchanged", func() {
		Expect(toKebabCase("fast")).To(Equal("fast"))
	})

	It("handles single character", func() {
		Expect(toKebabCase("A")).To(Equal("a"))
	})

	It("handles empty string", func() {
		Expect(toKebabCase("")).To(Equal(""))
	})

	It("converts camelCase", func() {
		Expect(toKebabCase("myField")).To(Equal("my-field"))
	})
})

var _ = Describe("registerStrategyFlags", func() {
	It("registers flags from struct tags with correct defaults", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlags(cmd, strategy)

		lookbackFlag := cmd.Flags().Lookup("lookback")
		Expect(lookbackFlag).NotTo(BeNil())
		Expect(lookbackFlag.DefValue).To(Equal("90"))
		Expect(lookbackFlag.Usage).To(Equal("lookback period"))

		thresholdFlag := cmd.Flags().Lookup("threshold")
		Expect(thresholdFlag).NotTo(BeNil())
		Expect(thresholdFlag.DefValue).To(Equal("0.5"))
		Expect(thresholdFlag.Usage).To(Equal("signal threshold"))
	})
})

var _ = Describe("applyStrategyFlags", func() {
	It("sets struct fields from viper values", func() {
		strategy := &testStrategy{}

		// Reset viper to avoid pollution between tests.
		viper.Reset()
		viper.Set("lookback", 120)
		viper.Set("threshold", 0.75)

		applyStrategyFlags(strategy)

		Expect(strategy.Lookback).To(Equal(120))
		Expect(strategy.Threshold).To(BeNumerically("~", 0.75, 1e-10))
	})
})
