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
	members []asset.Asset
}

// indexChange is a single add or remove event from the changelog.
type indexChange struct {
	date          time.Time
	compositeFigi string
	ticker        string
	action        string // "add" or "remove"
}

// indexState holds the stacks and current membership for one index.
// Dates passed to advance must be monotonically increasing.
type indexState struct {
	snapshots []indexSnapshot // sorted by date, earliest first (index 0 = earliest)
	changelog []indexChange   // sorted by date, earliest first
	snapIdx   int             // next unprocessed snapshot
	logIdx    int             // next unprocessed changelog entry
	members   []asset.Asset   // current constituents; mutated in place
}

// advance pops snapshots and changelog entries up through forDate,
// updating members in place. Returns the current members slice.
func (st *indexState) advance(forDate time.Time) []asset.Asset {
	// Pop snapshots: each one resets members entirely.
	for st.snapIdx < len(st.snapshots) && !st.snapshots[st.snapIdx].date.After(forDate) {
		snap := st.snapshots[st.snapIdx]
		st.snapIdx++

		// Replace members with this snapshot's constituents.
		st.members = st.members[:0]
		st.members = append(st.members, snap.members...)

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
			st.members = append(st.members, asset.Asset{
				CompositeFigi: ch.compositeFigi,
				Ticker:        ch.ticker,
			})
		case "remove":
			for ii := range st.members {
				if st.members[ii].CompositeFigi == ch.compositeFigi {
					last := len(st.members) - 1
					st.members[ii] = st.members[last]
					st.members = st.members[:last]

					break
				}
			}
		}
	}

	return st.members
}

// IndexSnapshotEntry is a snapshot used to construct an indexState.
type IndexSnapshotEntry struct {
	Date    time.Time
	Members []asset.Asset
}

// IndexChangeEntry is a changelog event used to construct an indexState.
type IndexChangeEntry struct {
	Date          time.Time
	CompositeFigi string
	Ticker        string
	Action        string
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
			date:          entry.Date,
			compositeFigi: entry.CompositeFigi,
			ticker:        entry.Ticker,
			action:        entry.Action,
		}
	}

	return &indexState{snapshots: ss, changelog: cc}
}

// Advance is the exported wrapper for advance.
func (st *indexState) Advance(forDate time.Time) []asset.Asset {
	return st.advance(forDate)
}
