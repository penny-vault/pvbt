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
		aapl asset.Asset
		goog asset.Asset
		msft asset.Asset
		day1 time.Time
		day2 time.Time
		day3 time.Time
	)

	BeforeEach(func() {
		aapl = asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
		goog = asset.Asset{CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"}
		msft = asset.Asset{CompositeFigi: "FIGI-MSFT", Ticker: "MSFT"}
		day1 = time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
		day2 = time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)
		day3 = time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	})

	It("returns snapshot members at the snapshot date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			nil,
		)
		members := state.Advance(day1)
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("carries forward snapshot members to later dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			nil,
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl))
	})

	It("applies changelog add between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"}},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("applies changelog remove between snapshots", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl))
	})

	It("resets members when a new snapshot arrives", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []asset.Asset{aapl, goog}},
				{Date: day3, Members: []asset.Asset{aapl, msft}},
			},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"}},
		)
		state.Advance(day1)
		members := state.Advance(day3)
		Expect(members).To(ConsistOf(aapl, msft))
	})

	It("discards changelog entries superseded by a snapshot", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{
				{Date: day1, Members: []asset.Asset{aapl}},
				{Date: day2, Members: []asset.Asset{aapl, goog}},
			},
			[]data.IndexChangeEntry{{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add"}},
		)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl, goog))
	})

	It("returns nil when no data exists before the requested date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day3, Members: []asset.Asset{aapl}}},
			nil,
		)
		members := state.Advance(day1)
		Expect(members).To(BeNil())
	})

	It("returns the borrowed slice without copying", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl, goog}}},
			nil,
		)
		first := state.Advance(day1)
		second := state.Advance(day2)
		Expect(&first[0]).To(BeIdenticalTo(&second[0]))
	})

	It("handles multiple changelog entries on the same date", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"},
				{Date: day2, CompositeFigi: "FIGI-MSFT", Ticker: "MSFT", Action: "add"},
			},
		)
		state.Advance(day1)
		members := state.Advance(day2)
		Expect(members).To(ConsistOf(aapl, goog, msft))
	})

	It("handles changelog add then remove across dates", func() {
		state := data.NewIndexState(
			[]data.IndexSnapshotEntry{{Date: day1, Members: []asset.Asset{aapl}}},
			[]data.IndexChangeEntry{
				{Date: day2, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "add"},
				{Date: day3, CompositeFigi: "FIGI-GOOG", Ticker: "GOOG", Action: "remove"},
			},
		)
		state.Advance(day1)
		state.Advance(day2)
		members := state.Advance(day3)
		Expect(members).To(ConsistOf(aapl))
	})
})
