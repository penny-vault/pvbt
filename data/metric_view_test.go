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

var _ = Describe("IsFundamental", func() {
	It("returns true for fundamental metrics", func() {
		Expect(data.IsFundamental(data.Revenue)).To(BeTrue())
		Expect(data.IsFundamental(data.WorkingCapital)).To(BeTrue())
		Expect(data.IsFundamental(data.NetIncome)).To(BeTrue())
	})

	It("returns false for eod metrics", func() {
		Expect(data.IsFundamental(data.MetricClose)).To(BeFalse())
		Expect(data.IsFundamental(data.Volume)).To(BeFalse())
	})

	It("returns false for derived metrics", func() {
		Expect(data.IsFundamental(data.MarketCap)).To(BeFalse())
		Expect(data.IsFundamental(data.PE)).To(BeFalse())
	})

	It("returns false for unknown metrics", func() {
		Expect(data.IsFundamental(data.Metric("nonexistent"))).To(BeFalse())
	})
})
