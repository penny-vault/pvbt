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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
)

var _ = Describe("PVDataProvider.LookupAsset FRED prefix", func() {
	It("returns a synthetic Asset for FRED-namespaced tickers without touching the database", func() {
		// A nil pool would normally panic on Acquire; the FRED prefix branch
		// must short-circuit before any database access.
		provider := &PVDataProvider{}

		got, err := provider.LookupAsset(context.Background(), "FRED:DGS3MO")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Ticker).To(Equal("FRED:DGS3MO"))
		Expect(got.CompositeFigi).To(Equal("FRED:DGS3MO"))
		Expect(got.AssetType).To(Equal(asset.AssetTypeFRED))
		Expect(got.PrimaryExchange).To(Equal(asset.ExchangeFRED))
	})
})
