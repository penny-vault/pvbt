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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

var _ = Describe("Config loading", func() {
	Describe("loadMiddlewareConfig", func() {
		It("reads a TOML file with explicit path and produces a valid config", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[risk]\nprofile = \"moderate\"\n"), 0o644)).To(Succeed())

			cfg, err := loadMiddlewareConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Risk.Profile).To(Equal("moderate"))
		})

		It("returns error when file does not exist", func() {
			cfg, err := loadMiddlewareConfig("/nonexistent/path/to/pvbt.toml")
			Expect(err).To(HaveOccurred())
			_ = cfg
		})

		It("returns error for malformed TOML", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "bad.toml")
			Expect(os.WriteFile(path, []byte("[[[\n"), 0o644)).To(Succeed())

			cfg, err := loadMiddlewareConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})

		It("pointer fields are nil when not set in TOML", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "test.toml")
			Expect(os.WriteFile(path, []byte("[risk]\nprofile = \"none\"\n"), 0o644)).To(Succeed())

			cfg, err := loadMiddlewareConfig(path)
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

			cfg, err := loadMiddlewareConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Risk.MaxPositionSize).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionSize).To(Equal(0.0))
			Expect(cfg.Risk.MaxPositionCount).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionCount).To(Equal(0))
		})
	})

	Describe("loadMiddlewareConfigFromCommand", func() {
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

			cfg, err := loadMiddlewareConfigFromCommand(cmd)
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

			cfg, err := loadMiddlewareConfigFromCommand(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Tax.Enabled).To(BeTrue())
		})

		It("re-validates after overrides and returns error for invalid --risk-profile", func() {
			cmd := newCmd()
			Expect(cmd.Flags().Set("risk-profile", "invalid")).To(Succeed())

			_, err := loadMiddlewareConfigFromCommand(cmd)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("unknown profile")))
		})

		It("returns error when config path does not exist", func() {
			cmd := newCmd()
			Expect(cmd.Flags().Set("config", "/nonexistent/pvbt.toml")).To(Succeed())

			_, err := loadMiddlewareConfigFromCommand(cmd)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("End-to-end loading from testdata/full.toml", func() {
		It("loads the full config fixture correctly", func() {
			cfg, err := loadMiddlewareConfig("testdata/full.toml")
			Expect(err).NotTo(HaveOccurred())

			// Risk
			Expect(cfg.Risk.Profile).To(Equal("moderate"))
			Expect(cfg.Risk.MaxPositionSize).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionSize).To(Equal(0.15))
			Expect(cfg.Risk.MaxPositionCount).NotTo(BeNil())
			Expect(*cfg.Risk.MaxPositionCount).To(Equal(20))
			Expect(cfg.Risk.DrawdownCircuitBreaker).NotTo(BeNil())
			Expect(*cfg.Risk.DrawdownCircuitBreaker).To(Equal(0.12))
			Expect(cfg.Risk.VolatilityScalerLookback).NotTo(BeNil())
			Expect(*cfg.Risk.VolatilityScalerLookback).To(Equal(60))
			Expect(cfg.Risk.GrossExposureLimit).NotTo(BeNil())
			Expect(*cfg.Risk.GrossExposureLimit).To(Equal(1.5))
			Expect(cfg.Risk.NetExposureLimit).NotTo(BeNil())
			Expect(*cfg.Risk.NetExposureLimit).To(Equal(1.0))

			// Tax
			Expect(cfg.Tax.Enabled).To(BeTrue())
			Expect(cfg.Tax.LossThreshold).To(Equal(0.05))
			Expect(cfg.Tax.GainOffsetOnly).To(BeFalse())
			// Viper lowercases TOML keys, so substitutes keys are lowercase.
			Expect(cfg.Tax.Substitutes).To(HaveLen(3))
			Expect(cfg.Tax.Substitutes["spy"]).To(Equal("VOO"))
			Expect(cfg.Tax.Substitutes["qqq"]).To(Equal("QQQM"))
			Expect(cfg.Tax.Substitutes["iwm"]).To(Equal("VTWO"))

			// Resolve should apply overrides
			resolved := cfg.Risk.Resolve()
			Expect(*resolved.MaxPositionSize).To(Equal(0.15))
			Expect(*resolved.DrawdownCircuitBreaker).To(Equal(0.12))
			Expect(*resolved.MaxPositionCount).To(Equal(20))

			Expect(cfg.HasMiddleware()).To(BeTrue())
		})
	})
})
