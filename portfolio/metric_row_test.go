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

package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MetricRow", func() {
	var acct *portfolio.Account

	BeforeEach(func() {
		acct = portfolio.New()
	})

	It("starts with empty metrics", func() {
		Expect(acct.Metrics()).To(BeEmpty())
	})

	It("appends metric rows", func() {
		row := portfolio.MetricRow{
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Name:   "sharpe",
			Window: "1yr",
			Value:  1.5,
		}
		acct.AppendMetric(row)
		Expect(acct.Metrics()).To(HaveLen(1))
		Expect(acct.Metrics()[0]).To(Equal(row))
	})
})
