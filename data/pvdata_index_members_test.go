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

package data_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

var _ = Describe("IndexState", func() {
	var (
		aapl   asset.Asset
		goog   asset.Asset
		msft   asset.Asset
		aaplIC data.IndexConstituent
		googIC data.IndexConstituent
		msftIC data.IndexConstituent
		day1   time.Time
		day2   time.Time
		day3   time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		aaplIC = data.IndexConstituent{Asset: aapl, Weight: 0.5}
		googIC = data.IndexConstituent{Asset: goog, Weight: 0.3}
		msftIC = data.IndexConstituent{Asset: msft, Weight: 0.2}
		day1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		day2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
		day3 = time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	})

	It("returns snapshot members at the snapshot date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC, googIC}}},
			nil,
		)
		assets, constituents := state.Advance(day1)
		Expect(assets).To(ConsistOf(aapl, goog))
		Expect(constituents).To(ConsistOf(aaplIC, googIC))
	})

	It("carries forward snapshot members to later dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC}}},
			nil,
		)
		state.Advance(day1)
		assets, _ := state.Advance(day2)
		Expect(assets).To(ConsistOf(aapl))
	})

	It("applies changelog add between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add", Weight: 0.3}},
		)
		state.Advance(day1)
		assets, constituents := state.Advance(day2)
		Expect(assets).To(ConsistOf(aapl, goog))
		Expect(constituents).To(HaveLen(2))
	})

	It("applies changelog remove between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC, googIC}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		assets, constituents := state.Advance(day2)
		Expect(assets).To(ConsistOf(aapl))
		Expect(constituents).To(HaveLen(1))
		Expect(constituents[0].Asset).To(Equal(aapl))
	})

	It("resets members when a new snapshot arrives", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []data.IndexConstituent{aaplIC, googIC}},
				{Date: day3, Members: []data.IndexConstituent{aaplIC, msftIC}},
			},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		assets, _ := state.Advance(day3)
		Expect(assets).To(ConsistOf(aapl, msft))
	})

	It("discards changelog entries superseded by a snapshot", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []data.IndexConstituent{aaplIC}},
				{Date: day2, Members: []data.IndexConstituent{aaplIC, googIC}},
			},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add"}},
		)
		assets, _ := state.Advance(day2)
		Expect(assets).To(ConsistOf(aapl, goog))
	})

	It("returns nil when no data exists before the requested date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day3, Members: []data.IndexConstituent{aaplIC}}},
			nil,
		)
		assets, constituents := state.Advance(day1)
		Expect(assets).To(BeNil())
		Expect(constituents).To(BeNil())
	})

	It("returns the borrowed slice without copying", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC, googIC}}},
			nil,
		)
		firstAssets, _ := state.Advance(day1)
		secondAssets, _ := state.Advance(day2)
		Expect(&firstAssets[0]).To(BeIdenticalTo(&secondAssets[0]))
	})

	It("handles multiple changelog entries on the same date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add", Weight: 0.3},
				{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add", Weight: 0.2},
			},
		)
		state.Advance(day1)
		assets, _ := state.Advance(day2)
		Expect(assets).To(ConsistOf(aapl, goog, msft))
	})

	It("handles changelog add then remove across dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add", Weight: 0.3},
				{Date: day3, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"},
			},
		)
		state.Advance(day1)
		state.Advance(day2)
		assets, _ := state.Advance(day3)
		Expect(assets).To(ConsistOf(aapl))
	})

	It("preserves weight data through snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{
				{Asset: aapl, Weight: 0.6},
				{Asset: goog, Weight: 0.4},
			}}},
			nil,
		)
		_, constituents := state.Advance(day1)
		Expect(constituents).To(HaveLen(2))

		weightMap := make(map[string]float64)
		for _, ic := range constituents {
			weightMap[ic.Asset.Ticker] = ic.Weight
		}
		Expect(weightMap["AAPL"]).To(BeNumerically("~", 0.6, 0.001))
		Expect(weightMap["GOOG"]).To(BeNumerically("~", 0.4, 0.001))
	})

	It("handles changelog-only data without snapshots", func() {
		state := data.NewIndexState(
			nil,
			[]data.IndexChangeEntry{
				{Date: day1, CompositeFigi: "FIGI-AAPL", Ticker: "AAPL", Action: "add", Weight: 0.5},
			},
		)
		assets, constituents := state.Advance(day1)
		Expect(assets).To(ConsistOf(aapl))
		Expect(constituents).To(HaveLen(1))
		Expect(constituents[0].Weight).To(BeNumerically("~", 0.5, 0.001))
	})

	It("preserves weight data through changelog adds", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []data.IndexConstituent{aaplIC}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add", Weight: 0.35}},
		)
		state.Advance(day1)
		_, constituents := state.Advance(day2)
		Expect(constituents).To(HaveLen(2))

		for _, ic := range constituents {
			if ic.Asset.Ticker == "GOOG" {
				Expect(ic.Weight).To(BeNumerically("~", 0.35, 0.001))
			}
		}
	})
})
