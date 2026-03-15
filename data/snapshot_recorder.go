package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"

	_ "modernc.org/sqlite"
)

// Compile-time interface checks.
var (
	_ BatchProvider  = (*SnapshotRecorder)(nil)
	_ AssetProvider  = (*SnapshotRecorder)(nil)
	_ IndexProvider  = (*SnapshotRecorder)(nil)
	_ RatingProvider = (*SnapshotRecorder)(nil)
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
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO assets (composite_figi, ticker) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range assets {
		if _, err := stmt.Exec(a.CompositeFigi, a.Ticker); err != nil {
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
	defer tx.Rollback()

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
	numTimes := len(df.times)
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

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			getValue := func(metric Metric) nullableFloat {
				if !want[metric] {
					return nil
				}
				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}
				colStart := (assetIdx*numMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					return nil
				}
				return val
			}

			_, err := tx.Exec(
				`INSERT OR REPLACE INTO eod
				 (composite_figi, event_date, open, high, low, close, adj_close, volume, dividend, split_factor)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				a.CompositeFigi, dateStr,
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
	numTimes := len(df.times)
	numMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			getValue := func(metric Metric) any {
				mi, ok := mIdx[metric]
				if !ok {
					return nil
				}
				colStart := (assetIdx*numMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					return nil
				}
				return val
			}

			_, err := tx.Exec(
				`INSERT OR REPLACE INTO metrics
				 (composite_figi, event_date, market_cap, ev, pe, pb, ps, ev_ebit, ev_ebitda, pe_forward, peg, price_to_cash_flow, beta)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				a.CompositeFigi, dateStr,
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
	numTimes := len(df.times)
	numDFMetrics := len(df.metrics)

	mIdx := make(map[Metric]int, len(df.metrics))
	for idx, metric := range df.metrics {
		mIdx[metric] = idx
	}

	// Build sorted column list from the metrics we have data for.
	var colNames []string
	var colMetrics []Metric

	for _, metric := range metrics {
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

	placeholders := make([]string, 3+len(colNames))
	placeholders[0] = "?"
	placeholders[1] = "?"
	placeholders[2] = "?"
	for idx := range colNames {
		placeholders[3+idx] = "?"
	}

	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO fundamentals (composite_figi, event_date, dimension, %s) VALUES (%s)",
		strings.Join(colNames, ", "),
		strings.Join(placeholders, ", "),
	)

	for assetIdx, a := range df.assets {
		for timeIdx, timestamp := range df.times {
			dateStr := timestamp.Format(time.RFC3339)

			args := make([]any, 3+len(colMetrics))
			args[0] = a.CompositeFigi
			args[1] = dateStr
			args[2] = "ARQ" // default dimension

			for idx, metric := range colMetrics {
				mi, ok := mIdx[metric]
				if !ok {
					args[3+idx] = nil
					continue
				}
				colStart := (assetIdx*numDFMetrics + mi) * numTimes
				val := df.data[colStart+timeIdx]
				if math.IsNaN(val) {
					args[3+idx] = nil
				} else {
					args[3+idx] = val
				}
			}

			if _, err := tx.Exec(query, args...); err != nil {
				return err
			}
		}
	}

	return nil
}

// -- IndexProvider --

// IndexMembers delegates to the inner IndexProvider and records the results.
func (r *SnapshotRecorder) IndexMembers(ctx context.Context, index string, t time.Time) ([]asset.Asset, error) {
	if r.indexProvider == nil {
		return nil, nil
	}

	members, err := r.indexProvider.IndexMembers(ctx, index, t)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(members); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record index member assets: %w", err)
	}

	if err := r.recordIndexMembers(index, t, members); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record index members: %w", err)
	}

	return members, nil
}

func (r *SnapshotRecorder) recordIndexMembers(index string, t time.Time, members []asset.Asset) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO index_members (index_name, event_date, composite_figi, ticker) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := t.Format(time.RFC3339)
	for _, a := range members {
		if _, err := stmt.Exec(index, dateStr, a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// -- RatingProvider --

// RatedAssets delegates to the inner RatingProvider and records the results.
func (r *SnapshotRecorder) RatedAssets(ctx context.Context, analyst string, filter RatingFilter, t time.Time) ([]asset.Asset, error) {
	if r.ratingProvider == nil {
		return nil, nil
	}

	assets, err := r.ratingProvider.RatedAssets(ctx, analyst, filter, t)
	if err != nil {
		return nil, err
	}

	if err := r.recordAssets(assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record rated assets: %w", err)
	}

	if err := r.recordRatedAssets(analyst, filter, t, assets); err != nil {
		return nil, fmt.Errorf("snapshot recorder: record ratings: %w", err)
	}

	return assets, nil
}

func (r *SnapshotRecorder) recordRatedAssets(analyst string, filter RatingFilter, t time.Time, assets []asset.Asset) error {
	filterJSON, err := json.Marshal(filter.Values)
	if err != nil {
		return fmt.Errorf("marshal filter values: %w", err)
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT OR IGNORE INTO ratings (analyst, filter_values, event_date, composite_figi, ticker) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	dateStr := t.Format(time.RFC3339)
	for _, a := range assets {
		if _, err := stmt.Exec(analyst, string(filterJSON), dateStr, a.CompositeFigi, a.Ticker); err != nil {
			return err
		}
	}

	return tx.Commit()
}
