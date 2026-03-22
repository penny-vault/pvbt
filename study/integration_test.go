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
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/stress"
)

// integrationMockStudy is a minimal study.Study implementation for
// integration testing the full pipeline without requiring real data providers.
type integrationMockStudy struct{}

func (mock *integrationMockStudy) Name() string        { return "Integration Mock" }
func (mock *integrationMockStudy) Description() string { return "mock study for integration testing" }

func (mock *integrationMockStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	// Return zero configs so the runner calls Analyze directly without
	// attempting to run any engine (which requires database connections).
	return []study.RunConfig{}, nil
}

func (mock *integrationMockStudy) Analyze(_ []study.RunResult) (report.Report, error) {
	return report.Report{
		Title: "Integration Test Results",
		Sections: []report.Section{
			&report.Text{
				SectionName: "Summary",
				Body:        "Integration test completed successfully with zero engine runs.",
			},
		},
	}, nil
}

var _ = Describe("Integration", func() {
	It("stress test satisfies the Study interface", func() {
		var iface study.Study = stress.New(nil)
		Expect(iface.Name()).To(Equal("Stress Test"))
	})

	It("stress test Configurations returns valid configs", func() {
		stressStudy := stress.New(nil)
		configs, err := stressStudy.Configurations(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Start).NotTo(BeZero())
		Expect(configs[0].End).NotTo(BeZero())
		Expect(configs[0].End.After(configs[0].Start)).To(BeTrue())
	})

	It("Runner with mock study produces a valid report", func() {
		runner := &study.Runner{
			Study:   &integrationMockStudy{},
			Workers: 2,
		}

		progressCh, resultCh, err := runner.Run(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Drain progress channel; with zero configs there should be no updates.
		var progressCount int
		for range progressCh {
			progressCount++
		}

		result := <-resultCh
		Expect(result.Err).NotTo(HaveOccurred())
		Expect(result.Report.Title).NotTo(BeEmpty())

		// Render in text format.
		var textBuf bytes.Buffer
		Expect(result.Report.Render(report.FormatText, &textBuf)).To(Succeed())
		Expect(textBuf.Len()).To(BeNumerically(">", 0))

		// Render in JSON format.
		var jsonBuf bytes.Buffer
		Expect(result.Report.Render(report.FormatJSON, &jsonBuf)).To(Succeed())
		Expect(jsonBuf.String()).To(ContainSubstring(`"title"`))
	})

	It("Runner field accepts stress.New(nil) as a Study", func() {
		// This test verifies the full type chain compiles: Runner{Study: stress.New(nil)}.
		// stress.New returns *stress.StressTest which implements study.Study.
		runner := &study.Runner{
			Study:   stress.New(nil),
			Workers: 1,
		}
		Expect(runner.Study.Name()).To(Equal("Stress Test"))
	})
})
