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

package data

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("buildFundamentalColumns", func() {
	It("does not emit REAL definitions for structural DDL columns", func() {
		cols := buildFundamentalColumns()

		// Structural columns are declared explicitly in the fundamentals DDL
		// template; emitting them again as REAL would produce invalid SQL.
		Expect(cols).NotTo(ContainSubstring("date_key REAL"))
		Expect(cols).NotTo(ContainSubstring("report_period REAL"))
		Expect(cols).NotTo(ContainSubstring("dimension REAL"))
	})

	It("includes at least one known fundamental metric column", func() {
		cols := buildFundamentalColumns()

		// Sanity-check that the function is not returning an empty string,
		// which would indicate all entries were incorrectly suppressed.
		Expect(cols).To(ContainSubstring("working_capital REAL"))
	})
})
