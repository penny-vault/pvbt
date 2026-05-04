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
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/rs/zerolog"
)

// Compile-time interface checks.
var _ BatchProvider = (*PVDataProvider)(nil)
var _ AssetProvider = (*PVDataProvider)(nil)
var _ RatingProvider = (*PVDataProvider)(nil)
var _ IndexProvider = (*PVDataProvider)(nil)
var _ interface{ Dimension() string } = (*PVDataProvider)(nil)
var _ FundamentalsByDateKeyProvider = (*PVDataProvider)(nil)

// scanPgxAsset scans a full asset row from a pgx result set. All metadata
// columns are nullable in the view, so we scan into pointers and fall back
// to zero values.
func scanPgxAsset(scanner interface{ Scan(dest ...any) error }) (asset.Asset, error) {
	var (
		aa                        asset.Asset
		name, assetType, exchange *string
		sector, industry, cik     *string
		sicCode                   *int
		listed, delisted          *time.Time
	)

	if err := scanner.Scan(
		&aa.CompositeFigi, &aa.Ticker,
		&name, &assetType, &exchange,
		&sector, &industry, &sicCode, &cik,
		&listed, &delisted,
	); err != nil {
		return asset.Asset{}, err
	}

	if name != nil {
		aa.Name = *name
	}

	if assetType != nil {
		aa.AssetType = asset.AssetType(*assetType)
	}

	if exchange != nil {
		aa.PrimaryExchange = asset.NormalizeExchange(*exchange)
	}

	if sector != nil {
		aa.Sector = asset.Sector(*sector)
	}

	if industry != nil {
		aa.Industry = asset.Industry(*industry)
	}

	if sicCode != nil {
		aa.SICCode = *sicCode
	}

	if cik != nil {
		aa.CIK = *cik
	}

	if listed != nil {
		aa.Listed = *listed
	}

	if delisted != nil {
		aa.Delisted = *delisted
	}

	return aa, nil
}

// pvdataConfig is the subset of ~/.pvdata.toml we care about.
type pvdataConfig struct {
	DB struct {
		URL string `toml:"url"`
	} `toml:"db"`
}

// PVDataProvider is a BatchProvider that reads from a pv-data
// PostgreSQL database through the canonical preferred views.
type PVDataProvider struct {
	pool      *pgxpool.Pool
	ownsPool  bool
	dimension string
	indexes   map[string]*indexState
}

// PVDataOption configures a PVDataProvider.
type PVDataOption func(*pvdataOptions)

type pvdataOptions struct {
	dimension  string
	configFile string
}

// WithDimension sets the fundamental dimension filter (default "ARQ").
func WithDimension(dim string) PVDataOption {
	return func(o *pvdataOptions) { o.dimension = dim }
}

// WithConfigFile overrides the default config file path (~/.pvdata.toml).
func WithConfigFile(path string) PVDataOption {
	return func(o *pvdataOptions) { o.configFile = path }
}

// SetDimension updates the fundamental dimension filter at runtime.
// Valid values: "ARQ", "ARY", "ART", "MRQ", "MRY", "MRT".
func (p *PVDataProvider) SetDimension(dim string) {
	p.dimension = dim
}

// Dimension returns the current fundamental dimension filter.
func (p *PVDataProvider) Dimension() string {
	return p.dimension
}

// NewPVDataProvider creates a provider that reads from a pv-data database.
// If pool is nil the provider reads ~/.pvdata.toml (or the path set via
// WithConfigFile) for the connection URL and creates its own pool.
func NewPVDataProvider(pool *pgxpool.Pool, opts ...PVDataOption) (*PVDataProvider, error) {
	options := pvdataOptions{
		dimension: "ARQ",
	}
	for _, fn := range opts {
		fn(&options)
	}

	ownsPool := false

	if pool == nil {
		cfgPath := options.configFile
		if cfgPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("pvdata: determine home directory: %w", err)
			}

			cfgPath = filepath.Join(home, ".pvdata.toml")
		}

		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("pvdata: read config %s: %w", cfgPath, err)
		}

		var cfg pvdataConfig
		if err := toml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("pvdata: parse config %s: %w", cfgPath, err)
		}

		if cfg.DB.URL == "" {
			return nil, fmt.Errorf("pvdata: no db.url in %s", cfgPath)
		}

		pool, err = pgxpool.New(context.Background(), cfg.DB.URL)
		if err != nil {
			return nil, fmt.Errorf("pvdata: connect to database: %w", err)
		}

		ownsPool = true
	}

	return &PVDataProvider{
		pool:      pool,
		ownsPool:  ownsPool,
		dimension: options.dimension,
	}, nil
}

// Provides returns all metrics that PVDataProvider can supply.
func (p *PVDataProvider) Provides() []Metric {
	metrics := make([]Metric, 0, len(metricView))
	for m := range metricView {
		metrics = append(metrics, m)
	}

	return metrics
}

// LookupAsset resolves a ticker to an Asset using the assets view.
// FRED-namespaced tickers (e.g. "FRED:DGS3MO") are resolved synthetically
// because economic indicators do not have rows in the assets table.
func (p *PVDataProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	if asset.IsFREDTicker(ticker) {
		return asset.NewFREDAsset(ticker), nil
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	row := conn.QueryRow(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets
		 WHERE ticker = $1 AND active = true LIMIT 1`,
		ticker,
	)

	foundAsset, scanErr := scanPgxAsset(row)
	if scanErr != nil {
		return asset.Asset{}, fmt.Errorf("pvdata: lookup asset %q: %w", ticker, scanErr)
	}

	return foundAsset, nil
}

// Assets returns all known assets from the database.
func (p *PVDataProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets ORDER BY ticker`)
	if err != nil {
		return nil, fmt.Errorf("query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanPgxAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}

// Fetch retrieves data for the requested assets, metrics, and time range.
func (p *PVDataProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().
		Int("assets", len(req.Assets)).
		Int("metrics", len(req.Metrics)).
		Time("start", req.Start).
		Time("end", req.End).
		Msg("PVDataProvider.Fetch")

	// group requested metrics by view
	viewMetrics := make(map[string][]Metric)

	for _, metric := range req.Metrics {
		v, ok := metricView[metric]
		if !ok {
			continue
		}

		viewMetrics[v] = append(viewMetrics[v], metric)
	}

	// Split assets by source: FRED economic indicators are queried separately
	// from the economic_indicators view; everything else queries the standard
	// eod/metrics/fundamentals views.
	figis := make([]string, 0, len(req.Assets))

	var fredAssets []asset.Asset

	for _, aa := range req.Assets {
		if aa.AssetType == asset.AssetTypeFRED {
			fredAssets = append(fredAssets, aa)

			continue
		}

		figis = append(figis, aa.CompositeFigi)
	}

	// accumulate timestamps and per-column data keyed by Unix seconds.
	// We use int64 keys because time.Time equality in Go compares Location
	// pointers, making it unsuitable as a map key for times from different
	// LoadLocation calls.
	type colKey struct {
		figi   string
		metric Metric
	}

	colData := make(map[colKey]map[int64]float64)
	timeSet := make(map[int64]time.Time)

	ensureCol := func(figi string, m Metric) map[int64]float64 {
		columnKey := colKey{figi, m}
		if c, ok := colData[columnKey]; ok {
			return c
		}

		c := make(map[int64]float64)
		colData[columnKey] = c

		return c
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	// fetch from each view that has requested metrics. Standard views are
	// only queried when there is at least one tradeable asset; FRED-only
	// requests skip them entirely.
	if len(figis) > 0 {
		if metrics, ok := viewMetrics["eod"]; ok {
			if err := p.fetchEod(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
				return nil, err
			}
		}

		if metrics, ok := viewMetrics["metrics"]; ok {
			if err := p.fetchMetrics(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
				return nil, err
			}
		}

		if metrics, ok := viewMetrics["fundamentals"]; ok {
			if err := p.fetchFundamentals(ctx, conn, figis, req.Start, req.End, metrics, ensureCol, timeSet); err != nil {
				return nil, err
			}
		}
	}

	// FRED economic indicators are sourced from the economic_indicators view.
	// Their value populates MetricClose -- callers requesting other eod
	// metrics for a FRED asset will see NaN, which is the correct signal
	// that the metric is meaningless for an economic indicator.
	if len(fredAssets) > 0 {
		if metrics, ok := viewMetrics["eod"]; ok && slices.Contains(metrics, MetricClose) {
			if err := p.fetchEconomicIndicators(ctx, conn, fredAssets, req.Start, req.End, ensureCol, timeSet); err != nil {
				return nil, err
			}
		}
	}

	// build sorted time axis
	times := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		times = append(times, t)
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	log.Debug().
		Int("unique_times", len(times)).
		Int("colData_keys", len(colData)).
		Msg("PVDataProvider.Fetch time axis")

	if len(times) == 0 {
		df, err := NewDataFrame(nil, nil, nil, req.Frequency, nil)
		if err != nil {
			return nil, fmt.Errorf("creating empty DataFrame: %w", err)
		}

		return df, nil
	}

	// build time index for fast lookup (by Unix seconds)
	timeIdx := make(map[int64]int, len(times))
	for i, t := range times {
		timeIdx[t.Unix()] = i
	}

	numTimes := len(times)

	// assemble the data slab
	data := make([]float64, numTimes*len(req.Assets)*len(req.Metrics))
	for i := range data {
		data[i] = math.NaN()
	}

	// build metric index
	mIdx := make(map[Metric]int, len(req.Metrics))
	for i, m := range req.Metrics {
		mIdx[m] = i
	}

	// build asset index
	aIdx := make(map[string]int, len(req.Assets))
	for i, a := range req.Assets {
		aIdx[a.CompositeFigi] = i
	}

	numMetrics := len(req.Metrics)

	for columnKey, vals := range colData {
		assetIdx, found := aIdx[columnKey.figi]
		if !found {
			continue
		}

		mi, found := mIdx[columnKey.metric]
		if !found {
			continue
		}

		colStart := (assetIdx*numMetrics + mi) * numTimes

		for sec, v := range vals {
			ti := timeIdx[sec]
			data[colStart+ti] = v
		}
	}

	df, err := NewDataFrame(times, req.Assets, req.Metrics, req.Frequency,
		SlabToColumns(data, len(req.Assets)*len(req.Metrics), len(times)))
	if err != nil {
		return nil, fmt.Errorf("building DataFrame: %w", err)
	}

	return df, nil
}

// Close releases resources. If the provider created its own pool it is closed.
func (p *PVDataProvider) Close() error {
	if p.ownsPool && p.pool != nil {
		p.pool.Close()
	}

	return nil
}

// FetchMarketHolidays loads all market holidays from the database.
func (p *PVDataProvider) FetchMarketHolidays(ctx context.Context) ([]tradecron.MarketHoliday, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection for holidays: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT event_date, early_close, close_time
		 FROM market_holidays
		 WHERE market = 'us'
		 ORDER BY event_date`)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query market_holidays: %w", err)
	}
	defer rows.Close()

	nyc := eodLocation

	var holidays []tradecron.MarketHoliday

	for rows.Next() {
		var (
			eventDate  time.Time
			earlyClose bool
			closeTime  time.Time
		)

		if err := rows.Scan(&eventDate, &earlyClose, &closeTime); err != nil {
			return nil, fmt.Errorf("pvdata: scan market_holidays row: %w", err)
		}

		closeHHMM := closeTime.Hour()*100 + closeTime.Minute()

		holidays = append(holidays, tradecron.MarketHoliday{
			Date:       time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 0, 0, 0, 0, nyc),
			EarlyClose: earlyClose,
			CloseTime:  closeHHMM,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pvdata: iterate market_holidays: %w", err)
	}

	return holidays, nil
}

// -- fetch helpers -----------------------------------------------------------

// fetchEconomicIndicators queries the economic_indicators view for FRED-typed
// assets. Values are written into the MetricClose column keyed by the asset's
// synthetic CompositeFigi (e.g. "FRED:DGS3MO"). The view stores annualized
// percent values (e.g. 5.25 for a 5.25% yield) -- callers are responsible for
// any unit conversion.
func (p *PVDataProvider) fetchEconomicIndicators(
	ctx context.Context,
	conn *pgxpool.Conn,
	fredAssets []asset.Asset,
	start, end time.Time,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	// Build series-name list and a series->figi map so we can attribute
	// each row back to the synthetic asset that requested it.
	series := make([]string, 0, len(fredAssets))
	figiBySeries := make(map[string]string, len(fredAssets))

	for _, aa := range fredAssets {
		seriesName := asset.FREDSeries(aa.Ticker)
		series = append(series, seriesName)
		figiBySeries[seriesName] = aa.CompositeFigi
	}

	zerolog.Ctx(ctx).Debug().
		Strs("series", series).
		Time("start", start).
		Time("end", end).
		Msg("fetchEconomicIndicators query")

	rows, err := conn.Query(ctx,
		`SELECT series, event_date, value
		 FROM economic_indicators
		 WHERE series = ANY($1) AND event_date BETWEEN $2::date AND $3::date
		 ORDER BY event_date`,
		series, start, end,
	)
	if err != nil {
		return fmt.Errorf("pvdata: query economic_indicators: %w", err)
	}
	defer rows.Close()

	rowCount := 0
	for rows.Next() {
		rowCount++

		var (
			seriesName string
			eventDate  time.Time
			value      float64
		)

		if err := rows.Scan(&seriesName, &eventDate, &value); err != nil {
			return fmt.Errorf("pvdata: scan economic_indicators row: %w", err)
		}

		figi, ok := figiBySeries[seriesName]
		if !ok {
			continue
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		ensureCol(figi, MetricClose)[sec] = value
	}

	zerolog.Ctx(ctx).Debug().Int("rows", rowCount).Msg("fetchEconomicIndicators result")

	return rows.Err()
}

func (p *PVDataProvider) fetchEod(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	zerolog.Ctx(ctx).Debug().
		Strs("figis", figis).
		Time("start", start).
		Time("end", end).
		Msg("fetchEod query")

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor
		 FROM eod
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date
		 ORDER BY event_date`,
		figis, start, end,
	)
	if err != nil {
		return fmt.Errorf("pvdata: query eod: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	type eodMapping struct {
		metric Metric
		dest   *float64
	}

	rowCount := 0
	for rows.Next() {
		rowCount++

		var (
			figi      string
			eventDate time.Time
			open, high, low, closeVal, adjClose,
			volume, dividend, splitFactor *float64
		)
		if err := rows.Scan(&figi, &eventDate, &open, &high, &low, &closeVal, &adjClose, &volume, &dividend, &splitFactor); err != nil {
			return fmt.Errorf("pvdata: scan eod row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		mappings := []eodMapping{
			{MetricOpen, open},
			{MetricHigh, high},
			{MetricLow, low},
			{MetricClose, closeVal},
			{AdjClose, adjClose},
			{Volume, volume},
			{Dividend, dividend},
			{SplitFactor, splitFactor},
		}

		for _, mp := range mappings {
			if want[mp.metric] && mp.dest != nil {
				ensureCol(figi, mp.metric)[sec] = *mp.dest
			}
		}
	}

	zerolog.Ctx(ctx).Debug().Int("rows", rowCount).Msg("fetchEod result")

	return rows.Err()
}

func (p *PVDataProvider) fetchMetrics(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	if len(metrics) == 0 {
		return nil
	}

	// boundCol ties a requested Metric to its scan type so we can convert
	// bigint columns to float64 after scanning.
	type boundCol struct {
		metric Metric
		intCol bool
	}

	sqlCols := make([]string, 0, len(metrics))
	bound := make([]boundCol, 0, len(metrics))

	for _, mm := range metrics {
		spec, ok := metricsColumn[mm]
		if !ok {
			return fmt.Errorf("pvdata: no SQL column for metrics-view metric %q", mm)
		}

		sqlCols = append(sqlCols, spec.sql)
		bound = append(bound, boundCol{metric: mm, intCol: spec.intCol})
	}

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, %s
		 FROM metrics
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
	)

	rows, err := conn.Query(ctx, query, figis, start, end)
	if err != nil {
		return fmt.Errorf("pvdata: query metrics: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	// Pre-allocate scan destinations reused across rows. We use pointers so
	// that SQL NULLs are represented as nil rather than the Go zero value.
	intVals := make([]*int64, len(bound))
	floatVals := make([]*float64, len(bound))
	scanArgs := make([]any, 0, 2+len(bound))

	for rows.Next() {
		var (
			figi      string
			eventDate time.Time
		)

		// Reset and rebuild scan args: figi, eventDate, then one dest per column.
		scanArgs = scanArgs[:0]
		scanArgs = append(scanArgs, &figi, &eventDate)

		for idx, col := range bound {
			if col.intCol {
				intVals[idx] = nil
				scanArgs = append(scanArgs, &intVals[idx])
			} else {
				floatVals[idx] = nil
				scanArgs = append(scanArgs, &floatVals[idx])
			}
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("pvdata: scan metrics row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		for idx, col := range bound {
			if !want[col.metric] {
				continue
			}

			if col.intCol {
				if intVals[idx] != nil {
					ensureCol(figi, col.metric)[sec] = float64(*intVals[idx])
				}
			} else {
				if floatVals[idx] != nil {
					ensureCol(figi, col.metric)[sec] = *floatVals[idx]
				}
			}
		}
	}

	return rows.Err()
}

func (p *PVDataProvider) fetchFundamentals(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
	start, end time.Time,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	// build the list of SQL columns we need, skipping metadata metrics
	// (date_key, report_period) which are always fetched by the fixed SELECT prefix.
	var (
		sqlCols     []string
		metricOrder []Metric
	)

	for _, metric := range metrics {
		if metric == FundamentalsDateKey || metric == FundamentalsReportPeriod {
			continue
		}

		col, ok := metricColumn[metric]
		if !ok {
			continue
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, metric)
	}

	wantDateKey := false
	wantReportPeriod := false

	for _, metric := range metrics {
		switch metric {
		case FundamentalsDateKey:
			wantDateKey = true
		case FundamentalsReportPeriod:
			wantReportPeriod = true
		}
	}

	if len(sqlCols) == 0 && !wantDateKey && !wantReportPeriod {
		return nil
	}

	cols := []string{"composite_figi", "event_date", "date_key", "report_period"}
	cols = append(cols, sqlCols...)

	query := fmt.Sprintf(
		`SELECT %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date AND dimension = $4
		 ORDER BY event_date`,
		strings.Join(cols, ", "),
	)

	rows, err := conn.Query(ctx, query, figis, start, end, p.dimension)
	if err != nil {
		return fmt.Errorf("pvdata: query fundamentals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			figi         string
			eventDate    time.Time
			dateKey      sql.NullTime
			reportPeriod sql.NullTime
		)

		vals := make([]any, 4+len(sqlCols))
		vals[0] = &figi
		vals[1] = &eventDate
		vals[2] = &dateKey
		vals[3] = &reportPeriod

		floatVals := make([]*float64, len(sqlCols))
		for idx := range sqlCols {
			vals[4+idx] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return fmt.Errorf("pvdata: scan fundamentals row: %w", err)
		}

		eventDate = eodTimestamp(eventDate)
		sec := eventDate.Unix()
		timeSet[sec] = eventDate

		for idx, mm := range metricOrder {
			if floatVals[idx] != nil {
				ensureCol(figi, mm)[sec] = *floatVals[idx]
			}
		}

		if wantDateKey && dateKey.Valid {
			ensureCol(figi, FundamentalsDateKey)[sec] = float64(dateKey.Time.Unix())
		}

		if wantReportPeriod && reportPeriod.Valid {
			ensureCol(figi, FundamentalsReportPeriod)[sec] = float64(reportPeriod.Time.Unix())
		}
	}

	return rows.Err()
}

// RatedAssets returns the set of assets whose most-recent rating (on or before t)
// from the named analyst matches filter. It returns nil, nil when filter has no
// values to match.
func (p *PVDataProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, asOfDate time.Time) ([]asset.Asset, error) {
	if len(filter.Values) == 0 {
		return nil, nil
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire connection: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx,
		`SELECT a.composite_figi, a.ticker, a.name, a.asset_type, a.primary_exchange,
		        a.sector, a.industry, a.sic_code, a.cik, a.listed, a.delisted
		 FROM ratings r
		 JOIN assets a ON a.composite_figi = r.composite_figi
		 WHERE r.analyst = $1 AND r.event_date = $2 AND r.rating = ANY($3)`,
		analyst, asOfDate, filter.Values,
	)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query rated assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		aa, scanErr := scanPgxAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("pvdata: scan rated asset: %w", scanErr)
		}

		assets = append(assets, aa)
	}

	return assets, rows.Err()
}

// IndexMembers returns the constituents of the named index at forDate. The
// returned slice is borrowed -- it is only valid for the current engine step.
// Callers that need data across steps must copy.
//
// Dates must be monotonically increasing across calls for a given index.
// The provider loads all snapshot and changelog data on the first call and
// advances an internal cursor as time progresses.
func (p *PVDataProvider) IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, []IndexConstituent, error) {
	if p.indexes == nil {
		p.indexes = make(map[string]*indexState)
	}

	state, ok := p.indexes[index]
	if !ok {
		var err error

		state, err = p.loadIndexState(ctx, index)
		if err != nil {
			return nil, nil, fmt.Errorf("pvdata: load index state for %q: %w", index, err)
		}

		p.indexes[index] = state
	}

	assets, constituents := state.Advance(forDate)

	return assets, constituents, nil
}

func (p *PVDataProvider) loadIndexState(ctx context.Context, index string) (*indexState, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Load snapshots. Each row contains a snapshot_date and a JSONB array of constituents.
	snapRows, err := conn.Query(ctx,
		`SELECT snapshot_date, constituents
		 FROM indices_snapshot
		 WHERE index_ticker = $1
		 ORDER BY snapshot_date`,
		index,
	)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer snapRows.Close()

	type constituentJSON struct {
		CompositeFigi string  `json:"composite_figi"`
		Ticker        string  `json:"ticker"`
		Weight        float64 `json:"weight"`
	}

	var (
		snapshots []IndexSnapshotEntry
		figis     []string
	)

	for snapRows.Next() {
		var (
			dt          time.Time
			rawConstits []byte
		)

		if err := snapRows.Scan(&dt, &rawConstits); err != nil {
			return nil, fmt.Errorf("scan snapshot row: %w", err)
		}

		var constits []constituentJSON
		if err := json.Unmarshal(rawConstits, &constits); err != nil {
			return nil, fmt.Errorf("unmarshal constituents for %s: %w", dt.Format("2006-01-02"), err)
		}

		entry := IndexSnapshotEntry{Date: dt}

		for _, cc := range constits {
			entry.Members = append(entry.Members, IndexConstituent{
				Asset: asset.Asset{
					CompositeFigi: cc.CompositeFigi,
					Ticker:        cc.Ticker,
				},
				Weight: cc.Weight,
			})
			figis = append(figis, cc.CompositeFigi)
		}

		snapshots = append(snapshots, entry)
	}

	if err := snapRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate snapshots: %w", err)
	}

	// Load changelog.
	logRows, err := conn.Query(ctx,
		`SELECT event_date, composite_figi, ticker, action, weight
		 FROM indices_changelog
		 WHERE index_ticker = $1
		 ORDER BY event_date, composite_figi`,
		index,
	)
	if err != nil {
		return nil, fmt.Errorf("query changelog: %w", err)
	}
	defer logRows.Close()

	var changelog []IndexChangeEntry

	for logRows.Next() {
		var (
			entry         IndexChangeEntry
			compositeFigi string
			ticker        string
		)

		if err := logRows.Scan(&entry.Date, &compositeFigi, &ticker, &entry.Action, &entry.Weight); err != nil {
			return nil, fmt.Errorf("scan changelog row: %w", err)
		}

		entry.Asset = asset.Asset{CompositeFigi: compositeFigi, Ticker: ticker}
		figis = append(figis, compositeFigi)

		changelog = append(changelog, entry)
	}

	if err := logRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changelog: %w", err)
	}

	// Enrich every stub asset with full metadata (Sector, Industry, Name,
	// etc.) from the assets table so strategies can gate on classification.
	// Missing figis keep their stub Asset; we log them as a single warning so
	// strategies still see the constituent rather than silently dropping the
	// whole index.
	assetsByFigi, err := p.loadAssetsByFigi(ctx, conn, figis)
	if err != nil {
		return nil, fmt.Errorf("load index member metadata for %q: %w", index, err)
	}

	if missing := enrichIndexState(snapshots, changelog, assetsByFigi); len(missing) > 0 {
		const sampleSize = 10

		sample := missing
		if len(sample) > sampleSize {
			sample = sample[:sampleSize]
		}

		zerolog.Ctx(ctx).Warn().
			Str("index", index).
			Int("missing_count", len(missing)).
			Strs("sample_tickers", sample).
			Msg("index constituents have no row in assets table; classification fields will be empty")
	}

	return NewIndexState(snapshots, changelog), nil
}

// loadAssetsByFigi fetches full asset rows for the given composite_figis from
// the assets view and returns them keyed by composite_figi. Used by
// loadIndexState to enrich index constituents with metadata that the
// indices_snapshot/indices_changelog tables do not carry. Duplicates in figis
// are tolerated by the SQL ANY() clause; the result map deduplicates them.
func (p *PVDataProvider) loadAssetsByFigi(
	ctx context.Context,
	conn *pgxpool.Conn,
	figis []string,
) (map[string]asset.Asset, error) {
	if len(figis) == 0 {
		return map[string]asset.Asset{}, nil
	}

	rows, err := conn.Query(ctx,
		`SELECT composite_figi, ticker, name, asset_type, primary_exchange,
		        sector, industry, sic_code, cik, listed, delisted
		 FROM assets WHERE composite_figi = ANY($1)`,
		figis,
	)
	if err != nil {
		return nil, fmt.Errorf("query assets by figi: %w", err)
	}
	defer rows.Close()

	assets := make(map[string]asset.Asset, len(figis))

	for rows.Next() {
		aa, scanErr := scanPgxAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan asset by figi: %w", scanErr)
		}

		assets[aa.CompositeFigi] = aa
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assets by figi: %w", err)
	}

	return assets, nil
}

// enrichIndexState replaces the stub Asset records on snapshot constituents
// and changelog entries with the full asset.Asset (Sector, Industry, Name,
// etc.) from assetsByFigi. Enrichment is best-effort: figis missing from
// assetsByFigi keep their stub Asset (CompositeFigi + Ticker only) so the
// strategy still sees the constituent and can fall back on its own policy
// when classification is empty. The returned tickers are the ones whose
// metadata could not be filled in, deduplicated and sorted, so the caller
// can log a single aggregated warning per index load.
func enrichIndexState(
	snapshots []IndexSnapshotEntry,
	changelog []IndexChangeEntry,
	assetsByFigi map[string]asset.Asset,
) []string {
	missing := make(map[string]string) // composite_figi -> ticker

	for ii := range snapshots {
		for jj := range snapshots[ii].Members {
			stub := snapshots[ii].Members[jj].Asset

			full, ok := assetsByFigi[stub.CompositeFigi]
			if !ok {
				missing[stub.CompositeFigi] = stub.Ticker

				continue
			}

			snapshots[ii].Members[jj].Asset = full
		}
	}

	for ii := range changelog {
		stub := changelog[ii].Asset

		full, ok := assetsByFigi[stub.CompositeFigi]
		if !ok {
			missing[stub.CompositeFigi] = stub.Ticker

			continue
		}

		changelog[ii].Asset = full
	}

	if len(missing) == 0 {
		return nil
	}

	tickers := make([]string, 0, len(missing))
	for _, ticker := range missing {
		tickers = append(tickers, ticker)
	}

	sort.Strings(tickers)

	return tickers
}

// RatingHistory returns the initial rating state just before start and all
// rating changes in [start, end] for the given analyst.
// -- metric mappings ---------------------------------------------------------

// metricView maps each Metric to the database view it comes from.
var metricView = map[Metric]string{
	// eod view
	MetricOpen:  "eod",
	MetricHigh:  "eod",
	MetricLow:   "eod",
	MetricClose: "eod",
	AdjClose:    "eod",
	Volume:      "eod",
	Dividend:    "eod",
	SplitFactor: "eod",

	// metrics view
	MarketCap:       "metrics",
	EnterpriseValue: "metrics",
	PE:              "metrics",
	PB:              "metrics",
	PS:              "metrics",
	EVtoEBIT:        "metrics",
	EVtoEBITDA:      "metrics",
	ForwardPE:       "metrics",
	PEG:             "metrics",
	PriceToCashFlow: "metrics",
	Beta:            "metrics",

	// fundamentals view
	Revenue:                             "fundamentals",
	CostOfRevenue:                       "fundamentals",
	GrossProfit:                         "fundamentals",
	OperatingExpenses:                   "fundamentals",
	OperatingIncome:                     "fundamentals",
	EBIT:                                "fundamentals",
	EBITDA:                              "fundamentals",
	EBT:                                 "fundamentals",
	ConsolidatedIncome:                  "fundamentals",
	NetIncome:                           "fundamentals",
	NetIncomeCommonStock:                "fundamentals",
	EarningsPerShare:                    "fundamentals",
	EPSDiluted:                          "fundamentals",
	InterestExpense:                     "fundamentals",
	IncomeTaxExpense:                    "fundamentals",
	RandDExpenses:                       "fundamentals",
	SGAExpense:                          "fundamentals",
	ShareBasedCompensation:              "fundamentals",
	DividendsPerShare:                   "fundamentals",
	NetLossIncomeDiscontinuedOperations: "fundamentals",
	NetIncomeToNonControllingInterests:  "fundamentals",
	PreferredDividendsImpact:            "fundamentals",
	TotalAssets:                         "fundamentals",
	CurrentAssets:                       "fundamentals",
	AssetsNonCurrent:                    "fundamentals",
	AverageAssets:                       "fundamentals",
	CashAndEquivalents:                  "fundamentals",
	Inventory:                           "fundamentals",
	Receivables:                         "fundamentals",
	Investments:                         "fundamentals",
	InvestmentsCurrent:                  "fundamentals",
	InvestmentsNonCur:                   "fundamentals",
	Intangibles:                         "fundamentals",
	PPENet:                              "fundamentals",
	TaxAssets:                           "fundamentals",
	TotalLiabilities:                    "fundamentals",
	CurrentLiabilities:                  "fundamentals",
	LiabilitiesNonCurrent:               "fundamentals",
	TotalDebt:                           "fundamentals",
	DebtCurrent:                         "fundamentals",
	DebtNonCurrent:                      "fundamentals",
	Payables:                            "fundamentals",
	DeferredRevenue:                     "fundamentals",
	Deposits:                            "fundamentals",
	TaxLiabilities:                      "fundamentals",
	Equity:                              "fundamentals",
	EquityAvg:                           "fundamentals",
	AccumulatedOtherComprehensiveIncome: "fundamentals",
	AccumulatedRetainedEarningsDeficit:  "fundamentals",
	FreeCashFlow:                        "fundamentals",
	NetCashFlow:                         "fundamentals",
	NetCashFlowFromOperations:           "fundamentals",
	NetCashFlowFromInvesting:            "fundamentals",
	NetCashFlowFromFinancing:            "fundamentals",
	NetCashFlowBusiness:                 "fundamentals",
	NetCashFlowCommon:                   "fundamentals",
	NetCashFlowDebt:                     "fundamentals",
	NetCashFlowDividend:                 "fundamentals",
	NetCashFlowInvest:                   "fundamentals",
	NetCashFlowFx:                       "fundamentals",
	CapitalExpenditure:                  "fundamentals",
	DepreciationAmortization:            "fundamentals",
	BookValue:                           "fundamentals",
	FreeCashFlowPerShare:                "fundamentals",
	SalesPerShare:                       "fundamentals",
	TangibleAssetsBookValuePerShare:     "fundamentals",
	ShareFactor:                         "fundamentals",
	SharesBasic:                         "fundamentals",
	WeightedAverageShares:               "fundamentals",
	WeightedAverageSharesDiluted:        "fundamentals",
	FundamentalPrice:                    "fundamentals",
	PE1:                                 "fundamentals",
	PS1:                                 "fundamentals",
	FxUSD:                               "fundamentals",
	GrossMargin:                         "fundamentals",
	EBITDAMargin:                        "fundamentals",
	ProfitMargin:                        "fundamentals",
	ROA:                                 "fundamentals",
	ROE:                                 "fundamentals",
	ROIC:                                "fundamentals",
	ReturnOnSales:                       "fundamentals",
	AssetTurnover:                       "fundamentals",
	CurrentRatio:                        "fundamentals",
	DebtToEquity:                        "fundamentals",
	DividendYield:                       "fundamentals",
	PayoutRatio:                         "fundamentals",
	InvestedCapital:                     "fundamentals",
	InvestedCapitalAvg:                  "fundamentals",
	TangibleAssetValue:                  "fundamentals",
	WorkingCapital:                      "fundamentals",
	MarketCapFundamental:                "fundamentals",
	FundamentalsDateKey:                 "fundamentals",
	FundamentalsReportPeriod:            "fundamentals",
}

// IsFundamental reports whether the given metric is sourced from the
// fundamentals table. Fundamental metrics are sparse (quarterly) and
// require forward-fill when merged with daily price data.
func IsFundamental(metric Metric) bool {
	return metricView[metric] == "fundamentals"
}

// metricColumn maps fundamental Metrics to their SQL column names.
var metricColumn = map[Metric]string{
	Revenue:                             "revenues",
	CostOfRevenue:                       "cost_of_revenue",
	GrossProfit:                         "gross_profit",
	OperatingExpenses:                   "operating_expenses",
	OperatingIncome:                     "operating_income",
	EBIT:                                "ebit",
	EBITDA:                              "ebitda",
	EBT:                                 "ebt",
	ConsolidatedIncome:                  "consolidated_income",
	NetIncome:                           "net_income",
	NetIncomeCommonStock:                "net_income_common_stock",
	EarningsPerShare:                    "eps",
	EPSDiluted:                          "eps_diluted",
	InterestExpense:                     "interest_expense",
	IncomeTaxExpense:                    "income_tax_expense",
	RandDExpenses:                       "r_and_d_expenses",
	SGAExpense:                          "selling_general_and_administrative_expense",
	ShareBasedCompensation:              "share_based_compensation",
	DividendsPerShare:                   "dividends_per_basic_common_share",
	NetLossIncomeDiscontinuedOperations: "net_loss_income_discontinued_operations",
	NetIncomeToNonControllingInterests:  "net_income_to_non_controlling_interests",
	PreferredDividendsImpact:            "preferred_dividends_income_statement_impact",
	TotalAssets:                         "total_assets",
	CurrentAssets:                       "current_assets",
	AssetsNonCurrent:                    "assets_non_current",
	AverageAssets:                       "average_assets",
	CashAndEquivalents:                  "cash_and_equivalents",
	Inventory:                           "inventory",
	Receivables:                         "receivables",
	Investments:                         "investments",
	InvestmentsCurrent:                  "investments_current",
	InvestmentsNonCur:                   "investments_non_current",
	Intangibles:                         "intangibles",
	PPENet:                              "property_plant_and_equipment_net",
	TaxAssets:                           "tax_assets",
	TotalLiabilities:                    "total_liabilities",
	CurrentLiabilities:                  "current_liabilities",
	LiabilitiesNonCurrent:               "liabilities_non_current",
	TotalDebt:                           "total_debt",
	DebtCurrent:                         "debt_current",
	DebtNonCurrent:                      "debt_non_current",
	Payables:                            "payables",
	DeferredRevenue:                     "deferred_revenue",
	Deposits:                            "deposits",
	TaxLiabilities:                      "tax_liabilities",
	Equity:                              "equity",
	EquityAvg:                           "equity_avg",
	AccumulatedOtherComprehensiveIncome: "accumulated_other_comprehensive_income",
	AccumulatedRetainedEarningsDeficit:  "accumulated_retained_earnings_deficit",
	FreeCashFlow:                        "free_cash_flow",
	NetCashFlow:                         "net_cash_flow",
	NetCashFlowFromOperations:           "net_cash_flow_from_operations",
	NetCashFlowFromInvesting:            "net_cash_flow_from_investing",
	NetCashFlowFromFinancing:            "net_cash_flow_from_financing",
	NetCashFlowBusiness:                 "net_cash_flow_business",
	NetCashFlowCommon:                   "net_cash_flow_common",
	NetCashFlowDebt:                     "net_cash_flow_debt",
	NetCashFlowDividend:                 "net_cash_flow_dividend",
	NetCashFlowInvest:                   "net_cash_flow_invest",
	NetCashFlowFx:                       "net_cash_flow_fx",
	CapitalExpenditure:                  "capital_expenditure",
	DepreciationAmortization:            "depreciation_amortization_and_accretion",
	BookValue:                           "book_value_per_share",
	FreeCashFlowPerShare:                "free_cash_flow_per_share",
	SalesPerShare:                       "sales_per_share",
	TangibleAssetsBookValuePerShare:     "tangible_assets_book_value_per_share",
	ShareFactor:                         "share_factor",
	SharesBasic:                         "shares_basic",
	WeightedAverageShares:               "weighted_average_shares",
	WeightedAverageSharesDiluted:        "weighted_average_shares_diluted",
	FundamentalPrice:                    "price",
	PE1:                                 "pe1",
	PS1:                                 "ps1",
	FxUSD:                               "fx_usd",
	GrossMargin:                         "gross_margin",
	EBITDAMargin:                        "ebitda_margin",
	ProfitMargin:                        "profit_margin",
	ROA:                                 "roa",
	ROE:                                 "roe",
	ROIC:                                "roic",
	ReturnOnSales:                       "return_on_sales",
	AssetTurnover:                       "asset_turnover",
	CurrentRatio:                        "current_ratio",
	DebtToEquity:                        "debt_to_equity_ratio",
	DividendYield:                       "dividend_yield",
	PayoutRatio:                         "payout_ratio",
	InvestedCapital:                     "invested_capital",
	InvestedCapitalAvg:                  "invested_capital_average",
	TangibleAssetValue:                  "tangible_asset_value",
	WorkingCapital:                      "working_capital",
	MarketCapFundamental:                "market_capitalization",
	FundamentalsDateKey:                 "date_key",
	FundamentalsReportPeriod:            "report_period",
}

// metricsColumn maps metrics-view Metrics to their SQL column names and
// whether the column is an integer (bigint) type. Integer columns are scanned
// as *int64 and converted to float64.
var metricsColumn = map[Metric]struct {
	sql    string
	intCol bool
}{
	MarketCap:       {sql: "market_cap", intCol: true},
	EnterpriseValue: {sql: "ev", intCol: true},
	PE:              {sql: "pe", intCol: false},
	PB:              {sql: "pb", intCol: false},
	PS:              {sql: "ps", intCol: false},
	EVtoEBIT:        {sql: "ev_ebit", intCol: false},
	EVtoEBITDA:      {sql: "ev_ebitda", intCol: false},
	ForwardPE:       {sql: "pe_forward", intCol: false},
	PEG:             {sql: "peg", intCol: false},
	PriceToCashFlow: {sql: "price_to_cash_flow", intCol: false},
	Beta:            {sql: "beta", intCol: false},
}

func metricSet(ms []Metric) map[Metric]bool {
	s := make(map[Metric]bool, len(ms))
	for _, m := range ms {
		s[m] = true
	}

	return s
}

// eodLocation is cached to avoid repeated time.LoadLocation calls
// (which read timezone data from disk each time).
var eodLocation = mustLoadLocation("America/New_York")

// eodTimestamp converts a database date to the market close timestamp (16:00 Eastern).
func eodTimestamp(timestamp time.Time) time.Time {
	return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 16, 0, 0, 0, eodLocation)
}

// FetchFundamentalsByDateKey implements FundamentalsByDateKeyProvider.
func (p *PVDataProvider) FetchFundamentalsByDateKey(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	dateKey time.Time,
	dimension string,
	maxEventDate time.Time,
) (*DataFrame, error) {
	figis := make([]string, len(assets))
	for idx, aa := range assets {
		figis[idx] = aa.CompositeFigi
	}

	var (
		sqlCols     []string
		metricOrder []Metric
	)

	wantDateKey := false
	wantReportPeriod := false

	for _, mm := range metrics {
		switch mm {
		case FundamentalsDateKey:
			wantDateKey = true
			continue
		case FundamentalsReportPeriod:
			wantReportPeriod = true
			continue
		}

		col, ok := metricColumn[mm]
		if !ok {
			return nil, fmt.Errorf("pvdata: no SQL column for fundamental metric %q", mm)
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, mm)
	}

	cols := []string{"composite_figi", "event_date", "date_key", "report_period"}
	cols = append(cols, sqlCols...)

	// DISTINCT ON (composite_figi) keeps the most recent event_date per
	// asset, which matters for MR dimensions where a single (figi,
	// date_key) tuple may appear multiple times due to restatements.
	query := fmt.Sprintf(
		`SELECT DISTINCT ON (composite_figi) %s
		 FROM fundamentals
		 WHERE composite_figi = ANY($1)
		   AND date_key = $2::date
		   AND dimension = $3
		   AND event_date <= $4::date
		 ORDER BY composite_figi, event_date DESC`,
		strings.Join(cols, ", "),
	)

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("pvdata: acquire conn for FetchFundamentalsByDateKey: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, query, figis, dateKey, dimension, maxEventDate)
	if err != nil {
		return nil, fmt.Errorf("pvdata: query fundamentals by date_key: %w", err)
	}
	defer rows.Close()

	// Build a per-figi value map: figi -> metric -> float64.
	perFigi := make(map[string]map[Metric]float64, len(assets))

	for rows.Next() {
		var (
			figi         string
			eventDate    time.Time
			rowDateKey   time.Time
			reportPeriod sql.NullTime
		)

		vals := make([]any, 4+len(sqlCols))
		vals[0] = &figi
		vals[1] = &eventDate
		vals[2] = &rowDateKey
		vals[3] = &reportPeriod

		floatVals := make([]*float64, len(sqlCols))
		for idx := range sqlCols {
			vals[4+idx] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return nil, fmt.Errorf("pvdata: scan fundamentals by date_key row: %w", err)
		}

		bucket, ok := perFigi[figi]
		if !ok {
			bucket = make(map[Metric]float64, len(metrics))
			perFigi[figi] = bucket
		}

		for idx, mm := range metricOrder {
			if floatVals[idx] != nil {
				bucket[mm] = *floatVals[idx]
			}
		}

		if wantDateKey {
			bucket[FundamentalsDateKey] = float64(rowDateKey.Unix())
		}

		if wantReportPeriod && reportPeriod.Valid {
			bucket[FundamentalsReportPeriod] = float64(reportPeriod.Time.Unix())
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pvdata: iterate fundamentals by date_key rows: %w", err)
	}

	// Assemble a single-row DataFrame at dateKey.
	times := []time.Time{dateKey}
	columns := make([][]float64, len(assets)*len(metrics))

	for aIdx, aa := range assets {
		bucket := perFigi[aa.CompositeFigi]

		for mIdx, mm := range metrics {
			val := math.NaN()

			if bucket != nil {
				if vv, ok := bucket[mm]; ok {
					val = vv
				}
			}

			columns[aIdx*len(metrics)+mIdx] = []float64{val}
		}
	}

	return NewDataFrame(times, assets, metrics, Daily, columns)
}
