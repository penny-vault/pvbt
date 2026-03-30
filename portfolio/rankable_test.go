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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Rankable", func() {
	It("Sharpe implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Sharpe.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Sharpe should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("CAGR implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.CAGR.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "CAGR should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("MaxDrawdown implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.MaxDrawdown.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "MaxDrawdown should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("Sortino implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Sortino.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Sortino should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})

	It("Calmar implements Rankable with HigherIsBetter true", func() {
		rankable, ok := portfolio.Calmar.(portfolio.Rankable)
		Expect(ok).To(BeTrue(), "Calmar should implement Rankable")
		Expect(rankable.HigherIsBetter()).To(BeTrue())
	})
})
