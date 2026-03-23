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

package study_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
)

var _ = Describe("AllScenarios", func() {
	It("returns exactly 17 named scenarios", func() {
		scenarios := study.AllScenarios()
		Expect(scenarios).To(HaveLen(17))
	})

	It("includes all expected scenario names", func() {
		scenarios := study.AllScenarios()

		names := make([]string, len(scenarios))
		for idx, scenario := range scenarios {
			names[idx] = scenario.Name
		}

		Expect(names).To(ContainElements(
			"1973-74 Oil Embargo Bear Market",
			"Volcker Tightening",
			"1987 Black Monday",
			"1994 Bond Massacre",
			"1998 LTCM / Russian Crisis",
			"Dot-com Bubble",
			"Dot-com Bust",
			"9/11",
			"2008 Financial Crisis",
			"2010 Flash Crash",
			"Euro Debt Crisis",
			"2011 Debt Ceiling Crisis",
			"2015-2017 Low-Volatility Grind",
			"2018 Q4 Selloff",
			"COVID Crash",
			"2022 Rate Hiking Cycle",
			"2023 Regional Banking Crisis",
		))
	})

	It("gives every scenario a non-empty description", func() {
		for _, scenario := range study.AllScenarios() {
			Expect(scenario.Description).NotTo(BeEmpty(), "scenario %q has empty description", scenario.Name)
		}
	})

	It("gives every scenario a start before its end", func() {
		for _, scenario := range study.AllScenarios() {
			Expect(scenario.Start.Before(scenario.End)).To(BeTrue(),
				"scenario %q: start %v is not before end %v", scenario.Name, scenario.Start, scenario.End)
		}
	})
})

var _ = Describe("ScenariosByName", func() {
	It("returns the matching scenarios in the requested order", func() {
		scenarios, err := study.ScenariosByName([]string{"COVID Crash", "2008 Financial Crisis"})
		Expect(err).NotTo(HaveOccurred())
		Expect(scenarios).To(HaveLen(2))
		Expect(scenarios[0].Name).To(Equal("COVID Crash"))
		Expect(scenarios[1].Name).To(Equal("2008 Financial Crisis"))
	})

	It("returns a single matching scenario", func() {
		scenarios, err := study.ScenariosByName([]string{"9/11"})
		Expect(err).NotTo(HaveOccurred())
		Expect(scenarios).To(HaveLen(1))
		Expect(scenarios[0].Name).To(Equal("9/11"))
	})

	It("returns an error for an unknown scenario name", func() {
		_, err := study.ScenariosByName([]string{"COVID Crash", "Not A Real Scenario"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Not A Real Scenario"))
	})

	It("returns an empty slice for an empty names list", func() {
		scenarios, err := study.ScenariosByName([]string{})
		Expect(err).NotTo(HaveOccurred())
		Expect(scenarios).To(BeEmpty())
	})
})
