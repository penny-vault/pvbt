package data

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/bytedance/sonic"
	"math"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/tradecron"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ BatchProvider                 = (*SnapshotRecorder)(nil)
	_ AssetProvider                 = (*SnapshotRecorder)(nil)
	_ IndexProvider                 = (*SnapshotRecorder)(nil)
	_ RatingProvider                = (*SnapshotRecorder)(nil)
	_ HolidayProvider               = (*SnapshotRecorder)(nil)
	_ FundamentalsByDateKeyProvider = (*SnapshotRecorder)(nil)
)

// SnapshotRecorderConfig holds the providers to wrap.
type SnapshotRecorderConfig struct {
	BatchProvider  BatchProvider
	AssetProvider  AssetProvider
	IndexProvider  IndexProvider  // optional
	RatingProvider RatingProvider // optional
}

// SnapshotRecorder wraps real data providers, delegates every call, and
// writes the results to a SQLite snapshot database.
type SnapshotRecorder struct {
	db             *sql.DB
	batchProvider  BatchProvider
	assetProvider  AssetProvider
	indexProvider  IndexProvider
	ratingProvider RatingProvider
}

// NewSnapshotRecorder opens (or creates) the SQLite file at path, creates
// the snapshot schema, and returns a recorder ready to wrap provider calls.
func NewSnapshotRecorder(path string, cfg SnapshotRecorderConfig) (*SnapshotRecorder, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("snapshot recorder: open database: %w", err)
	}

	if err := CreateSnapshotSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("snapshot recorder: create schema: %w", err)
	}

	return &SnapshotRecorder{
		db:             db,
		batchProvider:  cfg.BatchProvider,
		assetProvider:  cfg.AssetProvider,
		indexProvider:  cfg.IndexProvider,
		ratingProvider: cfg.RatingProvider,
	}, nil
}

// Close closes the underlying SQLite database.
func (r *SnapshotRecorder) Close() error {
	return r.db.Close()
}

// FetchMarketHolidays delegates to the underlying provider and records the
// holidays in the snapshot database.
func (r *SnapshotRecorder) FetchMarketHolidays(ctx context.Context) ([]tradecron.MarketHoliday, error) {
	// Find a HolidayProvider among the wrapped providers.
	var hp HolidayProvider

	if provider, ok := r.batchProvider.(HolidayProvider); ok {
		hp = provider
	} else if provider, ok := r.assetProvider.(HolidayProvider); ok {
		hp = provider
	}

	if hp == nil {
		return nil, nil
	}

	holidays, err := hp.FetchMarketHolidays(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.recordMarketHolidays(holidays); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record market holidays: %w", err)
	}

	return holidays, nil
}

// recordMarketHolidays writes the market holiday calendar to the snapshot.
func (r *SnapshotRecorder) recordMarketHolidays(holidays []tradecron.MarketHoliday) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO market_holidays (event_date, early_close, close_time) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, holiday := range holidays {
		earlyClose := 0
		if holiday.EarlyClose {
			earlyClose = 1
		}

		dateStr := holiday.Date.Format("2006-01-02")
		if _, err := stmt.Exec(dateStr, earlyClose, holiday.CloseTime); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- AssetProvider --

// Assets delegates to the inner AssetProvider and records the results.
func (r *SnapshotRecorder) Assets(ctx context.Context) ([]asset.Asset, error) {
	if r.assetProvider == nil {
		return nil, nil
	}

	assets, err := r.assetProvider.Assets(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record assets: %w", err)
	}

	return assets, nil
}

// LookupAsset delegates to the inner AssetProvider and records the result.
func (r *SnapshotRecorder) LookupAsset(ctx context.Context, ticker string) (asset.Asset, error) {
	if r.assetProvider == nil {
		return asset.Asset{}, fmt.Errorf("snapshot recorder: no asset provider configured")
	}

	result, err := r.assetProvider.LookupAsset(ctx, ticker)
	if err != nil {
		return asset.Asset{}, err
	}

	if err := r.recordAssets([]asset.Asset{result}); err != nil {
		return asset.Asset{}, fmt.Errorf("snapshot recorder: record asset: %w", err)
	}

	return result, nil
}

func (r *SnapshotRecorder) recordAssets(assets []asset.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO assets
		(composite_figi, ticker, name, asset_type, primary_exchange, sector, industry, sic_code, cik, listed, delisted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, aa := range assets {
		listedStr := ""
		if !aa.Listed.IsZero() {
			listedStr = aa.Listed.Format("2006-01-02")
		}

		delistedStr := ""
		if !aa.Delisted.IsZero() {
			delistedStr = aa.Delisted.Format("2006-01-02")
		}

		if _, err := stmt.Exec(
			aa.CompositeFigi, aa.Ticker, aa.Name,
			string(aa.AssetType), string(aa.PrimaryExchange),
			string(aa.Sector), string(aa.Industry),
			aa.SICCode, aa.CIK,
			listedStr, delistedStr,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- BatchProvider --

// Provides delegates to the inner BatchProvider.
func (r *SnapshotRecorder) Provides() []Metric {
	if r.batchProvider == nil {
		return nil
	}

	return r.batchProvider.Provides()
}

// Fetch delegates to the inner BatchProvider and records the results.
func (r *SnapshotRecorder) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	if r.batchProvider == nil {
		return nil, fmt.Errorf("snapshot recorder: no batch provider configured")
	}

	df, err := r.batchProvider.Fetch(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := r.recordDataFrame(df, req.Metrics); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record data: %w", err)
	}

	return df, nil
}

func (r *SnapshotRecorder) recordDataFrame(df *DataFrame, requestedMetrics []Metric) error {
	if df == nil || len(df.times) == 0 {
		return nil
	}

	// Ensure all assets in the frame are in the assets table.
	if err := r.recordAssets(df.assets); err != nil {
		return err
	}

	// Group requested metrics by view.
	viewMetrics := make(map[string][]Metric)

	for _, metric := range requestedMetrics {
		view, ok := metricView[metric]
		if !ok {
			continue
		}

		viewMetrics[view] = append(viewMetrics[view], metric)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	if metrics, ok := viewMetrics["eod"]; ok {
		if err := r.recordEod(tx, df, metrics); err != nil {
			return err
		}
	}

	if metrics, ok := viewMetrics["metrics"]; ok {
		if err := r.recordMetrics(tx, df, metrics); err != nil {
			return err
		}
	}

	if metrics, ok := viewMetrics["fundamentals"]; ok {
		if err := r.recordFundamentals(tx, df, metrics); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *SnapshotRecorder) recordEod(tx *sql.Tx, df *DataFrame, metrics []Metric) error {
	want := metricSet(metrics)
	numMetrics := len(df.metrics)

	// Build metric index for the DataFrame.
	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	// getValue returns the value for a metric, or nil if the metric was
	// not requested or the value is NaN. This ensures unrequested metrics
	// are stored as NULL rather than zero.
	type nullableFloat = any

	for assetIdx, currentAsset := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format("2006-01-02")

			getValue := func(metric Metric) nullableFloat {
				if !want[metric] {
					return nil
				}

				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}

				val := df.columns[assetIdx*numMetrics+mi][timeIdx]
				if math.IsNaN(val) {
					return nil
				}

				return val
			}

			_, err := tx.Exec(
				`INSERT INTO eod
				 (composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(composite_figi, event_date) DO UPDATE SET
				   open         = COALESCE(excluded.open, open),
				   high         = COALESCE(excluded.high, high),
				   low          = COALESCE(excluded.low, low),
				   close        = COALESCE(excluded.close, close),
				   adj_close    = COALESCE(excluded.adj_close, adj_close),
				   volume       = COALESCE(excluded.volume, volume),
				   dividend     = COALESCE(excluded.dividend, dividend),
				   split_factor = COALESCE(excluded.split_factor, split_factor)`,
				currentAsset.CompositeFigi, dateStr,
				getValue(MetricOpen), getValue(MetricHigh), getValue(MetricLow),
				getValue(MetricClose), getValue(AdjClose), getValue(Volume),
				getValue(Dividend), getValue(SplitFactor),
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SnapshotRecorder) recordMetrics(tx *sql.Tx, df *DataFrame, _ []Metric) error {
	numMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	for assetIdx, currentAsset := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format("2006-01-02")

			getValue := func(metric Metric) any {
				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}

				val := df.columns[assetIdx*numMetrics+mi][timeIdx]
				if math.IsNaN(val) {
					return nil
				}

				return val
			}

			_, err := tx.Exec(
				`INSERT INTO metrics
				 (composite_figi, event_date, market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda, pe_forward, peg, price_to_cash_flow, beta)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(composite_figi, event_date) DO UPDATE SET
				   market_cap        = COALESCE(excluded.market_cap, market_cap),
				   ev                = COALESCE(excluded.ev, ev),
				   pe                = COALESCE(excluded.pe, pe),
				   pb                = COALESCE(excluded.pb, pb),
				   ps                = COALESCE(excluded.ps, ps),
				   ev_ebit           = COALESCE(excluded.ev_ebit, ev_ebit),
				   ev_ebitda         = COALESCE(excluded.ev_ebitda, ev_ebitda),
				   pe_forward        = COALESCE(excluded.pe_forward, pe_forward),
				   peg               = COALESCE(excluded.peg, peg),
				   price_to_cash_flow = COALESCE(excluded.price_to_cash_flow, price_to_cash_flow),
				   beta              = COALESCE(excluded.beta, beta)`,
				currentAsset.CompositeFigi, dateStr,
				getValue(MarketCap), getValue(EnterpriseValue),
				getValue(PE), getValue(PB), getValue(PS),
				getValue(EVtoEBIT), getValue(EVtoEBITDA),
				getValue(ForwardPE), getValue(PEG),
				getValue(PriceToCashFlow), getValue(Beta),
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SnapshotRecorder) recordFundamentals(tx *sql.Tx, df *DataFrame, metrics []Metric) error {
	numDFMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	dateKeyDFIdx, hasDateKey := mIdx[FundamentalsDateKey]
	reportPeriodDFIdx, hasReportPeriod := mIdx[FundamentalsReportPeriod]

	// Build sorted column list from the metrics we have data for.
	var (
		colNames   []string
		colMetrics []Metric
	)

	for _, metric := range metrics {
		// Skip metadata metrics; they map to structural columns already declared in the INSERT.
		if metric == FundamentalsDateKey || metric == FundamentalsReportPeriod {
			continue
		}

		colName, ok := metricColumn[metric]
		if !ok {
			continue
		}

		colNames = append(colNames, colName)
		colMetrics = append(colMetrics, metric)
	}

	if len(colNames) == 0 {
		return nil
	}

	placeholders := make([]string, 5+len(colNames))
	placeholders[0] = "?" // composite_figi
	placeholders[1] = "?" // event_date
	placeholders[2] = "?" // date_key
	placeholders[3] = "?" // report_period
	placeholders[4] = "?" // dimension

	for idx := range colNames {
		placeholders[5+idx] = "?"
	}

	// Build ON CONFLICT clause for upsert.
	upsertCols := make([]string, len(colNames))
	for idx, col := range colNames {
		upsertCols[idx] = fmt.Sprintf("%s = COALESCE(excluded.%s, %s)", col, col, col)
	}

	query := fmt.Sprintf(
		"INSERT INTO fundamentals (composite_figi, event_date, date_key, report_period, dimension, %s) VALUES (%s) ON CONFLICT(composite_figi, event_date, dimension) DO UPDATE SET %s",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(upsertCols, ", "),
	)

	dimension := "ARQ"

	if dp, ok := r.batchProvider.(interface{ Dimension() string }); ok {
		if dim := dp.Dimension(); dim != "" {
			dimension = dim
		}
	}

	for assetIdx, aa := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format("2006-01-02")

			args := make([]any, 5+len(colMetrics))
			args[0] = aa.CompositeFigi
			args[1] = dateStr

			args[2] = nil

			if hasDateKey {
				raw := df.columns[assetIdx*numDFMetrics+dateKeyDFIdx][timeIdx]
				if !math.IsNaN(raw) {
					args[2] = time.Unix(int64(raw), 0).UTC().Format("2006-01-02")
				}
			}

			args[3] = nil

			if hasReportPeriod {
				raw := df.columns[assetIdx*numDFMetrics+reportPeriodDFIdx][timeIdx]
				if !math.IsNaN(raw) {
					args[3] = time.Unix(int64(raw), 0).UTC().Format("2006-01-02")
				}
			}

			args[4] = dimension

			for idx, metric := range colMetrics {
				mi, ok := mIdx[metric]
				if !ok {
					args[5+idx] = nil
					continue
				}

				val := df.columns[assetIdx*numDFMetrics+mi][timeIdx]
				if math.IsNaN(val) {
					args[5+idx] = nil
				} else {
					args[5+idx] = val
				}
			}

			if _, err := tx.Exec(query, args...); err != nil {
				return err
			}
		}
	}

	return nil
}

// FetchFundamentalsByDateKey delegates to a wrapped FundamentalsByDateKeyProvider
// and records the returned values so SnapshotProvider can replay the same call.
// Returns an error when no wrapped provider implements the interface.
//
// The recorder writes one row per asset at event_date = dateKey (the reporting
// period itself). This is synthetic: the true filing date is consumed inside
// the provider's DISTINCT ON ordering and not exposed on the returned
// DataFrame. Because replay queries filter with event_date <= maxEventDate
// and every sensible caller passes a maxEventDate on or after the reporting
// period, the recorded row always matches. A replay under a custom as-of date
// returns the same value the recording saw; it cannot reconstruct filings
// that the recording never observed.
func (r *SnapshotRecorder) FetchFundamentalsByDateKey(
	ctx context.Context,
	assets []asset.Asset,
	metrics []Metric,
	dateKey time.Time,
	dimension string,
	maxEventDate time.Time,
) (*DataFrame, error) {
	provider := r.fundamentalsByDateKeyProvider()
	if provider == nil {
		return nil, fmt.Errorf("snapshot recorder: no wrapped provider supports FundamentalsByDateKey")
	}

	df, err := provider.FetchFundamentalsByDateKey(ctx, assets, metrics, dateKey, dimension, maxEventDate)
	if err != nil {
		return nil, err
	}

	if err := r.recordFundamentalsByDateKey(df, metrics, dateKey, dimension); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record fundamentals by date_key: %w", err)
	}

	return df, nil
}

func (r *SnapshotRecorder) fundamentalsByDateKeyProvider() FundamentalsByDateKeyProvider {
	if provider, ok := r.batchProvider.(FundamentalsByDateKeyProvider); ok {
		return provider
	}

	if provider, ok := r.assetProvider.(FundamentalsByDateKeyProvider); ok {
		return provider
	}

	return nil
}

func (r *SnapshotRecorder) recordFundamentalsByDateKey(df *DataFrame, metrics []Metric, dateKey time.Time, dimension string) error {
	if df == nil || len(df.assets) == 0 {
		return nil
	}

	if err := r.recordAssets(df.assets); err != nil {
		return err
	}

	numDFMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	reportPeriodDFIdx, hasReportPeriod := mIdx[FundamentalsReportPeriod]

	var (
		colNames   []string
		colMetrics []Metric
	)

	for _, metric := range metrics {
		if metric == FundamentalsDateKey || metric == FundamentalsReportPeriod {
			continue
		}

		colName, ok := metricColumn[metric]
		if !ok {
			continue
		}

		colNames = append(colNames, colName)
		colMetrics = append(colMetrics, metric)
	}

	if len(colNames) == 0 {
		return nil
	}

	placeholders := make([]string, 5+len(colNames))
	for idx := range placeholders {
		placeholders[idx] = "?"
	}

	upsertCols := make([]string, len(colNames))
	for idx, col := range colNames {
		upsertCols[idx] = fmt.Sprintf("%s = COALESCE(excluded.%s, %s)", col, col, col)
	}

	query := fmt.Sprintf(
		"INSERT INTO fundamentals (composite_figi, event_date, date_key, report_period, dimension, %s) VALUES (%s) ON CONFLICT(composite_figi, event_date, dimension) DO UPDATE SET %s",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(upsertCols, ", "),
	)

	effectiveDimension := dimension
	if effectiveDimension == "" {
		effectiveDimension = "ARQ"
	}

	dateKeyStr := dateKey.UTC().Format("2006-01-02")

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	for assetIdx, aa := range df.assets {
		args := make([]any, 5+len(colMetrics))
		args[0] = aa.CompositeFigi
		args[1] = dateKeyStr
		args[2] = dateKeyStr
		args[3] = nil

		if hasReportPeriod {
			raw := df.columns[assetIdx*numDFMetrics+reportPeriodDFIdx][0]
			if !math.IsNaN(raw) {
				args[3] = time.Unix(int64(raw), 0).UTC().Format("2006-01-02")
			}
		}

		args[4] = effectiveDimension

		for idx, metric := range colMetrics {
			mi, ok := mIdx[metric]
			if !ok {
				args[5+idx] = nil
				continue
			}

			val := df.columns[assetIdx*numDFMetrics+mi][0]
			if math.IsNaN(val) {
				args[5+idx] = nil
			} else {
				args[5+idx] = val
			}
		}

		if _, err := tx.Exec(query, args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- IndexProvider --

// IndexMembers delegates to the inner IndexProvider and records the results.
func (r *SnapshotRecorder) IndexMembers(ctx context.Context, index string, forDate time.Time) ([]asset.Asset, []IndexConstituent, error) {
	if r.indexProvider == nil {
		return nil, nil, nil
	}

	assets, constituents, err := r.indexProvider.IndexMembers(ctx, index, forDate)
	if err != nil {
		return nil, nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, nil, fmt.Errorf("snapshot recorder: record index member assets: %w", err)
	}

	if err := r.recordIndexMembers(index, forDate, constituents); err != nil {
		return nil, nil, fmt.Errorf("snapshot recorder: record index members: %w", err)
	}

	return assets, constituents, nil
}

func (r *SnapshotRecorder) recordIndexMembers(index string, forDate time.Time, constituents []IndexConstituent) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO index_members (index_name, event_date, composite_figi, ticker, weight) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := forDate.Format("2006-01-02")
	for _, ic := range constituents {
		if _, err := stmt.Exec(index, dateStr, ic.Asset.CompositeFigi, ic.Asset.Ticker, ic.Weight); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- RatingProvider --

// RatedAssets delegates to the inner RatingProvider and records the results.
func (r *SnapshotRecorder) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, forDate time.Time) ([]asset.Asset, error) {
	if r.ratingProvider == nil {
		return nil, nil
	}

	assets, err := r.ratingProvider.RatedAssets(ctx, analyst, filter, forDate)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record rated assets: %w", err)
	}

	if err := r.recordRatedAssets(analyst, filter, forDate, assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record ratings: %w", err)
	}

	return assets, nil
}

func (r *SnapshotRecorder) recordRatedAssets(analyst string, filter RatingFilter, forDate time.Time, assets []asset.Asset) error {
	filterJSON, err := sonic.Marshal(filter.Values)
	if err != nil {
		return fmt.Errorf("marshal filter values: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO ratings (analyst, filter_values, event_date, composite_figi, ticker) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := forDate.Format("2006-01-02")
	for _, a := range assets {
		if _, err := stmt.Exec(analyst, string(filterJSON), dateStr, a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}
