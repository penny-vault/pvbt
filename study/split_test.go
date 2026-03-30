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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/study"
)

var _ = Describe("DateRange and Split", func() {
	var (
		jan2020 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		jul2020 = time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)
		dec2020 = time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)
	)

	Describe("TrainTest", func() {
		It("produces a single split with correct ranges", func() {
			splits, err := study.TrainTest(jan2020, jul2020, dec2020)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(1))

			sp := splits[0]
			Expect(sp.FullRange.Start).To(Equal(jan2020))
			Expect(sp.FullRange.End).To(Equal(dec2020))
			Expect(sp.Train.Start).To(Equal(jan2020))
			Expect(sp.Train.End).To(Equal(jul2020))
			Expect(sp.Test.Start).To(Equal(jul2020))
			Expect(sp.Test.End).To(Equal(dec2020))
		})

		It("allows cutoff equal to start (zero-length train)", func() {
			splits, err := study.TrainTest(jan2020, jan2020, dec2020)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(1))
			Expect(splits[0].Train.Start).To(Equal(jan2020))
			Expect(splits[0].Train.End).To(Equal(jan2020))
		})

		It("allows cutoff equal to end (zero-length test)", func() {
			splits, err := study.TrainTest(jan2020, dec2020, dec2020)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(1))
			Expect(splits[0].Test.Start).To(Equal(dec2020))
			Expect(splits[0].Test.End).To(Equal(dec2020))
		})

		It("returns an error when start equals end", func() {
			_, err := study.TrainTest(jan2020, jan2020, jan2020)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when cutoff is before start", func() {
			before := jan2020.Add(-24 * time.Hour)
			_, err := study.TrainTest(jan2020, before, dec2020)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when cutoff is after end", func() {
			after := dec2020.Add(24 * time.Hour)
			_, err := study.TrainTest(jan2020, after, dec2020)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("KFold", func() {
		It("returns the correct number of splits", func() {
			splits, err := study.KFold(jan2020, dec2020, 4)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(4))
		})

		It("each split trains on the full range", func() {
			splits, err := study.KFold(jan2020, dec2020, 3)
			Expect(err).ToNot(HaveOccurred())

			for _, sp := range splits {
				Expect(sp.Train.Start).To(Equal(jan2020))
				Expect(sp.Train.End).To(Equal(dec2020))
				Expect(sp.FullRange.Start).To(Equal(jan2020))
				Expect(sp.FullRange.End).To(Equal(dec2020))
			}
		})

		It("test folds cover the full range without gaps", func() {
			splits, err := study.KFold(jan2020, dec2020, 3)
			Expect(err).ToNot(HaveOccurred())

			// Each split has exactly one Exclude range equal to its Test range.
			for _, sp := range splits {
				Expect(sp.Exclude).To(HaveLen(1))
				Expect(sp.Exclude[0]).To(Equal(sp.Test))
			}

			// First fold starts at start.
			Expect(splits[0].Test.Start).To(Equal(jan2020))
			// Last fold ends at end.
			Expect(splits[len(splits)-1].Test.End).To(Equal(dec2020))

			// Adjacent folds share boundaries.
			for ii := 1; ii < len(splits); ii++ {
				Expect(splits[ii].Test.Start).To(Equal(splits[ii-1].Test.End))
			}
		})

		It("returns an error when folds is less than 2", func() {
			_, err := study.KFold(jan2020, dec2020, 1)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when folds is zero", func() {
			_, err := study.KFold(jan2020, dec2020, 0)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when folds is negative", func() {
			_, err := study.KFold(jan2020, dec2020, -1)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("WalkForward", func() {
		It("produces expanding training windows", func() {
			// 365-day range, 100-day min train, 50-day test, 50-day step.
			end := jan2020.Add(365 * 24 * time.Hour)
			minTrain := 100 * 24 * time.Hour
			testLen := 50 * 24 * time.Hour
			step := 50 * 24 * time.Hour

			splits, err := study.WalkForward(jan2020, end, minTrain, testLen, step)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(splits)).To(BeNumerically(">", 0))

			// Training window grows by step each iteration.
			for ii := 1; ii < len(splits); ii++ {
				prevTrainDur := splits[ii-1].Train.End.Sub(splits[ii-1].Train.Start)
				currTrainDur := splits[ii].Train.End.Sub(splits[ii].Train.Start)
				Expect(currTrainDur - prevTrainDur).To(Equal(step))
			}
		})

		It("train end equals test start for every split", func() {
			end := jan2020.Add(365 * 24 * time.Hour)
			minTrain := 100 * 24 * time.Hour
			testLen := 50 * 24 * time.Hour
			step := 50 * 24 * time.Hour

			splits, err := study.WalkForward(jan2020, end, minTrain, testLen, step)
			Expect(err).ToNot(HaveOccurred())

			for _, sp := range splits {
				Expect(sp.Train.End).To(Equal(sp.Test.Start))
				Expect(sp.FullRange.Start).To(Equal(sp.Train.Start))
				Expect(sp.FullRange.End).To(Equal(sp.Test.End))
			}
		})

		It("returns an error when minTrain+testLen exceeds the date range", func() {
			end := jan2020.Add(100 * 24 * time.Hour)
			minTrain := 80 * 24 * time.Hour
			testLen := 80 * 24 * time.Hour
			step := 10 * 24 * time.Hour

			_, err := study.WalkForward(jan2020, end, minTrain, testLen, step)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ScenarioLeaveNOut", func() {
		var (
			sc1 = study.Scenario{
				Name:  "Scenario A",
				Start: time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2010, 6, 30, 0, 0, 0, 0, time.UTC),
			}
			sc2 = study.Scenario{
				Name:  "Scenario B",
				Start: time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2015, 6, 30, 0, 0, 0, 0, time.UTC),
			}
			sc3 = study.Scenario{
				Name:  "Scenario C",
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2020, 6, 30, 0, 0, 0, 0, time.UTC),
			}
		)

		It("leave-one-out produces n splits", func() {
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2, sc3}, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(3))
		})

		It("leave-one-out test range matches the held-out scenario", func() {
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2, sc3}, 1)
			Expect(err).ToNot(HaveOccurred())

			// First split holds out sc1.
			Expect(splits[0].Test.Start).To(Equal(sc1.Start))
			Expect(splits[0].Test.End).To(Equal(sc1.End))

			// Second split holds out sc2.
			Expect(splits[1].Test.Start).To(Equal(sc2.Start))
			Expect(splits[1].Test.End).To(Equal(sc2.End))
		})

		It("leave-two-out produces C(3,2)=3 splits", func() {
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2, sc3}, 2)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(3))
		})

		It("Exclude contains the held-out ranges", func() {
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2, sc3}, 1)
			Expect(err).ToNot(HaveOccurred())

			// For non-overlapping scenarios, Exclude should contain only the test range.
			for _, sp := range splits {
				Expect(sp.Exclude).To(ContainElement(study.DateRange{Start: sp.Test.Start, End: sp.Test.End}))
			}
		})

		It("FullRange spans all scenarios", func() {
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2, sc3}, 1)
			Expect(err).ToNot(HaveOccurred())

			for _, sp := range splits {
				Expect(sp.FullRange.Start).To(Equal(sc1.Start))
				Expect(sp.FullRange.End).To(Equal(sc3.End))
			}
		})

		It("overlapping non-held-out scenario appears in Exclude", func() {
			// scB overlaps with scA in the held-out set.
			scA := study.Scenario{
				Name:  "A",
				Start: time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2010, 12, 31, 0, 0, 0, 0, time.UTC),
			}
			scB := study.Scenario{
				Name:  "B",
				Start: time.Date(2010, 6, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2011, 3, 31, 0, 0, 0, 0, time.UTC),
			}
			scC := study.Scenario{
				Name:  "C",
				Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC),
			}

			// Hold out scA; scB overlaps with scA so scB should be in Exclude.
			splits, err := study.ScenarioLeaveNOut([]study.Scenario{scA, scB, scC}, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(splits).To(HaveLen(3))

			// First split holds out scA.
			firstSplit := splits[0]
			Expect(firstSplit.Test.Start).To(Equal(scA.Start))

			// scB overlaps scA, so scB's range should appear in Exclude.
			scBRange := study.DateRange{Start: scB.Start, End: scB.End}
			Expect(firstSplit.Exclude).To(ContainElement(scBRange))

			// scC does not overlap scA, so it should NOT be in Exclude beyond the held-out ranges.
			scCRange := study.DateRange{Start: scC.Start, End: scC.End}
			Expect(firstSplit.Exclude).ToNot(ContainElement(scCRange))
		})

		It("returns an error when holdOut is less than 1", func() {
			_, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2}, 0)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when holdOut exceeds number of scenarios", func() {
			_, err := study.ScenarioLeaveNOut([]study.Scenario{sc1, sc2}, 3)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("SubtractRanges", func() {
		var (
			jan = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			mar = time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
			may = time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
			jul = time.Date(2020, 7, 1, 0, 0, 0, 0, time.UTC)
			sep = time.Date(2020, 9, 1, 0, 0, 0, 0, time.UTC)
			dec = time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC)
		)

		It("returns the full window when exclude is empty", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				nil,
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Start).To(Equal(jan))
			Expect(result[0].End).To(Equal(dec))
		})

		It("removes an exclusion from the middle", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: may, End: jul}},
			)
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: may}))
			Expect(result[1]).To(Equal(study.DateRange{Start: jul, End: dec}))
		})

		It("removes an exclusion at the start", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: jan, End: may}},
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(study.DateRange{Start: may, End: dec}))
		})

		It("removes an exclusion at the end", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: sep, End: dec}},
			)
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: sep}))
		})

		It("handles multiple exclusions", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{
					{Start: mar, End: may},
					{Start: jul, End: sep},
				},
			)
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(Equal(study.DateRange{Start: jan, End: mar}))
			Expect(result[1]).To(Equal(study.DateRange{Start: may, End: jul}))
			Expect(result[2]).To(Equal(study.DateRange{Start: sep, End: dec}))
		})

		It("returns empty when exclusion covers the full window", func() {
			result := study.SubtractRanges(
				study.DateRange{Start: jan, End: dec},
				[]study.DateRange{{Start: jan, End: dec}},
			)
			Expect(result).To(BeEmpty())
		})
	})
})
