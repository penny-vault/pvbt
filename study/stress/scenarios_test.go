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

package stress_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/stress"
)

var _ = Describe("DefaultScenarios", func() {
	It("returns exactly 17 named scenarios", func() {
		scenarios := stress.DefaultScenarios()
		Expect(scenarios).To(HaveLen(17))
	})

	It("includes all expected scenario names", func() {
		scenarios := stress.DefaultScenarios()

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
		for _, scenario := range stress.DefaultScenarios() {
			Expect(scenario.Description).NotTo(BeEmpty(), "scenario %q has empty description", scenario.Name)
		}
	})

	It("gives every scenario a start before its end", func() {
		for _, scenario := range stress.DefaultScenarios() {
			Expect(scenario.Start.Before(scenario.End)).To(BeTrue(),
				"scenario %q: start %v is not before end %v", scenario.Name, scenario.Start, scenario.End)
		}
	})
})

var _ = Describe("New", func() {
	It("uses DefaultScenarios when nil is passed", func() {
		stressTest := stress.New(nil)
		configs, err := stressTest.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))

		defaults := stress.DefaultScenarios()
		expectedEarliest := defaults[0].Start

		for _, scenario := range defaults[1:] {
			if scenario.Start.Before(expectedEarliest) {
				expectedEarliest = scenario.Start
			}
		}

		Expect(configs[0].Start).To(Equal(expectedEarliest))
	})

	It("uses DefaultScenarios when empty slice is passed", func() {
		stressTest := stress.New([]stress.Scenario{})
		configs, err := stressTest.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))
	})

	It("uses custom scenarios when provided", func() {
		customStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		customEnd := time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC)

		stressTest := stress.New([]stress.Scenario{
			{Name: "Custom", Description: "Test", Start: customStart, End: customEnd},
		})

		configs, err := stressTest.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Start).To(Equal(customStart))
		Expect(configs[0].End).To(Equal(customEnd))
	})

	It("returns a StressTest that satisfies the study.Study interface", func() {
		var studyInterface study.Study = stress.New(nil)
		Expect(studyInterface).NotTo(BeNil())
	})

	It("exposes the Name method", func() {
		stressTest := stress.New(nil)
		Expect(stressTest.Name()).To(Equal("Stress Test"))
	})

	It("exposes the Description method", func() {
		stressTest := stress.New(nil)
		Expect(stressTest.Description()).NotTo(BeEmpty())
	})
})

var _ = Describe("Configurations", func() {
	It("returns a single config whose range spans all scenario windows", func() {
		scenarios := []stress.Scenario{
			{
				Name:  "Early",
				Start: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				Name:  "Middle",
				Start: time.Date(2008, 6, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2009, 6, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				Name:  "Late",
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC),
			},
		}

		stressTest := stress.New(scenarios)
		configs, err := stressTest.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))

		cfg := configs[0]
		Expect(cfg.Start).To(Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)))
		Expect(cfg.End).To(Equal(time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)))
		Expect(cfg.Name).To(Equal("Full Range"))
	})

	It("sets the study metadata key on the config", func() {
		stressTest := stress.New(nil)
		configs, err := stressTest.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs[0].Metadata).To(HaveKeyWithValue("study", "stress-test"))
	})
})
