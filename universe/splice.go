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

package universe

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// SplicePeriod declares a fallback ticker for dates strictly before a cutoff.
// Used by NewSplice to substitute historical proxies when the primary ticker
// did not yet exist (e.g. QLD before TQQQ was listed in early 2010).
type SplicePeriod struct {
	Ticker string
	Before time.Time
}

// compile-time check
var _ Universe = (*SpliceUniverse)(nil)

// SpliceUniverse is a single-asset universe that substitutes different
// tickers across history. The primary ticker is "active" by default; each
// fallback covers dates strictly before its Before cutoff. Pre-cutoff bars in
// data fetches come from the fallback's price series, which is useful for
// extending backtests of leveraged or thematic ETFs that did not exist for
// the full study period.
//
// SpliceUniverse always reports a single active asset for any given date, so
// it composes naturally with weighting functions like portfolio.EqualWeight
// and with order routing -- the engine places orders against whichever ticker
// is live on the order date.
type SpliceUniverse struct {
	segments []spliceSegment // chronological; the last segment is the primary
	ds       DataSource
}

// spliceSegment defines [start, end) coverage of a particular ticker.
// Zero start means -infinity; zero end means +infinity. The primary segment
// is the last entry and has zero end.
type spliceSegment struct {
	asset asset.Asset
	start time.Time
	end   time.Time
}

// SetDataSource wires the universe to a data source.
func (u *SpliceUniverse) SetDataSource(ds DataSource) { u.ds = ds }

// Resolve replaces each segment's bare ticker with a fully-resolved asset.
// Strategies typically do not call this directly; the engine helper
// SpliceUniverse() handles resolution before returning.
func (u *SpliceUniverse) Resolve(lookup func(ticker string) asset.Asset) {
	for ii := range u.segments {
		u.segments[ii].asset = lookup(u.segments[ii].asset.Ticker)
	}
}

// Assets returns the active ticker as of asOf. Always a single-element slice.
func (u *SpliceUniverse) Assets(asOf time.Time) []asset.Asset {
	return []asset.Asset{u.segmentFor(asOf).asset}
}

// segmentFor finds the segment whose [start, end) covers asOf, defaulting to
// the primary segment if asOf lies before the earliest cutoff (which can
// happen only if Assets is queried at the zero time).
func (u *SpliceUniverse) segmentFor(asOf time.Time) spliceSegment {
	for _, seg := range u.segments {
		if !seg.start.IsZero() && asOf.Before(seg.start) {
			continue
		}

		if !seg.end.IsZero() && !asOf.Before(seg.end) {
			continue
		}

		return seg
	}

	return u.segments[len(u.segments)-1]
}

// Window fetches data for each segment intersecting the lookback range,
// relabels each piece's column to the currently-active ticker, and stitches
// them together chronologically. The returned DataFrame has a single asset
// column whose identity matches whichever ticker is live as of the current
// simulation date, so signals computed from it route orders to that ticker.
func (u *SpliceUniverse) Window(ctx context.Context, lookback portfolio.Period, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.SpliceUniverse()?")
	}

	now := u.ds.CurrentDate()
	windowStart := lookback.Before(now)
	active := u.segmentFor(now).asset

	pieces := make([]*data.DataFrame, 0, len(u.segments))

	for _, seg := range u.segments {
		segStart := seg.start
		if segStart.IsZero() || segStart.Before(windowStart) {
			segStart = windowStart
		}

		segEnd := seg.end
		if segEnd.IsZero() || now.Before(segEnd) {
			segEnd = now
		}

		if !segStart.Before(segEnd) {
			continue
		}

		raw, err := u.ds.Fetch(ctx, []asset.Asset{seg.asset}, lookback, metrics)
		if err != nil {
			return nil, fmt.Errorf("splice fetch %s: %w", seg.asset.Ticker, err)
		}

		// Between is inclusive on both ends; segEnd is exclusive in our model,
		// so step back by one nanosecond to avoid double-counting the boundary.
		sliced := raw.Between(segStart, segEnd.Add(-time.Nanosecond))
		if sliced.Len() == 0 {
			continue
		}

		relabeled, err := relabelSingle(sliced, seg.asset, active)
		if err != nil {
			return nil, fmt.Errorf("splice relabel %s -> %s: %w", seg.asset.Ticker, active.Ticker, err)
		}

		pieces = append(pieces, relabeled)
	}

	if len(pieces) == 0 {
		return data.WithErr(fmt.Errorf("splice %s: no data in window [%s, %s]",
			active.Ticker, windowStart.Format(time.DateOnly), now.Format(time.DateOnly))), nil
	}

	return data.MergeTimes(pieces...)
}

// At returns a single-row DataFrame at the current simulation date for
// whichever ticker is active on that date.
func (u *SpliceUniverse) At(ctx context.Context, metrics ...data.Metric) (*data.DataFrame, error) {
	if u.ds == nil {
		return nil, fmt.Errorf("universe has no data source; was it created via engine.SpliceUniverse()?")
	}

	now := u.ds.CurrentDate()

	return u.ds.FetchAt(ctx, []asset.Asset{u.segmentFor(now).asset}, now, metrics)
}

// CurrentDate returns the current simulation date from the data source, or
// the zero time if no data source is set.
func (u *SpliceUniverse) CurrentDate() time.Time {
	if u.ds == nil {
		return time.Time{}
	}

	return u.ds.CurrentDate()
}

// NewSplice creates a single-asset universe that substitutes proxy tickers
// for dates before the primary's listing. Fallbacks may be passed in any
// order; they are sorted by Before ascending. Each fallback covers dates
// strictly before its Before cutoff (and at-or-after the previous cutoff).
// The primary covers all remaining dates from the latest cutoff onward.
//
// The universe has no data source until it is wired via
// engine.SpliceUniverse(), which also resolves each ticker through the
// engine's asset registry so downstream fetches have full asset metadata.
func NewSplice(primary string, fallbacks ...SplicePeriod) *SpliceUniverse {
	sorted := make([]SplicePeriod, len(fallbacks))
	copy(sorted, fallbacks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before.Before(sorted[j].Before) })

	segments := make([]spliceSegment, 0, len(sorted)+1)

	var prevCutoff time.Time

	for _, fb := range sorted {
		segments = append(segments, spliceSegment{
			asset: asset.Asset{Ticker: fb.Ticker},
			start: prevCutoff,
			end:   fb.Before,
		})
		prevCutoff = fb.Before
	}

	segments = append(segments, spliceSegment{
		asset: asset.Asset{Ticker: primary},
		start: prevCutoff,
	})

	return &SpliceUniverse{segments: segments}
}

// relabelSingle returns a new DataFrame whose single asset is target instead
// of source, preserving the time axis, metrics, and column data. Used to
// give every spliced segment a common column identity so MergeTimes can
// concatenate them.
func relabelSingle(df *data.DataFrame, source, target asset.Asset) (*data.DataFrame, error) {
	assets := df.AssetList()
	if len(assets) != 1 || assets[0].CompositeFigi != source.CompositeFigi {
		return nil, fmt.Errorf("expected single-asset frame for %s", source.Ticker)
	}

	metrics := df.MetricList()
	cols := make([][]float64, len(metrics))

	for mi, metric := range metrics {
		col := df.Column(source, metric)

		out := make([]float64, len(col))
		copy(out, col)
		cols[mi] = out
	}

	times := df.Times()

	return data.NewDataFrame(times, []asset.Asset{target}, metrics, df.Frequency(), cols)
}
