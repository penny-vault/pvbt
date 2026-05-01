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

var _ = Describe("FRED ticker helpers", func() {
	DescribeTable("IsFREDTicker recognizes the namespace prefix",
		func(ticker string, expected bool) {
			Expect(asset.IsFREDTicker(ticker)).To(Equal(expected))
		},
		Entry("namespaced", "FRED:DGS3MO", true),
		Entry("namespaced empty series", "FRED:", true),
		Entry("plain ticker", "DGS3MO", false),
		Entry("equity ticker", "SPY", false),
		Entry("lower case prefix is not honored", "fred:DGS3MO", false),
		Entry("empty string", "", false),
	)

	DescribeTable("FREDSeries strips the namespace prefix",
		func(ticker, expected string) {
			Expect(asset.FREDSeries(ticker)).To(Equal(expected))
		},
		Entry("namespaced", "FRED:DGS3MO", "DGS3MO"),
		Entry("plain returns input", "DGS3MO", "DGS3MO"),
		Entry("multi-segment series", "FRED:T10Y2Y", "T10Y2Y"),
		Entry("empty namespace", "FRED:", ""),
	)

	Describe("NewFREDAsset", func() {
		It("constructs a synthetic FRED asset from a namespaced ticker", func() {
			a := asset.NewFREDAsset("FRED:DGS3MO")

			Expect(a.Ticker).To(Equal("FRED:DGS3MO"))
			Expect(a.CompositeFigi).To(Equal("FRED:DGS3MO"))
			Expect(a.AssetType).To(Equal(asset.AssetTypeFRED))
			Expect(a.PrimaryExchange).To(Equal(asset.ExchangeFRED))
		})

		It("normalizes a bare series name to the namespaced form", func() {
			a := asset.NewFREDAsset("DGS3MO")

			Expect(a.Ticker).To(Equal("FRED:DGS3MO"))
			Expect(a.CompositeFigi).To(Equal("FRED:DGS3MO"))
			Expect(a.AssetType).To(Equal(asset.AssetTypeFRED))
		})
	})
})
