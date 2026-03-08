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

package data_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("MetricRegistry", func() {
	Describe("MetricByName", func() {
		It("returns AdjClose for the name AdjClose", func() {
			m, ok := data.MetricByName("AdjClose")
			Expect(ok).To(BeTrue())
			Expect(m).To(Equal(data.AdjClose))
		})

		It("returns MetricClose for the name MetricClose", func() {
			m, ok := data.MetricByName("MetricClose")
			Expect(ok).To(BeTrue())
			Expect(m).To(Equal(data.MetricClose))
		})

		It("returns false for an unknown name", func() {
			_, ok := data.MetricByName("NotAMetric")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("AllMetricNames", func() {
		It("returns a non-empty sorted list containing expected entries", func() {
			names := data.AllMetricNames()
			Expect(names).NotTo(BeEmpty())
			Expect(names).To(ContainElements("AdjClose", "Volume", "PE", "Revenue"))

			// verify sorted order
			for i := 1; i < len(names); i++ {
				Expect(names[i] > names[i-1]).To(BeTrue(),
					"expected %q > %q", names[i], names[i-1])
			}
		})
	})
})
