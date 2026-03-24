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

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

type testStrategy struct {
	Lookback  int     `pvbt:"lookback" desc:"lookback period" default:"90"`
	Threshold float64 `pvbt:"threshold" desc:"signal threshold" default:"0.5"`
}

type universeStrategy struct {
	RiskOn  universe.Universe `pvbt:"risk-on" desc:"equity universe" default:"SPY,GLD"`
	RiskOff universe.Universe `pvbt:"risk-off" desc:"safe-haven" default:"TLT"`
}

func (s *testStrategy) Name() string           { return "test" }
func (s *testStrategy) Setup(e *engine.Engine) {}
func (s *testStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

func (s *universeStrategy) Name() string           { return "universeTest" }
func (s *universeStrategy) Setup(e *engine.Engine) {}
func (s *universeStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

// buildTestCmd creates a root + subcommand tree that mirrors cli.Run,
// wiring registerStrategyFlags and applyStrategyFlags through cobra
// the same way the real backtest/live/snapshot commands do.
func buildTestCmd(strategy engine.Strategy, runBody func()) (*cobra.Command, *cobra.Command) {
	rootCmd := &cobra.Command{Use: "test-strategy"}
	subCmd := &cobra.Command{
		Use: "backtest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyPreset(cmd, strategy); err != nil {
				return err
			}
			applyStrategyFlags(cmd, strategy)
			if runBody != nil {
				runBody()
			}

			return nil
		},
	}

	registerStrategyFlags(subCmd, strategy)
	subCmd.Flags().String("preset", "", "Apply a named parameter preset")
	rootCmd.AddCommand(subCmd)

	return rootCmd, subCmd
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
	It("sets struct fields from parsed cobra flags", func() {
		strategy := &testStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--lookback", "120", "--threshold", "0.75"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.Lookback).To(Equal(120))
		Expect(strategy.Threshold).To(BeNumerically("~", 0.75, 1e-10))
	})
})

var _ = Describe("registerStrategyFlags with universe fields", func() {
	It("registers string flags for universe.Universe fields", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &universeStrategy{}

		registerStrategyFlags(cmd, strategy)

		riskOnFlag := cmd.Flags().Lookup("risk-on")
		Expect(riskOnFlag).NotTo(BeNil())
		Expect(riskOnFlag.DefValue).To(Equal("SPY,GLD"))
		Expect(riskOnFlag.Usage).To(Equal("equity universe"))

		riskOffFlag := cmd.Flags().Lookup("risk-off")
		Expect(riskOffFlag).NotTo(BeNil())
		Expect(riskOffFlag.DefValue).To(Equal("TLT"))
		Expect(riskOffFlag.Usage).To(Equal("safe-haven"))
	})
})

var _ = Describe("applyStrategyFlags with universe fields", func() {
	It("creates a StaticUniverse from user-provided flags", func() {
		strategy := &universeStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--risk-on", "VOO,SCZ,GLD", "--risk-off", "AGG"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.RiskOn).NotTo(BeNil())
		members := strategy.RiskOn.Assets(time.Time{})
		Expect(members).To(HaveLen(3))
		Expect(members[0].Ticker).To(Equal("VOO"))
		Expect(members[1].Ticker).To(Equal("SCZ"))
		Expect(members[2].Ticker).To(Equal("GLD"))

		Expect(strategy.RiskOff).NotTo(BeNil())
		offMembers := strategy.RiskOff.Assets(time.Time{})
		Expect(offMembers).To(HaveLen(1))
		Expect(offMembers[0].Ticker).To(Equal("AGG"))
	})

	It("trims whitespace and upper-cases tickers", func() {
		strategy := &universeStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--risk-on", " spy , gld "})
		Expect(rootCmd.Execute()).To(Succeed())

		members := strategy.RiskOn.Assets(time.Time{})
		Expect(members).To(HaveLen(2))
		Expect(members[0].Ticker).To(Equal("SPY"))
		Expect(members[1].Ticker).To(Equal("GLD"))
	})

	It("uses default tickers when flag is not provided", func() {
		strategy := &universeStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.RiskOn).NotTo(BeNil())
		members := strategy.RiskOn.Assets(time.Time{})
		Expect(members).To(HaveLen(2))
		Expect(members[0].Ticker).To(Equal("SPY"))
		Expect(members[1].Ticker).To(Equal("GLD"))
	})

	It("does not silently fall back to defaults when user provides flags", func() {
		strategy := &universeStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--risk-on", "AAPL,MSFT,GOOG", "--risk-off", "BND"})
		Expect(rootCmd.Execute()).To(Succeed())

		members := strategy.RiskOn.Assets(time.Time{})
		tickers := make([]string, len(members))
		for idx, member := range members {
			tickers[idx] = member.Ticker
		}

		Expect(tickers).NotTo(ContainElement("SPY"), "still using default ticker SPY")
		Expect(tickers).NotTo(ContainElement("GLD"), "still using default ticker GLD")
		Expect(tickers).To(Equal([]string{"AAPL", "MSFT", "GOOG"}))

		offMembers := strategy.RiskOff.Assets(time.Time{})
		Expect(offMembers[0].Ticker).NotTo(Equal("TLT"), "still using default ticker TLT")
		Expect(offMembers[0].Ticker).To(Equal("BND"))
	})

	It("reads from the correct subcommand when multiple subcommands exist", func() {
		strategy := &universeStrategy{}

		// Mirror cli.Run: register strategy flags on multiple subcommands.
		rootCmd := &cobra.Command{Use: "test-strategy"}

		backtestCmd := &cobra.Command{
			Use: "backtest",
			RunE: func(cmd *cobra.Command, args []string) error {
				applyStrategyFlags(cmd, strategy)
				return nil
			},
		}

		liveCmd := &cobra.Command{
			Use:  "live",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}

		snapshotCmd := &cobra.Command{
			Use:  "snapshot",
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		}

		registerStrategyFlags(backtestCmd, strategy)
		registerStrategyFlags(liveCmd, strategy)
		registerStrategyFlags(snapshotCmd, strategy)

		rootCmd.AddCommand(backtestCmd)
		rootCmd.AddCommand(liveCmd)
		rootCmd.AddCommand(snapshotCmd)

		rootCmd.SetArgs([]string{"backtest", "--risk-on", "VOO,SCZ", "--risk-off", "AGG"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.RiskOn).NotTo(BeNil(), "RiskOn is nil after backtest with overrides")
		members := strategy.RiskOn.Assets(time.Time{})
		Expect(members).To(HaveLen(2))
		Expect(members[0].Ticker).To(Equal("VOO"))
		Expect(members[1].Ticker).To(Equal("SCZ"))

		Expect(strategy.RiskOff).NotTo(BeNil())
		offMembers := strategy.RiskOff.Assets(time.Time{})
		Expect(offMembers[0].Ticker).To(Equal("AGG"))
	})
})

type presetStrategy struct {
	RiskOn  string `pvbt:"riskOn"  desc:"equities"   default:"VOO" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
	RiskOff string `pvbt:"riskOff" desc:"safe haven" default:"TLT" suggest:"Classic=VUSTX|Modern=SHY"`
}

func (s *presetStrategy) Name() string           { return "presetTest" }
func (s *presetStrategy) Setup(_ *engine.Engine) {}
func (s *presetStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("applyPreset", func() {
	It("sets flag defaults from the named preset", func() {
		strategy := &presetStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--preset", "Classic"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.RiskOn).To(Equal("VFINX,PRIDX"))
		Expect(strategy.RiskOff).To(Equal("VUSTX"))
	})

	It("allows explicit flags to override preset values", func() {
		strategy := &presetStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--preset", "Classic", "--riskOff", "BND"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.RiskOn).To(Equal("VFINX,PRIDX"))
		Expect(strategy.RiskOff).To(Equal("BND"))
	})

	It("returns an error for unknown preset name", func() {
		strategy := &presetStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--preset", "DoesNotExist"})
		err := rootCmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown preset"))
	})
})
