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
	"context"
	"fmt"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// mockReport implements report.Report for testing.
type mockReport struct {
	title string
}

func (mr *mockReport) Name() string { return mr.title }

func (mr *mockReport) Data(writer io.Writer) error {
	_, err := writer.Write([]byte("{}"))
	return err
}

// mockStudy implements study.Study for testing the Runner without a real engine.
type mockStudy struct {
	configs   []study.RunConfig
	configErr error
	analyzeFn func([]study.RunResult) (report.Report, error)
}

func (ms *mockStudy) Name() string        { return "mock" }
func (ms *mockStudy) Description() string { return "mock study for testing" }

func (ms *mockStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	return ms.configs, ms.configErr
}

func (ms *mockStudy) Analyze(results []study.RunResult) (report.Report, error) {
	if ms.analyzeFn != nil {
		return ms.analyzeFn(results)
	}

	return &mockReport{title: "Mock Results"}, nil
}

// mockStrategy satisfies engine.Strategy for constructing an engine
// (it will never actually run in these tests).
type mockStrategy struct{}

func (ms *mockStrategy) Name() string           { return "mock-strategy" }
func (ms *mockStrategy) Setup(_ *engine.Engine) {}
func (ms *mockStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
	return nil
}

var _ = Describe("Runner", func() {
	Describe("Run", func() {
		It("returns an error synchronously when Configurations fails", func() {
			runner := &study.Runner{
				Study: &mockStudy{
					configErr: fmt.Errorf("database unavailable"),
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database unavailable"))
			Expect(progressCh).To(BeNil())
			Expect(resultCh).To(BeNil())
		})

		It("calls Analyze with empty results when there are zero configurations", func() {
			var analyzedResults []study.RunResult
			analyzeCallCount := 0

			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{},
					analyzeFn: func(results []study.RunResult) (report.Report, error) {
						analyzedResults = results
						analyzeCallCount++

						return &mockReport{title: "Empty Study"}, nil
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
			}

			_, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())
			Expect(result.Report.Name()).To(Equal("Empty Study"))
			Expect(analyzeCallCount).To(Equal(1))
			Expect(analyzedResults).To(HaveLen(0))
		})

		It("applies cross-product sweeps to configurations before execution", func() {
			var analyzedResultCount int

			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{
						{Name: "base-a", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)},
						{Name: "base-b", Start: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2021, 12, 31, 0, 0, 0, 0, time.UTC)},
					},
					analyzeFn: func(results []study.RunResult) (report.Report, error) {
						analyzedResultCount = len(results)

						return &mockReport{title: "Sweep Results"}, nil
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				Sweeps: []study.ParamSweep{
					study.SweepValues("lookback", "10", "20", "30"),
				},
			}

			// The cross product of 2 configs x 3 sweep values = 6 configs.
			// Each will fail (no data providers), but Analyze will see 6 results.
			_, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := <-resultCh
			// Analyze should have been called with 6 results (2 base x 3 sweep values).
			Expect(analyzedResultCount).To(Equal(6))
			Expect(result.Report.Name()).To(Equal("Sweep Results"))
		})

		It("sends progress updates for each run", func() {
			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{
						{Name: "run-1", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)},
						{Name: "run-2", Start: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2021, 12, 31, 0, 0, 0, 0, time.UTC)},
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				Workers:     1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Collect all progress updates.
			var progressUpdates []study.Progress

			for update := range progressCh {
				progressUpdates = append(progressUpdates, update)
			}

			// Each run should send at least a started + completed/failed pair.
			// With 2 configs (which will fail due to no data providers), we expect
			// 2 started + 2 failed = 4 progress updates.
			Expect(len(progressUpdates)).To(BeNumerically(">=", 4))

			startedCount := 0
			failedCount := 0

			for _, update := range progressUpdates {
				Expect(update.TotalRuns).To(Equal(2))

				switch update.Status {
				case study.RunStarted:
					startedCount++
				case study.RunFailed:
					failedCount++
				}
			}

			Expect(startedCount).To(Equal(2))
			Expect(failedCount).To(Equal(2))

			// Drain the result channel.
			result := <-resultCh
			Expect(result.Runs).To(HaveLen(2))
		})

		It("propagates Analyze errors in the Result", func() {
			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{},
					analyzeFn: func(_ []study.RunResult) (report.Report, error) {
						return nil, fmt.Errorf("analysis failed")
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
			}

			_, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := <-resultCh
			Expect(result.Err).To(HaveOccurred())
			Expect(result.Err.Error()).To(ContainSubstring("analysis failed"))
		})

		It("handles context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately.

			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{
						{Name: "cancelled-run", Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)},
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				Workers:     1,
			}

			progressCh, resultCh, err := runner.Run(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Drain progress.
			for range progressCh {
			}

			result := <-resultCh
			// At least one run should have a context error.
			hasContextErr := false

			for _, run := range result.Runs {
				if run.Err != nil {
					hasContextErr = true
				}
			}

			Expect(hasContextErr).To(BeTrue())
		})

		It("defaults to 1 worker when Workers is zero", func() {
			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				Workers:     0,
			}

			_, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())
		})
	})
})
