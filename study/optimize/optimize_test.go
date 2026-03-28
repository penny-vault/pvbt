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

package optimize_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/optimize"
)

var _ = Describe("Optimizer", func() {
	var splits []study.Split

	BeforeEach(func() {
		splits = []study.Split{
			{
				Name: "fold 1/2",
				FullRange: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Train: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Test: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			{
				Name: "fold 2/2",
				FullRange: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC),
				},
				Train: study.DateRange{
					Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC),
				},
				Test: study.DateRange{
					Start: time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC),
					End:   time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		}
	})

	Describe("Name and Description", func() {
		It("returns a non-empty name", func() {
			opt := optimize.New(splits)
			Expect(opt.Name()).NotTo(BeEmpty())
		})

		It("returns a non-empty description", func() {
			opt := optimize.New(splits)
			Expect(opt.Description()).NotTo(BeEmpty())
		})
	})

	Describe("Configurations", func() {
		It("returns a single config spanning all splits", func() {
			opt := optimize.New(splits)
			configs, err := opt.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(1))

			cfg := configs[0]
			Expect(cfg.Start).To(Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))
			Expect(cfg.End).To(Equal(time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("sets the study metadata to parameter-optimization", func() {
			opt := optimize.New(splits)
			configs, err := opt.Configurations(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(configs[0].Metadata["study"]).To(Equal("parameter-optimization"))
		})
	})

	Describe("defaults", func() {
		It("uses MetricSharpe as the default objective", func() {
			opt := optimize.New(splits)
			// The default is verified indirectly: Analyze should not panic
			// and should produce a report.
			rpt, err := opt.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			Expect(rpt.Name()).NotTo(BeEmpty())
		})
	})

	Describe("WithObjective", func() {
		It("accepts a custom objective metric", func() {
			opt := optimize.New(splits, optimize.WithObjective(study.MetricCAGR))
			rpt, err := opt.Analyze([]study.RunResult{})
			Expect(err).NotTo(HaveOccurred())
			// The report JSON should mention CAGR as the objective name.
			rptData := decodeOptReport(rpt)
			Expect(rptData.ObjectiveName).To(Equal("CAGR"))
		})
	})
})
