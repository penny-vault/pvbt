// Copyright 2021-2025
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

package filter_test

import (
	"github.com/penny-vault/pv-api/filter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database", func() {
	Describe("when building a select", func() {
		Context("with passed parameters", func() {
			It("should error for no 'from'", func() {
				_, _, err := filter.BuildQuery("", make([]string, 0), make([]string, 0), make(map[string]string), "")
				Expect(err).NotTo(BeNil())
			})
			It("should escape select identifiers", func() {
				fields := []string{"a\"a", "b"}
				where := map[string]string{}
				sql, _, err := filter.BuildQuery("my_table", fields, make([]string, 0), where, "event_date DESC")
				Expect(err).To(BeNil())
				Expect(sql).To(Equal(`select "a""a", "b" from "my_table" order by event_date DESC`))
			})
			It("should escape from identifier", func() {
				fields := []string{"a"}
				where := map[string]string{}
				sql, _, err := filter.BuildQuery("my_\"table", fields, make([]string, 0), where, "event_date DESC")
				Expect(err).To(BeNil())
				Expect(sql).To(Equal(`select "a" from "my_""table" order by event_date DESC`))
			})
		})
	})
})
