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
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/rs/zerolog"
)

// easternTZ is the market-local timezone used for intraday timestamps.
// pv-data writes ClickHouse event_date in UTC; pvbt converts to Eastern
// at decode time to match the convention used everywhere else.
var easternTZ *time.Location

func init() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("pvdata: load America/New_York: " + err.Error())
	}

	easternTZ = loc
}

// IntradayDataTypeKey is the pv-data data-type key for 1-minute bars.
const IntradayDataTypeKey = "intraday-bar"

// IntradayMetric reports whether the metric is one of the OHLCV columns
// served by the intraday backend (raw or adjusted).
func IntradayMetric(metric Metric) bool {
	switch metric {
	case MetricOpen, MetricHigh, MetricLow, MetricClose, Volume,
		AdjOpen, AdjHigh, AdjLow, AdjClose, AdjVolume:
		return true
	}

	return false
}

// adjustedMetric reports whether the metric requires split/dividend
// adjustment to be applied at decode time over raw ClickHouse rows.
func adjustedMetric(metric Metric) bool {
	switch metric {
	case AdjOpen, AdjHigh, AdjLow, AdjClose, AdjVolume:
		return true
	}

	return false
}

// resolveIntradayTable looks up the physical ClickHouse table that holds
// 1-minute bars. The pv-data subscriptions table records, for each active
// subscription, parallel arrays of data_types and data_tables; we find
// the row whose data_types contains 'intraday-bar' and pick the matching
// table name. When more than one active subscription supplies intraday
// bars, the configured intraday_provider disambiguates; an unset provider
// in the multi-subscription case is a hard error rather than a silent
// arbitrary pick.
func (p *PVDataProvider) resolveIntradayTable(ctx context.Context) (string, error) {
	p.intradayResolveOnce.Do(func() {
		p.intradayTable, p.intradayResolveErr = p.queryIntradayTable(ctx)
	})

	return p.intradayTable, p.intradayResolveErr
}

func (p *PVDataProvider) queryIntradayTable(ctx context.Context) (string, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("pvdata: acquire connection for intraday subscription lookup: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT provider, data_types, data_tables
		 FROM subscriptions
		 WHERE active = true AND $1 = ANY(data_types::text[])`,
		IntradayDataTypeKey,
	)
	if err != nil {
		return "", fmt.Errorf("pvdata: query intraday subscriptions: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		provider string
		table    string
	}

	var candidates []candidate

	for rows.Next() {
		var (
			provider   string
			dataTypes  []string
			dataTables []string
		)

		if scanErr := rows.Scan(&provider, &dataTypes, &dataTables); scanErr != nil {
			return "", fmt.Errorf("pvdata: scan intraday subscription row: %w", scanErr)
		}

		if len(dataTypes) != len(dataTables) {
			return "", fmt.Errorf(
				"pvdata: subscription for provider %q has %d data_types but %d data_tables",
				provider, len(dataTypes), len(dataTables))
		}

		for idx, dt := range dataTypes {
			if dt == IntradayDataTypeKey {
				candidates = append(candidates, candidate{provider: provider, table: dataTables[idx]})
				break
			}
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return "", fmt.Errorf("pvdata: iterate intraday subscriptions: %w", rowsErr)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf(
			"pvdata: no active subscription for data type %q in subscriptions table; "+
				"intraday bars are not available", IntradayDataTypeKey)
	}

	if len(candidates) == 1 {
		return candidates[0].table, nil
	}

	if p.intradayProvider == "" {
		providers := make([]string, len(candidates))
		for i, cand := range candidates {
			providers[i] = cand.provider
		}

		return "", fmt.Errorf(
			"pvdata: %d active subscriptions supply %q (providers: %s); set "+
				"[intraday] provider in ~/.pvdata.toml to disambiguate",
			len(candidates), IntradayDataTypeKey, strings.Join(providers, ", "))
	}

	for _, cand := range candidates {
		if cand.provider == p.intradayProvider {
			return cand.table, nil
		}
	}

	return "", fmt.Errorf(
		"pvdata: configured intraday provider %q does not match any active "+
			"subscription supplying %q",
		p.intradayProvider, IntradayDataTypeKey)
}

// fetchMinuteBars loads 1-minute bar columns from the ClickHouse
// intraday-bar table according to the access pattern carried by req.
// The pattern is one of:
//
//   - dense:  bars in [req.Start, req.End] for the requested figis
//   - sparse: bars whose time-of-day appears in req.TimesOfDay,
//     across [req.Start, req.End]
//
// Adjusted metrics are scaled at decode time using the splits and
// dividends rows from Postgres. Volume returns are split-only; open/
// high/low/close return are split-and-dividend.
func (p *PVDataProvider) fetchMinuteBars(
	ctx context.Context,
	req intradayRequest,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	log := zerolog.Ctx(ctx)

	if len(req.figis) == 0 || len(req.metrics) == 0 {
		return nil
	}

	for _, metric := range req.metrics {
		if !IntradayMetric(metric) {
			return fmt.Errorf("pvdata: metric %q is not an intraday metric", metric)
		}
	}

	conn, err := p.clickHouse(ctx)
	if err != nil {
		return err
	}

	table, err := p.resolveIntradayTable(ctx)
	if err != nil {
		return err
	}

	// Split metrics into raw vs adjusted. Raw metrics are fetched as-is;
	// adjusted metrics need a per-row scaling pass.
	var (
		needRaw bool
		needAdj bool
	)

	for _, metric := range req.metrics {
		if adjustedMetric(metric) {
			needAdj = true
		} else {
			needRaw = true
		}
	}

	// Build per-symbol cumulative adjustment factors only when needed.
	var (
		priceFactor  map[string]map[int64]float64 // figi -> sec -> split*div multiplier
		volumeFactor map[string]map[int64]float64 // figi -> sec -> split-only multiplier
	)

	if needAdj {
		priceFactor, volumeFactor, err = p.loadAdjustmentFactors(ctx, req.figis, req.start, req.end)
		if err != nil {
			return err
		}
	}

	// If only adjusted metrics were requested but no raw counterpart, we
	// still need to read raw OHLCV from ClickHouse and transform.
	_ = needRaw

	query, args := buildIntradayQuery(table, req)

	log.Debug().
		Str("table", table).
		Strs("figis", req.figis).
		Time("start", req.start).
		Time("end", req.end).
		Int("times_of_day", len(req.timesOfDay)).
		Msg("PVDataProvider.fetchMinuteBars")

	rows, queryErr := conn.Query(ctx, query, args...)
	if queryErr != nil {
		return fmt.Errorf("pvdata: query intraday bars: %w", queryErr)
	}
	defer rows.Close()

	rowCount := 0

	for rows.Next() {
		var (
			figi      string
			eventDate time.Time
			open      float64
			high      float64
			low       float64
			closeVal  float64
			volume    float64
		)

		if scanErr := rows.Scan(&figi, &eventDate, &open, &high, &low, &closeVal, &volume); scanErr != nil {
			return fmt.Errorf("pvdata: scan intraday row: %w", scanErr)
		}

		// ClickHouse stores event_date as DateTime in UTC. Convert to
		// Eastern at decode time so the in-memory timestamps are
		// market-local, matching the convention in the rest of pvbt.
		eventDate = eventDate.In(easternTZ)

		sec := eventDate.Unix()
		timeSet[sec] = eventDate
		rowCount++

		// Look up adjustment factors for this (figi, time) only when
		// adjusted metrics were requested.
		var (
			priceMul  float64 = 1
			volumeMul float64 = 1
		)

		if needAdj {
			if symFactor, ok := priceFactor[figi]; ok {
				priceMul = lookupFactorAt(symFactor, sec)
			}

			if symFactor, ok := volumeFactor[figi]; ok {
				volumeMul = lookupFactorAt(symFactor, sec)
			}
		}

		for _, metric := range req.metrics {
			var value float64

			switch metric {
			case MetricOpen:
				value = open
			case MetricHigh:
				value = high
			case MetricLow:
				value = low
			case MetricClose:
				value = closeVal
			case Volume:
				value = volume
			case AdjOpen:
				value = open * priceMul
			case AdjHigh:
				value = high * priceMul
			case AdjLow:
				value = low * priceMul
			case AdjClose:
				value = closeVal * priceMul
			case AdjVolume:
				value = volume * volumeMul
			default:
				continue
			}

			ensureCol(figi, metric)[sec] = value
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return fmt.Errorf("pvdata: iterate intraday rows: %w", rowsErr)
	}

	log.Debug().Int("rows", rowCount).Msg("PVDataProvider.fetchMinuteBars result")

	return nil
}

// intradayRequest is the internal request shape passed into the minute-bar
// fetch path. Sparse requests set timesOfDay; dense requests leave it empty.
type intradayRequest struct {
	figis      []string
	metrics    []Metric
	start      time.Time
	end        time.Time
	timesOfDay []TimeOfDay
}

// buildIntradayQuery composes the SELECT against the resolved intraday
// table. When timesOfDay is non-empty, the time-of-day predicate pushes
// down to ClickHouse so only matching rows are returned.
func buildIntradayQuery(table string, req intradayRequest) (string, []any) {
	args := []any{req.figis, req.start, req.end}

	var sb strings.Builder

	sb.WriteString("SELECT composite_figi, event_date, open, high, low, close, volume FROM ")
	sb.WriteString(table)
	sb.WriteString(" WHERE composite_figi IN ($1) AND event_date >= $2 AND event_date < $3")

	if len(req.timesOfDay) > 0 {
		minutes := make([]int32, len(req.timesOfDay))
		for i, tod := range req.timesOfDay {
			minutes[i] = int32(tod.MinutesSinceMidnight())
		}

		args = append(args, minutes)
		fmt.Fprintf(&sb,
			" AND (toHour(event_date)*60 + toMinute(event_date)) IN ($%d)",
			len(args))
	}

	sb.WriteString(" ORDER BY composite_figi, event_date")

	return sb.String(), args
}

// loadAdjustmentFactors builds per-symbol cumulative split/dividend
// factors from the Postgres splits and dividends tables. The factor at
// time t represents the multiplier that converts a raw price quoted at t
// into an adjusted price expressed in the most recent reporting basis.
//
// For prices: factor includes both splits and dividends.
// For volume: factor includes splits only (cash dividends do not change
// share counts).
func (p *PVDataProvider) loadAdjustmentFactors(
	ctx context.Context,
	figis []string,
	start, end time.Time,
) (priceFactor, volumeFactor map[string]map[int64]float64, _ error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("pvdata: acquire connection for adjustment factors: %w", err)
	}
	defer conn.Release()

	// Splits and dividends are sourced from the existing eod view; we
	// pick out only the rows where one of the adjustment fields is
	// non-null. The query covers a buffer beyond the requested window
	// because the most recent split/dividend after `end` would still
	// adjust historical prices within the window.
	bufferEnd := end.AddDate(2, 0, 0)

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, event_date, close, dividend, split_factor
		 FROM eod
		 WHERE composite_figi = ANY($1)
		   AND event_date >= $2::date AND event_date <= $3::date
		   AND (dividend IS NOT NULL AND dividend > 0
		        OR split_factor IS NOT NULL AND split_factor != 1)
		 ORDER BY composite_figi, event_date DESC`,
		figis, start.AddDate(0, 0, -1), bufferEnd,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("pvdata: query adjustment factors: %w", err)
	}
	defer rows.Close()

	type adjEvent struct {
		date    time.Time
		closeP  float64
		divid   float64
		splitFa float64
	}

	bySymbol := make(map[string][]adjEvent)

	for rows.Next() {
		var (
			figi    string
			eventDt time.Time
			closeP  *float64
			divid   *float64
			split   *float64
		)

		if scanErr := rows.Scan(&figi, &eventDt, &closeP, &divid, &split); scanErr != nil {
			return nil, nil, fmt.Errorf("pvdata: scan adjustment row: %w", scanErr)
		}

		ev := adjEvent{date: eventDt, splitFa: 1}

		if closeP != nil {
			ev.closeP = *closeP
		}

		if divid != nil {
			ev.divid = *divid
		}

		if split != nil && *split != 0 {
			ev.splitFa = *split
		}

		bySymbol[figi] = append(bySymbol[figi], ev)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, nil, fmt.Errorf("pvdata: iterate adjustment rows: %w", rowsErr)
	}

	priceFactor = make(map[string]map[int64]float64, len(bySymbol))
	volumeFactor = make(map[string]map[int64]float64, len(bySymbol))

	// For each symbol, walk events oldest-to-newest and accumulate the
	// going-backward factors. Volume uses split factors only; price uses
	// split * (1 - dividend/prevClose).
	for figi, events := range bySymbol {
		// Sort ascending by date for forward walk.
		sort.Slice(events, func(i, j int) bool { return events[i].date.Before(events[j].date) })

		// Build a per-event factor tier. We build cumulative factors
		// from "now" looking backwards: a price quoted before event E
		// must be multiplied by E's factor to be expressed in current
		// basis.
		priceCum := make([]float64, len(events))
		volumeCum := make([]float64, len(events))

		// Initialize with 1 then walk newest-to-oldest accumulating.
		cumPrice := 1.0
		cumVolume := 1.0

		for idx := len(events) - 1; idx >= 0; idx-- {
			ev := events[idx]
			priceCum[idx] = cumPrice
			volumeCum[idx] = cumVolume

			// Split factor S means an N-for-1 split: post-split shares
			// are S times pre-split shares. So pre-split prices need
			// to be divided by S to be expressed post-split, i.e.
			// multiplied by 1/S. Equivalently, going backwards from
			// post-split to pre-split, multiply prices by 1/S.
			if ev.splitFa > 0 && ev.splitFa != 1 {
				cumPrice /= ev.splitFa
				cumVolume *= ev.splitFa
			}

			// Dividend D paid on a price P_prev causes the ex-date
			// price to drop by D. To express pre-dividend prices in
			// post-dividend basis, multiply by (1 - D/P_prev).
			if ev.divid > 0 && ev.closeP > 0 {
				prevClose := ev.closeP + ev.divid
				if prevClose > 0 {
					cumPrice *= 1 - ev.divid/prevClose
				}
			}
		}

		priceMap := make(map[int64]float64, len(events))
		volumeMap := make(map[int64]float64, len(events))

		for idx, ev := range events {
			sec := ev.date.Unix()
			priceMap[sec] = priceCum[idx]
			volumeMap[sec] = volumeCum[idx]
		}

		priceFactor[figi] = priceMap
		volumeFactor[figi] = volumeMap
	}

	return priceFactor, volumeFactor, nil
}

// lookupFactorAt returns the cumulative adjustment factor at unix time
// sec, applying the factor of the most recent event at or after sec.
// Prices recorded before any event are multiplied by the latest tier
// factor; prices after the latest event have factor 1.
func lookupFactorAt(factors map[int64]float64, sec int64) float64 {
	if len(factors) == 0 {
		return 1
	}

	// Find the smallest event time strictly greater than sec. Its
	// factor is what applies to a quote at sec.
	var (
		bestSec int64
		bestVal = math.NaN()
	)

	for evSec, factor := range factors {
		if evSec > sec && (math.IsNaN(bestVal) || evSec < bestSec) {
			bestSec = evSec
			bestVal = factor
		}
	}

	if math.IsNaN(bestVal) {
		return 1
	}

	return bestVal
}

// IntradayFetch is the entry point used by the engine for intraday
// requests. It acts as a thin shim over fetchMinuteBars that constructs
// the internal intradayRequest from a DataRequest plus the optional
// time-of-day filter. The engine routes here when the request's lookback
// indicates an intraday access pattern.
func (p *PVDataProvider) IntradayFetch(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	start, end time.Time,
	timesOfDay []TimeOfDay,
) (*DataFrame, error) {
	figis := make([]string, 0, len(assets))
	for _, aa := range assets {
		if aa.AssetType == asset.AssetTypeFRED {
			continue
		}

		figis = append(figis, aa.CompositeFigi)
	}

	type colKey struct {
		figi   string
		metric Metric
	}

	colData := make(map[colKey]map[int64]float64)
	timeSet := make(map[int64]time.Time)

	ensureCol := func(figi string, m Metric) map[int64]float64 {
		ck := colKey{figi, m}
		if existing, ok := colData[ck]; ok {
			return existing
		}

		fresh := make(map[int64]float64)
		colData[ck] = fresh

		return fresh
	}

	if err := p.fetchMinuteBars(ctx, intradayRequest{
		figis:      figis,
		metrics:    metrics,
		start:      start,
		end:        end,
		timesOfDay: timesOfDay,
	}, ensureCol, timeSet); err != nil {
		return nil, err
	}

	times := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		times = append(times, t)
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	if len(times) == 0 {
		empty, dfErr := NewDataFrame(nil, nil, nil, Tick, nil)
		if dfErr != nil {
			return nil, fmt.Errorf("pvdata: empty intraday DataFrame: %w", dfErr)
		}

		return empty, nil
	}

	timeIdx := make(map[int64]int, len(times))
	for i, t := range times {
		timeIdx[t.Unix()] = i
	}

	numTimes := len(times)
	numAssets := len(assets)
	numMetrics := len(metrics)

	slab := make([]float64, numTimes*numAssets*numMetrics)
	for i := range slab {
		slab[i] = math.NaN()
	}

	mIdx := make(map[Metric]int, numMetrics)
	for i, m := range metrics {
		mIdx[m] = i
	}

	aIdx := make(map[string]int, numAssets)
	for i, aa := range assets {
		aIdx[aa.CompositeFigi] = i
	}

	for ck, vals := range colData {
		ai, found := aIdx[ck.figi]
		if !found {
			continue
		}

		mi, found := mIdx[ck.metric]
		if !found {
			continue
		}

		colStart := (ai*numMetrics + mi) * numTimes

		for sec, value := range vals {
			ti, ok := timeIdx[sec]
			if !ok {
				continue
			}

			slab[colStart+ti] = value
		}
	}

	df, err := NewDataFrame(times, assets, metrics, Tick,
		SlabToColumns(slab, numAssets*numMetrics, numTimes))
	if err != nil {
		return nil, fmt.Errorf("pvdata: build intraday DataFrame: %w", err)
	}

	return df, nil
}
