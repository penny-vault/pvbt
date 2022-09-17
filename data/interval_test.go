// Copyright 2021-2022
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

package data_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pv-api/data"
)

var _ = Describe("Interval tests", func() {
	Describe("When applying interval functions", func() {
		Context("with various date ranges", func() {
			DescribeTable("check adjacency",
				func(a, b *data.Interval, expected bool) {
					Expect(a.Adjacent(b)).To(Equal(expected))
				},

				Entry("When intervals are disjoint (left)", &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are disjoint (right)", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are left adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 2, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals are right adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 9, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 15, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When interval b is a subset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 4, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When interval b is a superset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals partially overlap left", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals partially overlap right", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, false),
			)

			DescribeTable("check containment",
				func(a, b *data.Interval, expected bool) {
					Expect(a.Contains(b)).To(Equal(expected))
				},

				Entry("When intervals are equal", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals are disjoint (left)", &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are disjoint (right)", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are left adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 2, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are right adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 9, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 15, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When interval b is a subset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 4, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When interval b is a superset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals partially overlap left", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals partially overlap right", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, false),
			)

			DescribeTable("check if intervals are contiguous",
				func(a, b *data.Interval, expected bool) {
					Expect(a.Contiguous(b)).To(Equal(expected))
				},

				Entry("When intervals are equal", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are disjoint (left)", &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are disjoint (right)", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are left adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 2, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals are right adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 9, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 15, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When interval b is a subset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 4, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When interval b is a superset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals partially overlap left", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals partially overlap right", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, true),
			)

			DescribeTable("check if intervals overlap",
				func(a, b *data.Interval, expected bool) {
					Expect(a.Overlaps(b)).To(Equal(expected))
				},

				Entry("When intervals are equal", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals are disjoint (left)", &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are disjoint (right)", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2022, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2022, 8, 8, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are left adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 2, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When intervals are right adjacent", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 9, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 15, 0, 0, 0, 0, tz()),
				}, false),

				Entry("When interval b is a subset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 4, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When interval b is a superset of interval a", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals partially overlap left", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 1, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
				}, true),

				Entry("When intervals partially overlap right", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, &data.Interval{
					Begin: time.Date(2021, 8, 6, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 10, 0, 0, 0, 0, tz()),
				}, true),
			)

			DescribeTable("check if interval is valid",
				func(a *data.Interval, valid bool) {
					if valid {
						Expect(a.Valid()).To(BeNil())
					} else {
						Expect(a.Valid()).ToNot(BeNil())
					}
				},

				Entry("Valid interval", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
				}, true),

				Entry("Zero-length interval", &data.Interval{
					Begin: time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
				}, true),

				Entry("Inverted interval, invalid", &data.Interval{
					Begin: time.Date(2021, 8, 8, 0, 0, 0, 0, tz()),
					End:   time.Date(2021, 8, 3, 0, 0, 0, 0, tz()),
				}, false),
			)

		})
	})
})
