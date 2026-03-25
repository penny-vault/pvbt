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

package risk_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Profiles", func() {
	Describe("Conservative", func() {
		It("returns 3 middleware", func() {
			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{},
				currentDate:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			}

			chain := risk.Conservative(ds)
			Expect(chain).To(HaveLen(3))
		})

		It("accepts a DataSource parameter", func() {
			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{},
				currentDate:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			}

			chain := risk.Conservative(ds)
			Expect(chain).NotTo(BeNil())
		})

		It("returns a valid []portfolio.Middleware usable with acct.Use()", func() {
			ds := &mockDataSource{
				pricesByAsset: map[string][]float64{},
				currentDate:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			}

			chain := risk.Conservative(ds)
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			// acct.Use accepts ...portfolio.Middleware; spread the slice to confirm compatibility.
			acct.Use(chain...)
		})
	})

	Describe("Moderate", func() {
		It("returns 2 middleware", func() {
			chain := risk.Moderate()
			Expect(chain).To(HaveLen(2))
		})

		It("returns a valid []portfolio.Middleware usable with acct.Use()", func() {
			chain := risk.Moderate()
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.Use(chain...)
		})
	})

	Describe("Aggressive", func() {
		It("returns 2 middleware", func() {
			chain := risk.Aggressive()
			Expect(chain).To(HaveLen(2))
		})

		It("returns a valid []portfolio.Middleware usable with acct.Use()", func() {
			chain := risk.Aggressive()
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.Use(chain...)
		})
	})
})
