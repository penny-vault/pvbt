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

package tax_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine/middleware/tax"
)

var _ = Describe("TaxConfig", func() {
	Describe("ApplyDefaults", func() {
		It("applies DefaultLossThreshold when tax enabled and threshold is 0", func() {
			tc := tax.TaxConfig{Enabled: true, LossThreshold: 0}
			Expect(tc.ApplyDefaults()).To(Succeed())
			Expect(tc.LossThreshold).To(Equal(tax.DefaultLossThreshold))
		})

		It("does not overwrite explicit threshold", func() {
			tc := tax.TaxConfig{Enabled: true, LossThreshold: 0.03}
			Expect(tc.ApplyDefaults()).To(Succeed())
			Expect(tc.LossThreshold).To(Equal(0.03))
		})

		It("does not apply default when tax is disabled", func() {
			tc := tax.TaxConfig{Enabled: false, LossThreshold: 0}
			Expect(tc.ApplyDefaults()).To(Succeed())
			Expect(tc.LossThreshold).To(Equal(0.0))
		})
	})
})
