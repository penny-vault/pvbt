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

package asset_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
)

var _ = Describe("NormalizeExchange", func() {
	DescribeTable("maps raw exchange codes to normalized values",
		func(raw string, expected asset.Exchange) {
			Expect(asset.NormalizeExchange(raw)).To(Equal(expected))
		},
		Entry("NYSE", "NYSE", asset.ExchangeNYSE),
		Entry("XNYS", "XNYS", asset.ExchangeNYSE),
		Entry("NYSE ARCA", "NYSE ARCA", asset.ExchangeNYSE),
		Entry("NYSE MKT", "NYSE MKT", asset.ExchangeNYSE),
		Entry("ARCX", "ARCX", asset.ExchangeNYSE),
		Entry("XASE", "XASE", asset.ExchangeNYSE),
		Entry("AMEX", "AMEX", asset.ExchangeNYSE),
		Entry("NASDAQ", "NASDAQ", asset.ExchangeNASDAQ),
		Entry("XNAS", "XNAS", asset.ExchangeNASDAQ),
		Entry("NMFQS", "NMFQS", asset.ExchangeNASDAQ),
		Entry("BATS", "BATS", asset.ExchangeBATS),
		Entry("FRED", "FRED", asset.ExchangeFRED),
		Entry("unknown passes through", "MYSTERY", asset.Exchange("MYSTERY")),
		Entry("empty string passes through", "", asset.Exchange("")),
	)
})
