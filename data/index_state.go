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

	"github.com/penny-vault/pvbt/asset"
)

// indexSnapshot is a point-in-time capture of all constituents for one index.
type indexSnapshot struct {
	date    time.Time
	members []IndexConstituent
}

// indexChange is a single add or remove event from the changelog.
type indexChange struct {
	date   time.Time
	asset  asset.Asset
	action string // "add" or "remove"
	weight float64
}

// indexState holds the stacks and current membership for one index.
// Dates passed to advance must be monotonically increasing.
type indexState struct {
	snapshots    []indexSnapshot    // sorted by date, earliest first (index 0 = earliest)
	changelog    []indexChange      // sorted by date, earliest first
	snapIdx      int                // next unprocessed snapshot
	logIdx       int                // next unprocessed changelog entry
	constituents []IndexConstituent // current constituents; mutated in place
	assets       []asset.Asset      // parallel view for asset-only access
}

// advance pops snapshots and changelog entries up through forDate,
// updating constituents and assets in place.
func (st *indexState) advance(forDate time.Time) {
	// Pop snapshots: each one resets members entirely.
	for st.snapIdx < len(st.snapshots) && !st.snapshots[st.snapIdx].date.After(forDate) {
		snap := st.snapshots[st.snapIdx]
		st.snapIdx++

		// Replace constituents with this snapshot's members.
		st.constituents = st.constituents[:0]
		st.assets = st.assets[:0]
		st.constituents = append(st.constituents, snap.members...)

		for _, ic := range snap.members {
			st.assets = append(st.assets, ic.Asset)
		}

		// Discard changelog entries at or before this snapshot date.
		for st.logIdx < len(st.changelog) && !st.changelog[st.logIdx].date.After(snap.date) {
			st.logIdx++
		}
	}

	// Apply changelog entries up through forDate.
	for st.logIdx < len(st.changelog) && !st.changelog[st.logIdx].date.After(forDate) {
		ch := st.changelog[st.logIdx]
		st.logIdx++

		switch ch.action {
		case "add":
			st.constituents = append(st.constituents, IndexConstituent{
				Asset:  ch.asset,
				Weight: ch.weight,
			})
			st.assets = append(st.assets, ch.asset)
		case "remove":
			for ii := range st.constituents {
				if st.constituents[ii].Asset.CompositeFigi == ch.asset.CompositeFigi {
					last := len(st.constituents) - 1
					st.constituents[ii] = st.constituents[last]
					st.constituents = st.constituents[:last]
					st.assets[ii] = st.assets[last]
					st.assets = st.assets[:last]

					break
				}
			}
		}
	}
}

// IndexSnapshotEntry is a snapshot used to construct an indexState.
type IndexSnapshotEntry struct {
	Date    time.Time
	Members []IndexConstituent
}

// IndexChangeEntry is a changelog event used to construct an indexState.
// Asset must carry full metadata (Sector, Industry, Name, etc.); the engine
// surfaces this asset directly to strategies via IndexUniverse.Assets().
type IndexChangeEntry struct {
	Date   time.Time
	Asset  asset.Asset
	Action string
	Weight float64
}

// NewIndexState creates an indexState from pre-sorted snapshots and changelog
// entries (earliest first). Used by PVDataProvider and tests.
func NewIndexState(snapshots []IndexSnapshotEntry, changelog []IndexChangeEntry) *indexState {
	ss := make([]indexSnapshot, len(snapshots))
	for ii, entry := range snapshots {
		ss[ii] = indexSnapshot{date: entry.Date, members: entry.Members}
	}

	cc := make([]indexChange, len(changelog))
	for ii, entry := range changelog {
		cc[ii] = indexChange{
			date:   entry.Date,
			asset:  entry.Asset,
			action: entry.Action,
			weight: entry.Weight,
		}
	}

	return &indexState{snapshots: ss, changelog: cc}
}

// Advance is the exported wrapper for advance. It returns both the asset-only
// slice and the full constituent slice with weights.
func (st *indexState) Advance(forDate time.Time) ([]asset.Asset, []IndexConstituent) {
	st.advance(forDate)
	return st.assets, st.constituents
}
