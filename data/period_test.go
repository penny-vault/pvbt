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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("Period", func() {
	ref := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	Describe("Before", func() {
		It("subtracts days", func() {
			p := data.Days(10)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts months", func() {
			p := data.Months(2)
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)))
		})

		It("subtracts years", func() {
			p := data.Years(1)
			Expect(p.Before(ref)).To(Equal(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)))
		})

		It("returns Jan 1 for YTD", func() {
			p := data.YTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns 1st of month for MTD", func() {
			p := data.MTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("returns most recent Monday for WTD", func() {
			p := data.WTD()
			Expect(p.Before(ref)).To(Equal(time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)))
		})

		It("returns ref for WTD when ref is Monday", func() {
			monday := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
			p := data.WTD()
			Expect(p.Before(monday)).To(Equal(monday))
		})
	})
})
