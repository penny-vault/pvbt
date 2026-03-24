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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// fakeOptStudy implements study.Study for search-path testing.
type fakeOptStudy struct {
	configs []study.RunConfig
}

func (fs *fakeOptStudy) Name() string        { return "fake" }
func (fs *fakeOptStudy) Description() string { return "fake study for search tests" }

func (fs *fakeOptStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	return fs.configs, nil
}

func (fs *fakeOptStudy) Analyze(_ []study.RunResult) (report.Report, error) {
	return report.Report{Title: "fake"}, nil
}

// recordingSearch records each call to Next and returns canned batches.
type recordingSearch struct {
	batches [][]study.RunConfig
	calls   int
}

func (rs *recordingSearch) Next(scores []study.CombinationScore) ([]study.RunConfig, bool) {
	if rs.calls >= len(rs.batches) {
		return nil, true
	}

	batch := rs.batches[rs.calls]
	rs.calls++

	done := rs.calls >= len(rs.batches)

	return batch, done
}

var _ = Describe("Runner Search Path", func() {
	Describe("mutual exclusivity", func() {
		It("returns an error when both SearchStrategy and Sweeps are set", func() {
			runner := &study.Runner{
				Study: &fakeOptStudy{
					configs: []study.RunConfig{{Name: "base"}},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				SearchStrategy: study.NewGrid(
					study.SweepValues("lookback", "10"),
				),
				Sweeps: []study.ParamSweep{
					study.SweepValues("lookback", "10"),
				},
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
			Expect(progressCh).To(BeNil())
			Expect(resultCh).To(BeNil())
		})
	})

	Describe("grid search without splits", func() {
		It("executes the correct number of runs", func() {
			runner := &study.Runner{
				Study: &fakeOptStudy{
					configs: []study.RunConfig{
						{
							Name:  "base",
							Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
							End:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
						},
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				SearchStrategy: study.NewGrid(
					study.SweepValues("lookback", "10", "20", "30"),
				),
				Workers: 1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Drain progress.
			var progressEvents []study.Progress
			for ev := range progressCh {
				progressEvents = append(progressEvents, ev)
			}

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())

			// 3 param combos x 1 base config = 3 runs.
			Expect(result.Runs).To(HaveLen(3))

			// Each run should have _combination_id metadata.
			ids := map[string]bool{}
			for _, run := range result.Runs {
				cid, ok := run.Config.Metadata["_combination_id"]
				Expect(ok).To(BeTrue(), "expected _combination_id metadata")
				ids[cid] = true
			}

			Expect(ids).To(HaveLen(3))
		})
	})

	Describe("split expansion", func() {
		It("expands combinations with splits and tags metadata correctly", func() {
			baseStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			baseEnd := time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)
			midPoint := time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)

			splits := []study.Split{
				{
					Name:      "first-half",
					FullRange: study.DateRange{Start: baseStart, End: baseEnd},
					Train:     study.DateRange{Start: baseStart, End: midPoint},
					Test:      study.DateRange{Start: midPoint, End: baseEnd},
				},
				{
					Name:      "second-half",
					FullRange: study.DateRange{Start: baseStart, End: baseEnd},
					Train:     study.DateRange{Start: midPoint, End: baseEnd},
					Test:      study.DateRange{Start: baseStart, End: midPoint},
				},
			}

			runner := &study.Runner{
				Study: &fakeOptStudy{
					configs: []study.RunConfig{
						{Name: "base"},
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				SearchStrategy: study.NewGrid(
					study.SweepValues("lookback", "10", "20"),
				),
				Splits:  splits,
				Workers: 1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			for range progressCh {
			}

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())

			// 2 combos x 2 splits x 1 base = 4 runs.
			Expect(result.Runs).To(HaveLen(4))

			// Verify metadata tagging.
			splitNames := map[string]int{}
			comboIDs := map[string]int{}

			for _, run := range result.Runs {
				splitName, hasSplit := run.Config.Metadata["_split_name"]
				Expect(hasSplit).To(BeTrue(), "expected _split_name metadata")
				splitNames[splitName]++

				cid := run.Config.Metadata["_combination_id"]
				comboIDs[cid]++

				_, hasIndex := run.Config.Metadata["_split_index"]
				Expect(hasIndex).To(BeTrue(), "expected _split_index metadata")
			}

			// Each split name should appear twice (once per combo).
			Expect(splitNames["first-half"]).To(Equal(2))
			Expect(splitNames["second-half"]).To(Equal(2))

			// Each combo ID should appear twice (once per split).
			for _, count := range comboIDs {
				Expect(count).To(Equal(2))
			}
		})
	})

	Describe("progress messages", func() {
		It("includes BatchIndex and BatchSize in progress events", func() {
			runner := &study.Runner{
				Study: &fakeOptStudy{
					configs: []study.RunConfig{
						{
							Name:  "base",
							Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
							End:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
						},
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				SearchStrategy: study.NewGrid(
					study.SweepValues("lookback", "10", "20"),
				),
				Workers: 1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			var progressEvents []study.Progress
			for ev := range progressCh {
				progressEvents = append(progressEvents, ev)
			}

			<-resultCh

			// Grid returns everything in one batch, so BatchIndex=0 for all.
			Expect(progressEvents).NotTo(BeEmpty())

			for _, ev := range progressEvents {
				Expect(ev.BatchIndex).To(Equal(0))
				Expect(ev.BatchSize).To(Equal(2))
			}
		})
	})

	Describe("multi-batch search", func() {
		It("calls Next repeatedly until done", func() {
			search := &recordingSearch{
				batches: [][]study.RunConfig{
					{
						{Params: map[string]string{"lookback": "10"}},
					},
					{
						{Params: map[string]string{"lookback": "20"}},
					},
				},
			}

			runner := &study.Runner{
				Study: &fakeOptStudy{
					configs: []study.RunConfig{
						{
							Name:  "base",
							Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
							End:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
						},
					},
				},
				NewStrategy:    func() engine.Strategy { return &mockStrategy{} },
				SearchStrategy: search,
				Workers:        1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			for range progressCh {
			}

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())

			// 2 batches x 1 combo each x 1 base = 2 total runs.
			Expect(result.Runs).To(HaveLen(2))

			// Search strategy should have been called twice (plus the final done check is implicit).
			Expect(search.calls).To(Equal(2))
		})
	})

	Describe("existing sweep path unchanged", func() {
		It("still works with Sweeps and no SearchStrategy", func() {
			var analyzedCount int

			runner := &study.Runner{
				Study: &mockStudy{
					configs: []study.RunConfig{
						{
							Name:  "base",
							Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
							End:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
						},
					},
					analyzeFn: func(results []study.RunResult) (report.Report, error) {
						analyzedCount = len(results)
						return report.Report{Title: "Sweep"}, nil
					},
				},
				NewStrategy: func() engine.Strategy { return &mockStrategy{} },
				Sweeps: []study.ParamSweep{
					study.SweepValues("lookback", "10", "20"),
				},
				Workers: 1,
			}

			progressCh, resultCh, err := runner.Run(context.Background())
			Expect(err).NotTo(HaveOccurred())

			for range progressCh {
			}

			result := <-resultCh
			Expect(result.Err).NotTo(HaveOccurred())
			Expect(analyzedCount).To(Equal(2))
		})
	})
})
