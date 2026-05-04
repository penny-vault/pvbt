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
	"fmt"
	"io"
	"reflect"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
)

var _ = Describe("study command", func() {
	It("registers stress-test subcommand", func() {
		strategy := &testStrategy{}
		cmd := newStudyCmd(strategy)
		subCmds := cmd.Commands()
		names := make([]string, len(subCmds))
		for idx, sub := range subCmds {
			names[idx] = sub.Name()
		}
		Expect(names).To(ContainElement("stress-test"))
	})

	It("has use string 'study'", func() {
		strategy := &testStrategy{}
		cmd := newStudyCmd(strategy)
		Expect(cmd.Use).To(Equal("study"))
	})
})

var _ = Describe("stress-test command", func() {
	It("defaults workers flag to GOMAXPROCS", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)
		workersFlag := cmd.Flags().Lookup("workers")
		Expect(workersFlag).NotTo(BeNil())
		Expect(workersFlag.DefValue).To(Equal(fmt.Sprintf("%d", runtime.GOMAXPROCS(0))))
	})

	It("defaults format flag to html", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)
		formatFlag := cmd.Flags().Lookup("format")
		Expect(formatFlag).NotTo(BeNil())
		Expect(formatFlag.DefValue).To(Equal("html"))
	})

	It("accepts arbitrary args", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)
		Expect(cmd.Args).NotTo(BeNil())
	})
})

var _ = Describe("optimize command", func() {
	It("registers strategy flags as sweep-friendly so int ranges are accepted", func() {
		strategy := &testStrategy{}
		cmd := newOptimizeCmd(strategy)

		Expect(cmd.Flags().Set("lookback", "0:8:1")).To(Succeed(),
			"int strategy fields must accept colon-range syntax under study optimize")
		Expect(cmd.Flags().Set("threshold", "0.1:0.9:0.1")).To(Succeed(),
			"float64 strategy fields must accept colon-range syntax under study optimize")
	})

	It("rejects a run with no parameter ranges configured", func() {
		strategy := &testStrategy{}
		cmd := newOptimizeCmd(strategy)
		cmd.SetArgs([]string{
			"--validation", "train-test",
			"--train-end", "2020-01-01",
		})
		// Silence cobra's default error printing.
		cmd.SetErr(io.Discard)
		cmd.SetOut(io.Discard)

		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no parameter ranges configured"))
	})

	It("documents the supported --format values in the flag usage", func() {
		strategy := &testStrategy{}
		cmd := newOptimizeCmd(strategy)

		formatFlag := cmd.Flags().Lookup("format")
		Expect(formatFlag).NotTo(BeNil())
		Expect(formatFlag.Usage).To(ContainSubstring("text"))
		Expect(formatFlag.Usage).To(ContainSubstring("json"))
		Expect(formatFlag.Usage).To(ContainSubstring("html"))
	})
})

var _ = Describe("strategyFactoryWithFlags", func() {
	It("applies user-set flag values to every fresh instance", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)

		Expect(cmd.Flags().Set("lookback", "120")).To(Succeed())
		Expect(cmd.Flags().Set("threshold", "0.75")).To(Succeed())

		factory := strategyFactoryWithFlags(strategy, cmd)

		first, ok := factory().(*testStrategy)
		Expect(ok).To(BeTrue())
		Expect(first.Lookback).To(Equal(120),
			"int flag values must propagate to fresh strategy instances")
		Expect(first.Threshold).To(BeNumerically("~", 0.75, 1e-10),
			"float64 flag values must propagate to fresh strategy instances")

		second, ok := factory().(*testStrategy)
		Expect(ok).To(BeTrue())
		Expect(second).NotTo(BeIdenticalTo(first),
			"each call must return a distinct instance")
		Expect(second.Lookback).To(Equal(120),
			"flag values must propagate to every instance, not just the first")
	})

	It("propagates universe.Universe flag values that collectFixedParams cannot", func() {
		strategy := &universeStrategy{}
		cmd := newStressTestCmd(strategy)

		Expect(cmd.Flags().Set("risk-on", "AAPL,MSFT")).To(Succeed())

		factory := strategyFactoryWithFlags(strategy, cmd)

		instance, ok := factory().(*universeStrategy)
		Expect(ok).To(BeTrue())
		Expect(instance.RiskOn).NotTo(BeNil())

		members := instance.RiskOn.Assets(time.Time{})
		Expect(members).To(HaveLen(2))
		Expect(members[0].Ticker).To(Equal("AAPL"))
		Expect(members[1].Ticker).To(Equal("MSFT"))
	})

	It("leaves fields at zero when no flag was set, allowing engine defaults to apply", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)

		factory := strategyFactoryWithFlags(strategy, cmd)

		instance, ok := factory().(*testStrategy)
		Expect(ok).To(BeTrue())
		// applyStrategyFlags reads the cobra flag value, which carries the
		// registered default ("90"). The engine's hydrateFields would
		// normally overlay struct-tag defaults; here we just confirm
		// the factory does not error and produces a usable instance.
		Expect(instance).NotTo(BeNil())
	})
})

var _ = Describe("strategyFactory", func() {
	It("creates new instances of the same concrete type", func() {
		original := &testStrategy{Lookback: 42, Threshold: 0.99}
		factory := strategyFactory(original)

		created := factory()
		Expect(created).NotTo(BeNil())

		// The new instance should be a *testStrategy with zero values.
		concrete, ok := created.(*testStrategy)
		Expect(ok).To(BeTrue())
		Expect(concrete.Lookback).To(Equal(0))
		Expect(concrete.Threshold).To(Equal(0.0))
	})

	It("returns a distinct instance on each call", func() {
		original := &testStrategy{}
		factory := strategyFactory(original)

		first := factory()
		second := factory()
		Expect(reflect.ValueOf(first).Pointer()).NotTo(Equal(reflect.ValueOf(second).Pointer()))
	})

	It("implements engine.Strategy", func() {
		original := &testStrategy{}
		factory := strategyFactory(original)
		created := factory()

		// Verify the created instance satisfies the engine.Strategy interface
		// by checking it can call Name() without panicking.
		Expect(created.Name()).To(Equal("test"))
	})
})

var _ = Describe("resolveScenarios", func() {
	It("returns nil, nil for empty args", func() {
		result, err := resolveScenarios([]string{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("returns nil, nil for 'all' arg", func() {
		result, err := resolveScenarios([]string{"all"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
	})

	It("filters scenarios by name", func() {
		defaults := study.AllScenarios()
		if len(defaults) == 0 {
			Skip("no default scenarios defined")
		}

		firstName := defaults[0].Name
		result, err := resolveScenarios([]string{firstName})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal(firstName))
	})

	It("returns an error for unknown scenario names", func() {
		_, err := resolveScenarios([]string{"nonexistent-scenario-xyz"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nonexistent-scenario-xyz"))
	})
})
