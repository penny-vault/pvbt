package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/tradecron"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ BatchProvider   = (*SnapshotProvider)(nil)
	_ AssetProvider   = (*SnapshotProvider)(nil)
	_ IndexProvider   = (*SnapshotProvider)(nil)
	_ RatingProvider  = (*SnapshotProvider)(nil)
	_ HolidayProvider = (*SnapshotProvider)(nil)
)

// SnapshotProvider replays data from a snapshot SQLite database.
type SnapshotProvider struct {
	db *sql.DB
}

// NewSnapshotProvider opens the snapshot database at path in read-only mode.
func NewSnapshotProvider(path string) (*SnapshotProvider, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: open database: %w", err)
	}

	// Set read-only mode via pragma (modernc.org/sqlite does not support
	// ?mode=ro in the DSN).
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("snapshot provider: set read-only: %w", err)
	}

	return &SnapshotProvider{db: db}, nil
}

// Close closes the database connection.
func (p *SnapshotProvider) Close() error {
	return p.db.Close()
}

// snapshotDateFormat is the canonical format for dates in snapshot databases.
const snapshotDateFormat = "2006-01-02"

// snapshotLocation is the timezone used when parsing date-only strings from
// snapshot databases. The PV data provider returns timestamps at 4pm Eastern
// (market close). Parsing snapshot dates in the same timezone ensures
// downstream timestamp matching works correctly.
var snapshotLocation = mustLoadLocation("America/New_York")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic("snapshot provider: load timezone " + name + ": " + err.Error())
	}

	return loc
}

// parseSnapshotDate parses a date string from the snapshot database.
// Date-only strings are parsed at 4pm Eastern (market close) to match
// the timestamps produced by the PV data provider. Legacy RFC3339
// strings are parsed as-is for backward compatibility.
func parseSnapshotDate(dateStr string) (time.Time, error) {
	parsed, err := time.ParseInLocation(snapshotDateFormat, dateStr, snapshotLocation)
	if err == nil {
		// Set to 4pm Eastern (market close) to match PV provider timestamps.
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(),
			16, 0, 0, 0, snapshotLocation), nil
	}

	return time.Parse(time.RFC3339, dateStr)
}

// FetchMarketHolidays loads market holidays from the snapshot database.
func (p *SnapshotProvider) FetchMarketHolidays(ctx context.Context) ([]tradecron.MarketHoliday, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT event_date, early_close, close_time FROM market_holidays ORDER BY event_date")
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query market holidays: %w", err)
	}
	defer rows.Close()

	var holidays []tradecron.MarketHoliday

	for rows.Next() {
		var (
			dateStr    string
			earlyClose int
			closeTime  int
		)

		if err := rows.Scan(&dateStr, &earlyClose, &closeTime); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan market holiday: %w", err)
		}

		parsedDate, err := parseSnapshotDate(dateStr)
		if err != nil {
			return nil, fmt.Errorf("snapshot provider: parse holiday date: %w", err)
		}

		holidays = append(holidays, tradecron.MarketHoliday{
			Date:       parsedDate,
			EarlyClose: earlyClose != 0,
			CloseTime:  closeTime,
		})
	}

	return holidays, rows.Err()
}

// -- AssetProvider --

func (p *SnapshotProvider) Assets(ctx context.Context) ([]asset.Asset, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT composite_figi, ticker FROM assets ORDER BY ticker")
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query assets: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan asset: %w", err)
		}

		assets = append(assets, a)
	}

	return assets, rows.Err()
}

func (p *SnapshotProvider) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	var foundAsset asset.Asset

	err := p.db.QueryRowContext(ctx,
		"SELECT composite_figi, ticker FROM assets WHERE ticker = ? LIMIT 1", ticker,
	).Scan(&foundAsset.CompositeFigi, &foundAsset.Ticker)
	if err != nil {
		return asset.Asset{}, fmt.Errorf("snapshot provider: lookup asset %q: %w", ticker, err)
	}

	return foundAsset, nil
}

// -- BatchProvider --

func (p *SnapshotProvider) Provides() []Metric {
	tables := []string{"eod", "metrics", "fundamentals"}

	var result []Metric

	for _, table := range tables {
		var count int
		if err := p.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count == 0 {
			continue
		}

		for metric, view := range metricView {
			if view == table {
				result = append(result, metric)
			}
		}
	}

	// Sort for deterministic output.
	slices.Sort(result)

	return result
}

func (p *SnapshotProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	// Group requested metrics by view.
	viewMetrics := make(map[string][]Metric)

	for _, metric := range req.Metrics {
		view, ok := metricView[metric]
		if !ok {
			continue
		}

		viewMetrics[view] = append(viewMetrics[view], metric)
	}

	figis := make([]string, len(req.Assets))
	for idx, a := range req.Assets {
		figis[idx] = a.CompositeFigi
	}

	type colKey struct {
		figi   string
		metric Metric
	}

	colData := make(map[colKey]map[int64]float64)
	timeSet := make(map[int64]time.Time)

	ensureCol := func(figi string, m Metric) map[int64]float64 {
		key := colKey{figi, m}
		if c, ok := colData[key]; ok {
			return c
		}

		c := make(map[int64]float64)
		colData[key] = c

		return c
	}

	startStr := req.Start.Format("2006-01-02")
	endStr := req.End.Format("2006-01-02")

	if metrics, ok := viewMetrics["eod"]; ok {
		if err := p.fetchEod(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	if metrics, ok := viewMetrics["metrics"]; ok {
		if err := p.fetchMetrics(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	if metrics, ok := viewMetrics["fundamentals"]; ok {
		if err := p.fetchFundamentals(ctx, figis, startStr, endStr, metrics, ensureCol, timeSet); err != nil {
			return nil, err
		}
	}

	// Build sorted time axis.
	times := make([]time.Time, 0, len(timeSet))
	for _, t := range timeSet {
		times = append(times, t)
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	if len(times) == 0 {
		return NewDataFrame(nil, nil, nil, req.Frequency, nil)
	}

	timeIdx := make(map[int64]int, len(times))
	for idx, t := range times {
		timeIdx[t.Unix()] = idx
	}

	numTimes := len(times)
	numMetrics := len(req.Metrics)

	slab := make([]float64, numTimes*len(req.Assets)*numMetrics)
	for idx := range slab {
		slab[idx] = math.NaN()
	}

	aIdx := make(map[string]int, len(req.Assets))
	for idx, a := range req.Assets {
		aIdx[a.CompositeFigi] = idx
	}

	mIdx := make(map[Metric]int, numMetrics)
	for idx, m := range req.Metrics {
		mIdx[m] = idx
	}

	for key, vals := range colData {
		ai, ok := aIdx[key.figi]
		if !ok {
			continue
		}

		mi, ok := mIdx[key.metric]
		if !ok {
			continue
		}

		colStart := (ai*numMetrics + mi) * numTimes

		for sec, val := range vals {
			ti := timeIdx[sec]
			slab[colStart+ti] = val
		}
	}

	return NewDataFrame(times, req.Assets, req.Metrics, req.Frequency,
		SlabToColumns(slab, len(req.Assets)*len(req.Metrics), len(times)))
}

func (p *SnapshotProvider) fetchEod(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	placeholders := make([]string, len(figis))

	args := make([]any, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}

	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor
		 FROM eod
		 WHERE composite_figi IN (%s) AND substr(event_date, 1, 10) BETWEEN ? AND ?
		 ORDER BY event_date`,
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query eod: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	type eodCol struct {
		metric Metric
	}

	columns := []eodCol{
		{MetricOpen}, {MetricHigh}, {MetricLow}, {MetricClose},
		{AdjClose}, {Volume}, {Dividend}, {SplitFactor},
	}

	vals := make([]sql.NullFloat64, len(columns))

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		scanArgs := make([]any, 0, 2+len(columns))

		scanArgs = append(scanArgs, &figi, &dateStr)
		for idx := range columns {
			scanArgs = append(scanArgs, &vals[idx])
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("snapshot provider: scan eod: %w", err)
		}

		parsedTime, err := parseSnapshotDate(dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse eod date: %w", err)
		}

		sec := parsedTime.Unix()
		timeSet[sec] = parsedTime

		for idx, col := range columns {
			if !want[col.metric] {
				continue
			}

			if vals[idx].Valid {
				ensureCol(figi, col.metric)[sec] = vals[idx].Float64
			}
		}
	}

	return rows.Err()
}

func (p *SnapshotProvider) fetchMetrics(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	placeholders := make([]string, len(figis))

	args := make([]any, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}

	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date,
		        market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda,
		        pe_forward, peg, price_to_cash_flow, beta
		 FROM metrics
		 WHERE composite_figi IN (%s) AND substr(event_date, 1, 10) BETWEEN ? AND ?
		 ORDER BY event_date`,
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query metrics: %w", err)
	}
	defer rows.Close()

	want := metricSet(metrics)

	type metricsCol struct {
		metric Metric
		intCol bool
	}

	columns := []metricsCol{
		{MarketCap, true}, {EnterpriseValue, true},
		{PE, false}, {PB, false}, {PS, false},
		{EVtoEBIT, false}, {EVtoEBITDA, false},
		{ForwardPE, false}, {PEG, false},
		{PriceToCashFlow, false}, {Beta, false},
	}

	intVals := make([]sql.NullInt64, len(columns))
	floatVals := make([]sql.NullFloat64, len(columns))

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		scanArgs := make([]any, 0, 2+len(columns))
		scanArgs = append(scanArgs, &figi, &dateStr)

		for idx, col := range columns {
			if col.intCol {
				scanArgs = append(scanArgs, &intVals[idx])
			} else {
				scanArgs = append(scanArgs, &floatVals[idx])
			}
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("snapshot provider: scan metrics: %w", err)
		}

		parsedTime, err := parseSnapshotDate(dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse metrics date: %w", err)
		}

		sec := parsedTime.Unix()
		timeSet[sec] = parsedTime

		for idx, col := range columns {
			if !want[col.metric] {
				continue
			}

			if col.intCol {
				if intVals[idx].Valid {
					ensureCol(figi, col.metric)[sec] = float64(intVals[idx].Int64)
				}
			} else {
				if floatVals[idx].Valid {
					ensureCol(figi, col.metric)[sec] = floatVals[idx].Float64
				}
			}
		}
	}

	return rows.Err()
}

func (p *SnapshotProvider) fetchFundamentals(
	ctx context.Context,
	figis []string,
	startStr, endStr string,
	metrics []Metric,
	ensureCol func(string, Metric) map[int64]float64,
	timeSet map[int64]time.Time,
) error {
	var (
		sqlCols     []string
		metricOrder []Metric
	)

	for _, metric := range metrics {
		col, ok := metricColumn[metric]
		if !ok {
			continue
		}

		sqlCols = append(sqlCols, col)
		metricOrder = append(metricOrder, metric)
	}

	if len(sqlCols) == 0 {
		return nil
	}

	placeholders := make([]string, len(figis))

	args := make([]any, len(figis)+2)
	for idx, figi := range figis {
		placeholders[idx] = "?"
		args[idx] = figi
	}

	args[len(figis)] = startStr
	args[len(figis)+1] = endStr

	// Add dimension filter -- the recorder stores "ARQ" as the default.
	args = append(args, "ARQ")

	query := fmt.Sprintf(
		`SELECT composite_figi, event_date, %s
		 FROM fundamentals
		 WHERE composite_figi IN (%s) AND substr(event_date, 1, 10) BETWEEN ? AND ? AND dimension = ?
		 ORDER BY event_date`,
		strings.Join(sqlCols, ", "),
		strings.Join(placeholders, ","),
	)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("snapshot provider: query fundamentals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			figi    string
			dateStr string
		)

		vals := make([]any, len(sqlCols)+2)
		vals[0] = &figi
		vals[1] = &dateStr

		floatVals := make([]sql.NullFloat64, len(sqlCols))
		for idx := range sqlCols {
			vals[idx+2] = &floatVals[idx]
		}

		if err := rows.Scan(vals...); err != nil {
			return fmt.Errorf("snapshot provider: scan fundamentals: %w", err)
		}

		parsedTime, err := parseSnapshotDate(dateStr)
		if err != nil {
			return fmt.Errorf("snapshot provider: parse fundamentals date: %w", err)
		}

		sec := parsedTime.Unix()
		timeSet[sec] = parsedTime

		for idx, metric := range metricOrder {
			if floatVals[idx].Valid {
				ensureCol(figi, metric)[sec] = floatVals[idx].Float64
			}
		}
	}

	return rows.Err()
}

// -- IndexProvider --

func (p *SnapshotProvider) IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, error) {
	dateStr := forDate.Format("2006-01-02")

	rows, err := p.db.QueryContext(ctx,
		"SELECT composite_figi, ticker FROM index_members WHERE index_name = ? AND event_date = ?",
		index, dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query index members: %w", err)
	}
	defer rows.Close()

	var members []asset.Asset

	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan index member: %w", err)
		}

		members = append(members, a)
	}

	return members, rows.Err()
}

// -- RatingProvider --

func (p *SnapshotProvider) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, forDate time.Time) ([]asset.Asset, error) {
	filterJSON, err := json.Marshal(filter.Values)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: marshal filter: %w", err)
	}

	dateStr := forDate.Format("2006-01-02")

	rows, err := p.db.QueryContext(ctx,
		"SELECT composite_figi, ticker FROM ratings WHERE analyst = ? AND filter_values = ? AND event_date = ?",
		analyst, string(filterJSON), dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot provider: query ratings: %w", err)
	}
	defer rows.Close()

	var assets []asset.Asset

	for rows.Next() {
		var a asset.Asset
		if err := rows.Scan(&a.CompositeFigi, &a.Ticker); err != nil {
			return nil, fmt.Errorf("snapshot provider: scan rated asset: %w", err)
		}

		assets = append(assets, a)
	}

	return assets, rows.Err()
}
