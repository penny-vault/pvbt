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

package portfolio

// taxRates holds the assumed effective tax rates used by tax metrics.
// All tax-aware metrics (TaxDrag, AfterTaxTWRR, AfterTaxCAGR, and the
// benchmark after-tax variants) read from this single source so they
// stay consistent.
type taxRates struct {
	// LTCG is the long-term capital gains rate applied to gains on
	// positions held longer than 365 days.
	LTCG float64
	// STCG is the short-term capital gains rate applied to gains on
	// positions held 365 days or less.
	STCG float64
}

// defaultTaxRates returns the assumed effective rates: 15% LTCG and
// 25% STCG. These match the values used by the original TaxDrag
// implementation and apply uniformly to portfolio and benchmark
// after-tax metrics.
func defaultTaxRates() taxRates {
	return taxRates{LTCG: 0.15, STCG: 0.25}
}
