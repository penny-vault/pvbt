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

package summary

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("formatDollar", func() {
	It("formats positive amounts with commas and cents", func() {
		Expect(formatDollar(1234.56)).To(Equal("$1,234.56"))
	})

	It("formats negative amounts with a leading minus", func() {
		Expect(formatDollar(-1234.56)).To(Equal("$-1,234.56"))
	})

	It("renders NaN as $NaN instead of garbage", func() {
		Expect(formatDollar(math.NaN())).To(Equal("$NaN"))
	})

	It("renders +Inf as $Inf instead of garbage", func() {
		Expect(formatDollar(math.Inf(1))).To(Equal("$Inf"))
	})

	It("renders -Inf as $-Inf instead of garbage", func() {
		Expect(formatDollar(math.Inf(-1))).To(Equal("$-Inf"))
	})

	It("renders out-of-range positive values as $Inf", func() {
		Expect(formatDollar(1e30)).To(Equal("$Inf"))
	})

	It("renders out-of-range negative values as $-Inf", func() {
		Expect(formatDollar(-1e30)).To(Equal("$-Inf"))
	})
})
