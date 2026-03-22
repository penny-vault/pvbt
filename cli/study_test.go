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
	"reflect"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study/stress"
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

	It("defaults format flag to text", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)
		formatFlag := cmd.Flags().Lookup("format")
		Expect(formatFlag).NotTo(BeNil())
		Expect(formatFlag.DefValue).To(Equal("text"))
	})

	It("accepts arbitrary args", func() {
		strategy := &testStrategy{}
		cmd := newStressTestCmd(strategy)
		Expect(cmd.Args).NotTo(BeNil())
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
	It("returns nil for empty args", func() {
		result := resolveScenarios([]string{})
		Expect(result).To(BeNil())
	})

	It("returns nil for 'all' arg", func() {
		result := resolveScenarios([]string{"all"})
		Expect(result).To(BeNil())
	})

	It("filters scenarios by name", func() {
		defaults := stress.DefaultScenarios()
		if len(defaults) == 0 {
			Skip("no default scenarios defined")
		}

		firstName := defaults[0].Name
		result := resolveScenarios([]string{firstName})
		Expect(result).To(HaveLen(1))
		Expect(result[0].Name).To(Equal(firstName))
	})

	It("ignores unknown scenario names", func() {
		result := resolveScenarios([]string{"nonexistent-scenario-xyz"})
		Expect(result).To(BeEmpty())
	})
})
