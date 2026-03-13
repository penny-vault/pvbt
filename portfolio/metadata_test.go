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

var _ = Describe("Metadata", func() {
	var acct *portfolio.Account

	BeforeEach(func() {
		acct = portfolio.New()
	})

	It("returns empty string for unset key", func() {
		Expect(acct.GetMetadata("missing")).To(Equal(""))
	})

	It("round-trips a key-value pair", func() {
		acct.SetMetadata("run_id", "abc-123")
		Expect(acct.GetMetadata("run_id")).To(Equal("abc-123"))
	})

	It("overwrites an existing key", func() {
		acct.SetMetadata("key", "old")
		acct.SetMetadata("key", "new")
		Expect(acct.GetMetadata("key")).To(Equal("new"))
	})

	It("stores multiple keys independently", func() {
		acct.SetMetadata("a", "1")
		acct.SetMetadata("b", "2")
		Expect(acct.GetMetadata("a")).To(Equal("1"))
		Expect(acct.GetMetadata("b")).To(Equal("2"))
	})
})
