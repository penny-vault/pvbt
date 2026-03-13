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

var _ = Describe("Stats", func() {
	Describe("AnnualizationFactor", func() {
		It("returns 252 for daily timestamps", func() {
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 10)
			for i := range times {
				times[i] = start.AddDate(0, 0, i)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("returns 12 for monthly timestamps", func() {
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := make([]time.Time, 6)
			for i := range times {
				times[i] = start.AddDate(0, i, 0)
			}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 12))
		})

		It("defaults to 252 for a single timestamp", func() {
			times := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
			Expect(data.AnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("defaults to 252 for empty slice", func() {
			Expect(data.AnnualizationFactor(nil)).To(BeNumerically("==", 252))
		})

		It("returns 252 for gap of exactly 20 days (boundary)", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 21, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 252))
		})

		It("returns 12 for gap of 21 days", func() {
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
			Expect(data.AnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 12))
		})
	})
})
