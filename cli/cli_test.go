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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/penny-vault/pvbt/asset"
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

	It("returns the kebab-case names of fields it actually wrote", func() {
		strategy := &testStrategy{}
		cmd := &cobra.Command{Use: "test"}
		registerStrategyFlags(cmd, strategy)

		Expect(cmd.Flags().Set("lookback", "0")).To(Succeed())

		applied := applyStrategyFlags(cmd, strategy)

		// testStrategy has lookback (int) and threshold (float64).
		// Both have flags, both get set on every call (one to user value,
		// one to cobra default), so both should appear in the applied list.
		Expect(applied).To(ConsistOf("lookback", "threshold"))
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

type assetStrategy struct {
	Bench asset.Asset `pvbt:"bench" desc:"benchmark ticker" default:"SPY"`
	Lev   asset.Asset `pvbt:"lev"   desc:"leveraged ticker"`
}

func (s *assetStrategy) Name() string           { return "assetTest" }
func (s *assetStrategy) Setup(_ *engine.Engine) {}
func (s *assetStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("registerStrategyFlags with asset.Asset fields", func() {
	It("registers a string flag carrying the default ticker", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &assetStrategy{}

		registerStrategyFlags(cmd, strategy)

		benchFlag := cmd.Flags().Lookup("bench")
		Expect(benchFlag).NotTo(BeNil())
		Expect(benchFlag.DefValue).To(Equal("SPY"))
		Expect(benchFlag.Usage).To(Equal("benchmark ticker"))

		levFlag := cmd.Flags().Lookup("lev")
		Expect(levFlag).NotTo(BeNil())
		Expect(levFlag.DefValue).To(BeEmpty())
	})
})

var _ = Describe("applyStrategyFlags with asset.Asset fields", func() {
	It("sets the field's Ticker from a user-provided flag", func() {
		strategy := &assetStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest", "--bench", "qqq", "--lev", " tqqq "})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.Bench.Ticker).To(Equal("QQQ"))
		Expect(strategy.Lev.Ticker).To(Equal("TQQQ"))
	})

	It("falls back to the default tag value when no flag is given", func() {
		strategy := &assetStrategy{}
		rootCmd, _ := buildTestCmd(strategy, nil)

		rootCmd.SetArgs([]string{"backtest"})
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(strategy.Bench.Ticker).To(Equal("SPY"))
		Expect(strategy.Lev.Ticker).To(BeEmpty())
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

type testOnlyFlagStrategy struct {
	Lookback int `pvbt:"lookback" desc:"lookback" default:"30"`
	Seed     int `pvbt:"seed" testonly:"true"`
	Window   int `pvbt:"window" desc:"window" default:"5"`
}

func (s *testOnlyFlagStrategy) Name() string           { return "testOnlyFlag" }
func (s *testOnlyFlagStrategy) Setup(e *engine.Engine) {}
func (s *testOnlyFlagStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("registerStrategyFlags with testonly fields", func() {
	It("does not register a cobra flag for a test-only field", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testOnlyFlagStrategy{}

		registerStrategyFlags(cmd, strategy)

		Expect(cmd.Flags().Lookup("lookback")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("window")).NotTo(BeNil())
		Expect(cmd.Flags().Lookup("seed")).To(BeNil())
	})
})

var _ = Describe("applyStrategyFlags with testonly fields", func() {
	It("leaves a test-only field untouched even if a flag is registered out of band", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testOnlyFlagStrategy{}

		// Manually register a flag for "seed" to simulate a flag being
		// registered out of band (e.g., by another command). The
		// test-only check inside applyStrategyFlags must still skip the
		// field, leaving it at its zero value.
		cmd.Flags().Int("seed", 999, "")

		applyStrategyFlags(cmd, strategy)
		Expect(strategy.Seed).To(Equal(0))
	})
})

var _ = Describe("collectParamSweeps with testonly fields", func() {
	It("does not collect a sweep for a test-only field even if a flag is registered out of band", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testOnlyFlagStrategy{}

		registerStrategyFlags(cmd, strategy)

		// Manually register a "seed" flag with range syntax. Without the
		// explicit IsTestOnlyField check inside collectParamSweeps, this
		// would be picked up as a parameter sweep.
		cmd.Flags().String("seed", "1:5:1", "")

		sweeps := collectParamSweeps(cmd, strategy)
		for _, sweep := range sweeps {
			Expect(sweep.Field()).NotTo(Equal("seed"))
		}
	})
})

var _ = Describe("registerStrategyFlagsForSweep", func() {
	It("accepts min:max:step syntax on int fields", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		Expect(cmd.Flags().Set("lookback", "0:8:1")).To(Succeed(),
			"int fields must accept colon-range syntax under sweep registration")

		fl := cmd.Flags().Lookup("lookback")
		Expect(fl).NotTo(BeNil())
		Expect(fl.Value.String()).To(Equal("0:8:1"))
	})

	It("accepts min:max:step syntax on float64 fields", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		Expect(cmd.Flags().Set("threshold", "0.1:0.5:0.1")).To(Succeed(),
			"float64 fields must accept colon-range syntax under sweep registration")
	})

	It("still accepts a fixed single value", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		Expect(cmd.Flags().Set("lookback", "5")).To(Succeed())
		Expect(cmd.Flags().Set("threshold", "0.7")).To(Succeed())
	})

	It("preserves the field's default value when the flag is not set", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		Expect(cmd.Flags().Lookup("lookback").DefValue).To(Equal("90"))
		Expect(cmd.Flags().Lookup("threshold").DefValue).To(Equal("0.5"))
	})
})

var _ = Describe("collectFixedParams", func() {
	It("returns user-set values for non-swept fields", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		Expect(cmd.Flags().Set("lookback", "5")).To(Succeed())
		Expect(cmd.Flags().Set("threshold", "0.1:0.5:0.1")).To(Succeed())

		sweeps := collectParamSweeps(cmd, strategy)
		fixed := collectFixedParams(cmd, strategy, sweeps)

		Expect(fixed).To(HaveKeyWithValue("lookback", "5"))
		Expect(fixed).NotTo(HaveKey("threshold"),
			"swept fields must not appear as fixed params")
	})

	It("returns nil when no flags were changed", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		fixed := collectFixedParams(cmd, strategy, nil)
		Expect(fixed).To(BeNil(),
			"defaults should not become fixed params")
	})

	It("excludes test-only fields", func() {
		cmd := &cobra.Command{Use: "test"}
		strategy := &testOnlyFlagStrategy{}

		registerStrategyFlagsForSweep(cmd, strategy)

		// Register a "seed" flag out of band (mirroring the existing
		// collectParamSweeps test) and set it; the test-only check
		// inside collectFixedParams must still skip the field.
		cmd.Flags().Int("seed", 0, "")
		Expect(cmd.Flags().Set("seed", "42")).To(Succeed())

		fixed := collectFixedParams(cmd, strategy, nil)
		Expect(fixed).NotTo(HaveKey("seed"),
			"test-only fields must never propagate via fixed params")
	})
})

var _ = Describe("strategyParams with testonly fields", func() {
	It("does not include a test-only field in the backtest metadata map", func() {
		strategy := &testOnlyFlagStrategy{}

		params := strategyParams(strategy)

		Expect(params).NotTo(HaveKey("seed"))
		Expect(params).To(HaveKey("lookback"))
		Expect(params).To(HaveKey("window"))
	})
})

// addCPUBurnSubcommand registers a lightweight "burn" subcommand on the
// given root. The subcommand spins in a small arithmetic loop for a few
// hundred milliseconds so the Go CPU profiler has time to collect
// samples. It is used only from the --cpu-profile tests below so we can
// exercise the persistent pre/post-run hooks without bringing up the
// full backtest runtime (data provider, config, output DB, ...).
func addCPUBurnSubcommand(rootCmd *cobra.Command) {
	burnCmd := &cobra.Command{
		Use: "burn",
		RunE: func(_ *cobra.Command, _ []string) error {
			deadline := time.Now().Add(400 * time.Millisecond)
			accumulator := 0.0
			for iteration := 0; time.Now().Before(deadline); iteration++ {
				accumulator += float64(iteration) * 1.0000001
				if accumulator > 1e18 {
					accumulator = 0
				}
			}
			return nil
		},
	}
	rootCmd.AddCommand(burnCmd)
}

// addFailingBurnSubcommand mirrors addCPUBurnSubcommand but its RunE
// returns a sentinel error after the CPU burn completes. It is used to
// exercise the code path where a subcommand errors out so the tests can
// verify that the CPU profile is still flushed to disk. Cobra does not
// invoke PersistentPostRunE when a subcommand's RunE returns a non-nil
// error, so the profile cleanup must happen via a deferred callback
// wired around rootCmd.Execute.
func addFailingBurnSubcommand(rootCmd *cobra.Command) {
	burnCmd := &cobra.Command{
		Use: "burn-fail",
		RunE: func(_ *cobra.Command, _ []string) error {
			deadline := time.Now().Add(200 * time.Millisecond)
			accumulator := 0.0
			for iteration := 0; time.Now().Before(deadline); iteration++ {
				accumulator += float64(iteration) * 1.0000001
				if accumulator > 1e18 {
					accumulator = 0
				}
			}
			return errors.New("test burn-fail")
		},
	}
	rootCmd.AddCommand(burnCmd)
}

var _ = Describe("--cpu-profile persistent flag", func() {
	It("writes a CPU profile when --cpu-profile is set", func() {
		tmpDir := GinkgoT().TempDir()
		profilePath := filepath.Join(tmpDir, "cpu.prof")

		strategy := &testStrategy{}

		// Mirror how cli.Run wraps rootCmd.Execute with a deferred
		// cleanup in an inner function so the defer fires -- which
		// stops the profile and closes the file -- before the
		// assertions below inspect the file on disk.
		execErr := func() error {
			rootCmd, cleanup := newRootCmd(strategy)
			defer cleanup()

			addCPUBurnSubcommand(rootCmd)
			rootCmd.SetArgs([]string{"burn", "--cpu-profile", profilePath})

			return rootCmd.Execute()
		}()
		Expect(execErr).NotTo(HaveOccurred())

		info, err := os.Stat(profilePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Size()).To(BeNumerically(">", 0),
			"CPU profile file must be non-empty after a command run")
	})

	It("does not create a profile file when --cpu-profile is not set", func() {
		tmpDir := GinkgoT().TempDir()
		profilePath := filepath.Join(tmpDir, "cpu.prof")

		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()
		addCPUBurnSubcommand(rootCmd)

		rootCmd.SetArgs([]string{"burn"})
		Expect(rootCmd.Execute()).To(Succeed())

		_, err := os.Stat(profilePath)
		Expect(os.IsNotExist(err)).To(BeTrue(),
			"profile file should not exist when --cpu-profile is unset")
	})

	It("returns an error when --cpu-profile target cannot be created", func() {
		tmpDir := GinkgoT().TempDir()
		// A path inside a non-existent directory cannot be created.
		profilePath := filepath.Join(tmpDir, "does-not-exist", "cpu.prof")

		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()
		addCPUBurnSubcommand(rootCmd)

		rootCmd.SetArgs([]string{"burn", "--cpu-profile", profilePath})
		err := rootCmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cpu profile"))
	})

	It("exposes --cpu-profile as a persistent flag on every subcommand", func() {
		strategy := &testStrategy{}
		rootCmd, cleanup := newRootCmd(strategy)
		defer cleanup()

		for _, sub := range rootCmd.Commands() {
			flag := sub.PersistentFlags().Lookup("cpu-profile")
			if flag == nil {
				flag = sub.InheritedFlags().Lookup("cpu-profile")
			}
			Expect(flag).NotTo(BeNil(),
				"subcommand %q should inherit the --cpu-profile flag", sub.Use)
		}
	})

	It("still flushes the CPU profile when the subcommand returns an error", func() {
		tmpDir := GinkgoT().TempDir()
		profilePath := filepath.Join(tmpDir, "cpu.prof")

		strategy := &testStrategy{}

		// Mirror how cli.Run wraps rootCmd.Execute with a deferred
		// cleanup in an inner function so the defer fires *before* the
		// assertions run. If the fix is in place, cleanup stops the
		// profile and closes the file; if it regresses back to
		// PersistentPostRunE, the file stays open and empty because
		// cobra skips PersistentPostRunE when RunE returns an error.
		execErr := func() error {
			rootCmd, cleanup := newRootCmd(strategy)
			defer cleanup()

			addFailingBurnSubcommand(rootCmd)
			rootCmd.SetArgs([]string{"burn-fail", "--cpu-profile", profilePath})

			return rootCmd.Execute()
		}()

		Expect(execErr).To(HaveOccurred(),
			"burn-fail must propagate its error so we're actually testing the error path")

		info, statErr := os.Stat(profilePath)
		Expect(statErr).NotTo(HaveOccurred(),
			"profile file must exist even when the subcommand errored")
		Expect(info.Size()).To(BeNumerically(">", 0),
			"profile file must be non-empty -- pprof.StopCPUProfile must have run")
	})
})
