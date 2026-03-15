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

var _ = Describe("RatingFilter", func() {
	Describe("zero value", func() {
		It("matches nothing", func() {
			var f data.RatingFilter
			Expect(f.Matches(1)).To(BeFalse())
			Expect(f.Matches(0)).To(BeFalse())
			Expect(f.Matches(5)).To(BeFalse())
		})
	})

	Describe("RatingEq", func() {
		It("matches the exact value", func() {
			f := data.RatingEq(3)
			Expect(f.Matches(3)).To(BeTrue())
		})

		It("rejects other values", func() {
			f := data.RatingEq(3)
			Expect(f.Matches(2)).To(BeFalse())
			Expect(f.Matches(4)).To(BeFalse())
			Expect(f.Matches(0)).To(BeFalse())
		})
	})

	Describe("RatingIn", func() {
		It("matches any value in the set", func() {
			f := data.RatingIn(1, 3, 5)
			Expect(f.Matches(1)).To(BeTrue())
			Expect(f.Matches(3)).To(BeTrue())
			Expect(f.Matches(5)).To(BeTrue())
		})

		It("rejects values not in the set", func() {
			f := data.RatingIn(1, 3, 5)
			Expect(f.Matches(2)).To(BeFalse())
			Expect(f.Matches(4)).To(BeFalse())
			Expect(f.Matches(0)).To(BeFalse())
		})

		It("with no arguments matches nothing", func() {
			f := data.RatingIn()
			Expect(f.Matches(1)).To(BeFalse())
			Expect(f.Matches(0)).To(BeFalse())
		})
	})

	Describe("RatingLTE", func() {
		It("matches 1 through v inclusive", func() {
			f := data.RatingLTE(3)
			Expect(f.Matches(1)).To(BeTrue())
			Expect(f.Matches(2)).To(BeTrue())
			Expect(f.Matches(3)).To(BeTrue())
		})

		It("rejects 0 and v+1", func() {
			f := data.RatingLTE(3)
			Expect(f.Matches(0)).To(BeFalse())
			Expect(f.Matches(4)).To(BeFalse())
		})

		It("with v=0 matches nothing", func() {
			f := data.RatingLTE(0)
			Expect(f.Matches(0)).To(BeFalse())
			Expect(f.Matches(1)).To(BeFalse())
		})

		It("with negative v matches nothing", func() {
			f := data.RatingLTE(-1)
			Expect(f.Matches(1)).To(BeFalse())
		})
	})
})
