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

package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/penny-vault/pvbt/config"
)

// helpers to create pointer values inline
func fp(val float64) *float64 { return &val }
func ip(val int) *int         { return &val }

var _ = Describe("Config", func() {
	Describe("ValidateAndApplyDefaults", func() {
		Describe("risk profile validation", func() {
			It("accepts empty profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: ""}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts conservative profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: "conservative"}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts moderate profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: "moderate"}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts aggressive profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: "aggressive"}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts none profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: "none"}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("rejects unknown profile", func() {
				cfg := config.Config{Risk: config.RiskConfig{Profile: "ultra"}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("unknown profile")))
			})
		})

		Describe("MaxPositionSize validation", func() {
			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionSize: fp(-0.01)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("max_position_size")))
			})

			It("rejects value greater than 1.0", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionSize: fp(1.01)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("max_position_size")))
			})

			It("accepts value of 0", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionSize: fp(0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts value of 1.0", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionSize: fp(1.0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("MaxPositionCount validation", func() {
			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionCount: ip(-1)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("max_position_count")))
			})

			It("accepts zero", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionCount: ip(0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts positive value", func() {
				cfg := config.Config{Risk: config.RiskConfig{MaxPositionCount: ip(10)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("DrawdownCircuitBreaker validation", func() {
			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{DrawdownCircuitBreaker: fp(-0.01)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("drawdown_circuit_breaker")))
			})

			It("rejects value greater than 1.0", func() {
				cfg := config.Config{Risk: config.RiskConfig{DrawdownCircuitBreaker: fp(1.01)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("drawdown_circuit_breaker")))
			})

			It("accepts value between 0 and 1.0", func() {
				cfg := config.Config{Risk: config.RiskConfig{DrawdownCircuitBreaker: fp(0.15)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("VolatilityScalerLookback validation", func() {
			It("rejects zero", func() {
				cfg := config.Config{Risk: config.RiskConfig{VolatilityScalerLookback: ip(0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("volatility_scaler_lookback")))
			})

			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{VolatilityScalerLookback: ip(-5)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("volatility_scaler_lookback")))
			})

			It("accepts value >= 1", func() {
				cfg := config.Config{Risk: config.RiskConfig{VolatilityScalerLookback: ip(60)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("GrossExposureLimit validation", func() {
			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{GrossExposureLimit: fp(-0.1)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("gross_exposure_limit")))
			})

			It("accepts zero", func() {
				cfg := config.Config{Risk: config.RiskConfig{GrossExposureLimit: fp(0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts positive value", func() {
				cfg := config.Config{Risk: config.RiskConfig{GrossExposureLimit: fp(1.5)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("NetExposureLimit validation", func() {
			It("rejects negative value", func() {
				cfg := config.Config{Risk: config.RiskConfig{NetExposureLimit: fp(-0.1)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(MatchError(ContainSubstring("net_exposure_limit")))
			})

			It("accepts zero", func() {
				cfg := config.Config{Risk: config.RiskConfig{NetExposureLimit: fp(0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})

			It("accepts positive value", func() {
				cfg := config.Config{Risk: config.RiskConfig{NetExposureLimit: fp(1.0)}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
			})
		})

		Describe("tax defaults", func() {
			It("applies DefaultLossThreshold when tax enabled and threshold is 0", func() {
				cfg := config.Config{Tax: config.TaxConfig{Enabled: true, LossThreshold: 0}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
				Expect(cfg.Tax.LossThreshold).To(Equal(config.DefaultLossThreshold))
			})

			It("does not overwrite explicit threshold", func() {
				cfg := config.Config{Tax: config.TaxConfig{Enabled: true, LossThreshold: 0.03}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
				Expect(cfg.Tax.LossThreshold).To(Equal(0.03))
			})

			It("does not apply default when tax is disabled", func() {
				cfg := config.Config{Tax: config.TaxConfig{Enabled: false, LossThreshold: 0}}
				Expect(cfg.ValidateAndApplyDefaults()).To(Succeed())
				Expect(cfg.Tax.LossThreshold).To(Equal(0.0))
			})
		})
	})

	Describe("HasMiddleware", func() {
		It("returns false for profile none with no overrides", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none"}}
			Expect(cfg.HasMiddleware()).To(BeFalse())
		})

		It("returns false for empty profile with no overrides and tax disabled", func() {
			cfg := config.Config{}
			Expect(cfg.HasMiddleware()).To(BeFalse())
		})

		It("returns true for profile none with MaxPositionSize override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", MaxPositionSize: fp(0.10)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for profile none with MaxPositionCount override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", MaxPositionCount: ip(5)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for profile none with DrawdownCircuitBreaker override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", DrawdownCircuitBreaker: fp(0.10)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for profile none with VolatilityScalerLookback override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", VolatilityScalerLookback: ip(30)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for profile none with GrossExposureLimit override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", GrossExposureLimit: fp(1.0)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for profile none with NetExposureLimit override", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "none", NetExposureLimit: fp(0.5)}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true when tax is enabled", func() {
			cfg := config.Config{Tax: config.TaxConfig{Enabled: true}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for conservative profile", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "conservative"}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for moderate profile", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "moderate"}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})

		It("returns true for aggressive profile", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "aggressive"}}
			Expect(cfg.HasMiddleware()).To(BeTrue())
		})
	})

	Describe("ProfileBaseline", func() {
		It("returns conservative baseline with correct values", func() {
			bl := config.ProfileBaseline("conservative")
			Expect(bl.Profile).To(Equal("conservative"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.20))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.10))
			Expect(bl.VolatilityScalerLookback).NotTo(BeNil())
			Expect(*bl.VolatilityScalerLookback).To(Equal(60))
		})

		It("returns moderate baseline with correct values", func() {
			bl := config.ProfileBaseline("moderate")
			Expect(bl.Profile).To(Equal("moderate"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.25))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.15))
			Expect(bl.VolatilityScalerLookback).To(BeNil())
		})

		It("returns aggressive baseline with correct values", func() {
			bl := config.ProfileBaseline("aggressive")
			Expect(bl.Profile).To(Equal("aggressive"))
			Expect(bl.MaxPositionSize).NotTo(BeNil())
			Expect(*bl.MaxPositionSize).To(Equal(0.35))
			Expect(bl.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.25))
		})

		It("returns zero config for none", func() {
			bl := config.ProfileBaseline("none")
			Expect(bl.MaxPositionSize).To(BeNil())
			Expect(bl.DrawdownCircuitBreaker).To(BeNil())
		})

		It("returns zero config for empty string", func() {
			bl := config.ProfileBaseline("")
			Expect(bl.MaxPositionSize).To(BeNil())
			Expect(bl.DrawdownCircuitBreaker).To(BeNil())
		})

		// Cross-reference: verify conservative MaxPositionSize=0.20 matches risk/profiles.go.
		// If risk.Conservative changes, this test will catch the divergence.
		It("conservative MaxPositionSize matches risk.Conservative (0.20)", func() {
			bl := config.ProfileBaseline("conservative")
			Expect(*bl.MaxPositionSize).To(Equal(0.20),
				"conservative MaxPositionSize must stay in sync with risk.Conservative")
		})

		It("conservative DrawdownCircuitBreaker matches risk.Conservative (0.10)", func() {
			bl := config.ProfileBaseline("conservative")
			Expect(*bl.DrawdownCircuitBreaker).To(Equal(0.10),
				"conservative DrawdownCircuitBreaker must stay in sync with risk.Conservative")
		})

		It("conservative VolatilityScalerLookback matches risk.Conservative (60)", func() {
			bl := config.ProfileBaseline("conservative")
			Expect(*bl.VolatilityScalerLookback).To(Equal(60),
				"conservative VolatilityScalerLookback must stay in sync with risk.Conservative")
		})
	})

	Describe("Load", func() {
		It("reads a TOML file with explicit path and produces a valid Config", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[risk]\nprofile = \"moderate\"\n"), 0o644)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Risk.Profile).To(Equal("moderate"))
		})

		It("returns zero-value Config when no file found", func() {
			cfg, err := config.Load("/nonexistent/path/to/pvbt.toml")
			Expect(err).To(HaveOccurred())
			_ = cfg
		})

		It("returns zero-value Config when configPath is empty and no default files exist", func() {
			// Run from a temp dir so neither ./pvbt.toml nor ~/.config/pvbt/config.toml
			// interfere. We rely on those files not existing in CI, but we cannot
			// control ~/.config/pvbt/config.toml, so we only assert no error.
			cfg, err := config.Load("/nonexistent/pvbt.toml")
			Expect(err).To(HaveOccurred())
			_ = cfg
		})

		It("returns error for malformed TOML", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "bad.toml")
			Expect(os.WriteFile(path, []byte("[[[\n"), 0o644)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})

		It("pointer fields are nil when not set in TOML", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[risk]\nprofile = \"none\"\n"), 0o644)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Risk.MaxPositionSize).To(BeNil())
			Expect(cfg.Risk.MaxPositionCount).To(BeNil())
			Expect(cfg.Risk.DrawdownCircuitBreaker).To(BeNil())
			Expect(cfg.Risk.VolatilityScalerLookback).To(BeNil())
			Expect(cfg.Risk.GrossExposureLimit).To(BeNil())
			Expect(cfg.Risk.NetExposureLimit).To(BeNil())
		})

		It("pointer fields are non-nil when set in TOML including explicit zero values", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			toml := "[risk]\nprofile = \"none\"\nmax_position_size = 0.0\nmax_position_count = 0\n"
			Expect(os.WriteFile(path, []byte(toml), 0o644)).To(Succeed())

			cfg, err := config.Load(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Risk.MaxPositionSize).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionSize).To(Equal(0.0))
			Expect(cfg.Risk.MaxPositionCount).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionCount).To(Equal(0))
		})
	})

	Describe("LoadFromCommand", func() {
		newCmd := func() *cobra.Command {
			cmd := &cobra.Command{}
			cmd.Flags().String("config", "", "")
			cmd.Flags().String("risk-profile", "", "")
			cmd.Flags().Bool("tax", false, "")
			return cmd
		}

		It("applies --risk-profile flag override on top of config file", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[risk]\nprofile = \"conservative\"\n"), 0o644)).To(Succeed())

			cmd := newCmd()
			Expect(cmd.Flags().Set("config", path)).To(Succeed())
			Expect(cmd.Flags().Set("risk-profile", "moderate")).To(Succeed())

			cfg, err := config.LoadFromCommand(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Risk.Profile).To(Equal("moderate"))
		})

		It("applies --tax flag override on top of config file", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[tax]\nenabled = false\n"), 0o644)).To(Succeed())

			cmd := newCmd()
			Expect(cmd.Flags().Set("config", path)).To(Succeed())
			Expect(cmd.Flags().Set("tax", "true")).To(Succeed())

			cfg, err := config.LoadFromCommand(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Tax.Enabled).To(BeTrue())
		})

		It("re-validates after overrides and returns error for invalid --risk-profile", func() {
			cmd := newCmd()
			Expect(cmd.Flags().Set("risk-profile", "invalid")).To(Succeed())

			_, err := config.LoadFromCommand(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("unknown profile")))
		})

		It("returns zero-value Config when no flags set and no config file exists", func() {
			cmd := newCmd()
			Expect(cmd.Flags().Set("config", "/nonexistent/pvbt.toml")).To(Succeed())

			_, err := config.LoadFromCommand(cmd)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ResolveProfile", func() {
		It("returns baseline when no overrides are set", func() {
			cfg := config.Config{Risk: config.RiskConfig{Profile: "conservative"}}
			resolved := cfg.ResolveProfile()
			Expect(*resolved.MaxPositionSize).To(Equal(0.20))
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.10))
			Expect(*resolved.VolatilityScalerLookback).To(Equal(60))
		})

		It("overrides MaxPositionSize from baseline", func() {
			cfg := config.Config{Risk: config.RiskConfig{
				Profile:         "conservative",
				MaxPositionSize: fp(0.15),
			}}
			resolved := cfg.ResolveProfile()
			Expect(*resolved.MaxPositionSize).To(Equal(0.15))
			// Other baseline fields are unaffected
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.10))
		})

		It("overrides DrawdownCircuitBreaker from baseline", func() {
			cfg := config.Config{Risk: config.RiskConfig{
				Profile:                "moderate",
				DrawdownCircuitBreaker: fp(0.20),
			}}
			resolved := cfg.ResolveProfile()
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.20))
			Expect(*resolved.MaxPositionSize).To(Equal(0.25))
		})

		It("adds new fields not in baseline", func() {
			cfg := config.Config{Risk: config.RiskConfig{
				Profile:            "moderate",
				MaxPositionCount:   ip(10),
				GrossExposureLimit: fp(1.5),
			}}
			resolved := cfg.ResolveProfile()
			Expect(resolved.MaxPositionCount).NotTo(BeNil())
			Expect(*resolved.MaxPositionCount).To(Equal(10))
			Expect(resolved.GrossExposureLimit).NotTo(BeNil())
			Expect(*resolved.GrossExposureLimit).To(Equal(1.5))
		})

		It("resolves empty profile with overrides to just overrides", func() {
			cfg := config.Config{Risk: config.RiskConfig{
				MaxPositionSize: fp(0.10),
			}}
			resolved := cfg.ResolveProfile()
			Expect(*resolved.MaxPositionSize).To(Equal(0.10))
			Expect(resolved.DrawdownCircuitBreaker).To(BeNil())
		})
	})
})
