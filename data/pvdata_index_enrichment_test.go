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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
)

var _ = Describe("enrichIndexState", func() {
	var (
		mtbStub      asset.Asset
		mtbFull      asset.Asset
		vloStub      asset.Asset
		vloFull      asset.Asset
		day1         time.Time
		day2         time.Time
		assetsByFigi map[string]asset.Asset
	)

	BeforeEach(func() {
		mtbStub = asset.Asset{CompositeFigi: "BBG000D9KWL9", Ticker: "MTB"}
		mtbFull = asset.Asset{
			CompositeFigi:   "BBG000D9KWL9",
			Ticker:          "MTB",
			Name:            "M&T Bank Corporation",
			AssetType:       asset.AssetTypeCommonStock,
			PrimaryExchange: asset.ExchangeNYSE,
			Sector:          asset.SectorFinancialServices,
			Industry:        asset.IndustryBanksRegional,
		}
		vloStub = asset.Asset{CompositeFigi: "BBG000BBGFG6", Ticker: "VLO"}
		vloFull = asset.Asset{
			CompositeFigi:   "BBG000BBGFG6",
			Ticker:          "VLO",
			Name:            "Valero Energy Corporation",
			AssetType:       asset.AssetTypeCommonStock,
			PrimaryExchange: asset.ExchangeNYSE,
			Sector:          asset.SectorEnergy,
			Industry:        asset.IndustryOilGasRefiningMarketing,
		}
		day1 = time.Date(2023, 12, 29, 16, 0, 0, 0, time.UTC)
		day2 = time.Date(2024, 1, 5, 16, 0, 0, 0, time.UTC)

		assetsByFigi = map[string]asset.Asset{
			mtbFull.CompositeFigi: mtbFull,
			vloFull.CompositeFigi: vloFull,
		}
	})

	It("replaces stub Asset on snapshot constituents with full metadata", func() {
		snapshots := []IndexSnapshotEntry{
			{
				Date: day1,
				Members: []IndexConstituent{
					{Asset: mtbStub, Weight: 0.5},
					{Asset: vloStub, Weight: 0.5},
				},
			},
		}

		Expect(enrichIndexState(snapshots, nil, assetsByFigi)).To(Succeed())

		Expect(snapshots[0].Members[0].Asset).To(Equal(mtbFull))
		Expect(snapshots[0].Members[0].Asset.Sector).To(Equal(asset.SectorFinancialServices))
		Expect(snapshots[0].Members[1].Asset).To(Equal(vloFull))
		Expect(snapshots[0].Members[1].Asset.Sector).To(Equal(asset.SectorEnergy))
	})

	It("replaces stub Asset on changelog entries with full metadata", func() {
		changelog := []IndexChangeEntry{
			{Date: day2, Asset: mtbStub, Action: "add", Weight: 0.4},
			{Date: day2, Asset: vloStub, Action: "remove"},
		}

		Expect(enrichIndexState(nil, changelog, assetsByFigi)).To(Succeed())

		Expect(changelog[0].Asset).To(Equal(mtbFull))
		Expect(changelog[0].Asset.Sector).To(Equal(asset.SectorFinancialServices))
		Expect(changelog[1].Asset).To(Equal(vloFull))
		Expect(changelog[1].Asset.Sector).To(Equal(asset.SectorEnergy))
	})

	It("flows enriched metadata through to indexState.Advance", func() {
		snapshots := []IndexSnapshotEntry{
			{Date: day1, Members: []IndexConstituent{{Asset: mtbStub, Weight: 1.0}}},
		}
		changelog := []IndexChangeEntry{
			{Date: day2, Asset: vloStub, Action: "add", Weight: 0.5},
		}

		Expect(enrichIndexState(snapshots, changelog, assetsByFigi)).To(Succeed())

		state := NewIndexState(snapshots, changelog)
		state.Advance(day1)
		assets, constituents := state.Advance(day2)

		Expect(assets).To(ConsistOf(mtbFull, vloFull))

		bySector := map[asset.Sector]int{}
		for _, ic := range constituents {
			bySector[ic.Asset.Sector]++
		}

		Expect(bySector[asset.SectorFinancialServices]).To(Equal(1))
		Expect(bySector[asset.SectorEnergy]).To(Equal(1))
		Expect(bySector[""]).To(Equal(0), "no constituent should have empty Sector")
	})

	It("returns an error when a snapshot constituent is missing from the assets map", func() {
		snapshots := []IndexSnapshotEntry{
			{
				Date: day1,
				Members: []IndexConstituent{
					{Asset: mtbStub, Weight: 0.5},
					{Asset: asset.Asset{CompositeFigi: "BBG-UNKNOWN", Ticker: "UNK"}, Weight: 0.5},
				},
			},
		}

		err := enrichIndexState(snapshots, nil, assetsByFigi)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("BBG-UNKNOWN"))
		Expect(err.Error()).To(ContainSubstring("UNK"))
	})

	It("returns an error when a changelog entry is missing from the assets map", func() {
		changelog := []IndexChangeEntry{
			{Date: day2, Asset: asset.Asset{CompositeFigi: "BBG-MISSING", Ticker: "MIS"}, Action: "add"},
		}

		err := enrichIndexState(nil, changelog, assetsByFigi)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("BBG-MISSING"))
		Expect(err.Error()).To(ContainSubstring("MIS"))
	})
})
