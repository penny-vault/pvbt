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

package portfolio

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	_ "modernc.org/sqlite"
)

const schemaVersion = "3"

const dateFormat = "2006-01-02"

const createSchema = `
CREATE TABLE metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE perf_data (
    date      TEXT NOT NULL,
    metric    TEXT NOT NULL,
    value     REAL NOT NULL
);

CREATE TABLE transactions (
    date          TEXT NOT NULL,
    type          TEXT NOT NULL,
    ticker        TEXT,
    figi          TEXT,
    quantity      REAL,
    price         REAL,
    amount        REAL,
    qualified     INTEGER,
    justification TEXT
);

CREATE TABLE holdings (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    quantity     REAL NOT NULL,
    avg_cost     REAL NOT NULL,
    market_value REAL NOT NULL
);

CREATE TABLE tax_lots (
    asset_ticker TEXT NOT NULL,
    asset_figi   TEXT NOT NULL,
    date         TEXT NOT NULL,
    quantity     REAL NOT NULL,
    price        REAL NOT NULL
);

CREATE TABLE metrics (
    date   TEXT NOT NULL,
    name   TEXT NOT NULL,
    window TEXT NOT NULL,
    value  REAL
);

CREATE INDEX idx_metrics_date ON metrics(date);
CREATE INDEX idx_metrics_name ON metrics(name);
CREATE INDEX idx_transactions_date ON transactions(date);

CREATE TABLE annotations (
    timestamp INTEGER NOT NULL,
    key       TEXT NOT NULL,
    value     TEXT NOT NULL
);

CREATE INDEX idx_annotations_timestamp ON annotations(timestamp);
`

// transactionTypeToString maps a TransactionType to its lowercase string
// representation for storage.
func transactionTypeToString(txnType TransactionType) string {
	switch txnType {
	case BuyTransaction:
		return "buy"
	case SellTransaction:
		return "sell"
	case DividendTransaction:
		return "dividend"
	case FeeTransaction:
		return "fee"
	case DepositTransaction:
		return "deposit"
	case WithdrawalTransaction:
		return "withdrawal"
	default:
		return fmt.Sprintf("unknown(%d)", int(txnType))
	}
}

// stringToTransactionType maps a lowercase string back to a TransactionType.
func stringToTransactionType(str string) (TransactionType, error) {
	switch str {
	case "buy":
		return BuyTransaction, nil
	case "sell":
		return SellTransaction, nil
	case "dividend":
		return DividendTransaction, nil
	case "fee":
		return FeeTransaction, nil
	case "deposit":
		return DepositTransaction, nil
	case "withdrawal":
		return WithdrawalTransaction, nil
	default:
		return 0, fmt.Errorf("unknown transaction type: %q", str)
	}
}

// ToSQLite serializes the account state to a SQLite database at the given path.
// The database is created fresh; if a file already exists at path it is
// overwritten.
func (a *Account) ToSQLite(path string) error {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer database.Close()

	dbTx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer dbTx.Rollback() //nolint:errcheck

	// Create schema.
	if _, err := dbTx.Exec(createSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Write metadata.
	if err := a.writeMetadata(dbTx); err != nil {
		return err
	}

	// Write perf data.
	if err := a.writePerfData(dbTx); err != nil {
		return err
	}

	// Write transactions.
	if err := a.writeTransactions(dbTx); err != nil {
		return err
	}

	// Write holdings.
	if err := a.writeHoldings(dbTx); err != nil {
		return err
	}

	// Write tax lots.
	if err := a.writeTaxLots(dbTx); err != nil {
		return err
	}

	// Write metrics.
	if err := a.writeMetrics(dbTx); err != nil {
		return err
	}

	// Write annotations.
	if err := a.writeAnnotations(dbTx); err != nil {
		return err
	}

	if err := dbTx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (a *Account) writeMetadata(tx *sql.Tx) error {
	stmt, err := tx.Prepare("INSERT INTO metadata (key, value) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare metadata: %w", err)
	}
	defer stmt.Close()

	// Schema version.
	if _, err := stmt.Exec("schema_version", schemaVersion); err != nil {
		return fmt.Errorf("insert schema_version: %w", err)
	}

	// Cash.
	if _, err := stmt.Exec("cash", strconv.FormatFloat(a.cash, 'f', -1, 64)); err != nil {
		return fmt.Errorf("insert cash: %w", err)
	}

	// Perf data frequency.
	if a.perfData != nil {
		if _, err := stmt.Exec("perf_data_frequency", a.perfData.Frequency().String()); err != nil {
			return fmt.Errorf("insert perf_data_frequency: %w", err)
		}
	}

	// Benchmark identity.
	if a.benchmark != (asset.Asset{}) {
		if _, err := stmt.Exec("benchmark_ticker", a.benchmark.Ticker); err != nil {
			return fmt.Errorf("insert benchmark_ticker: %w", err)
		}

		if _, err := stmt.Exec("benchmark_figi", a.benchmark.CompositeFigi); err != nil {
			return fmt.Errorf("insert benchmark_figi: %w", err)
		}
	}

	// Risk-free identity (always DGS3MO).
	if _, err := stmt.Exec("risk_free_ticker", "DGS3MO"); err != nil {
		return fmt.Errorf("insert risk_free_ticker: %w", err)
	}

	if _, err := stmt.Exec("risk_free_figi", ""); err != nil {
		return fmt.Errorf("insert risk_free_figi: %w", err)
	}

	// User metadata.
	for k, v := range a.metadata {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("insert metadata %q: %w", k, err)
		}
	}

	return nil
}

func (a *Account) writePerfData(tx *sql.Tx) error {
	if a.perfData == nil {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO perf_data (date, metric, value) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare perf_data: %w", err)
	}
	defer stmt.Close()

	times := a.perfData.Times()
	for _, m := range a.perfData.MetricList() {
		col := a.perfData.Column(portfolioAsset, m)
		for i, v := range col {
			if !math.IsNaN(v) {
				d := times[i].Format(dateFormat)
				if _, err := stmt.Exec(d, string(m), v); err != nil {
					return fmt.Errorf("insert perf_data: %w", err)
				}
			}
		}
	}

	return nil
}

func (a *Account) writeTransactions(tx *sql.Tx) error {
	if len(a.transactions) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO transactions (date, type, ticker, figi, quantity, price, amount, qualified, justification) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare transactions: %w", err)
	}
	defer stmt.Close()

	for _, txn := range a.transactions {
		dateStr := txn.Date.Format(dateFormat)
		typStr := transactionTypeToString(txn.Type)

		qualified := 0
		if txn.Qualified {
			qualified = 1
		}

		if _, err := stmt.Exec(dateStr, typStr, txn.Asset.Ticker, txn.Asset.CompositeFigi, txn.Qty, txn.Price, txn.Amount, qualified, sql.NullString{String: txn.Justification, Valid: txn.Justification != ""}); err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}
	}

	return nil
}

func (a *Account) writeHoldings(tx *sql.Tx) error {
	if len(a.holdings) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO holdings (asset_ticker, asset_figi, quantity, avg_cost, market_value) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare holdings: %w", err)
	}
	defer stmt.Close()

	for ast, qty := range a.holdings {
		// Compute average cost from tax lots.
		var avgCost float64

		lots := a.taxLots[ast]
		if len(lots) > 0 {
			var totalCost, totalQty float64
			for _, lot := range lots {
				totalCost += lot.Price * lot.Qty
				totalQty += lot.Qty
			}

			if totalQty > 0 {
				avgCost = totalCost / totalQty
			}
		}

		// Compute market value from latest prices.
		var marketValue float64

		if a.prices != nil {
			v := a.prices.Value(ast, data.MetricClose)
			if !math.IsNaN(v) {
				marketValue = qty * v
			}
		}

		if _, err := stmt.Exec(ast.Ticker, ast.CompositeFigi, qty, avgCost, marketValue); err != nil {
			return fmt.Errorf("insert holding: %w", err)
		}
	}

	return nil
}

func (a *Account) writeTaxLots(tx *sql.Tx) error {
	if len(a.taxLots) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO tax_lots (asset_ticker, asset_figi, date, quantity, price) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare tax_lots: %w", err)
	}
	defer stmt.Close()

	for ast, lots := range a.taxLots {
		for _, lot := range lots {
			d := lot.Date.Format(dateFormat)
			if _, err := stmt.Exec(ast.Ticker, ast.CompositeFigi, d, lot.Qty, lot.Price); err != nil {
				return fmt.Errorf("insert tax_lot: %w", err)
			}
		}
	}

	return nil
}

func (a *Account) writeMetrics(tx *sql.Tx) error {
	if len(a.metrics) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO metrics (date, name, window, value) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare metrics: %w", err)
	}
	defer stmt.Close()

	for _, m := range a.metrics {
		d := m.Date.Format(dateFormat)
		if _, err := stmt.Exec(d, m.Name, m.Window, m.Value); err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
	}

	return nil
}

func (a *Account) writeAnnotations(tx *sql.Tx) error {
	if len(a.annotations) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO annotations (timestamp, key, value) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare annotations: %w", err)
	}
	defer stmt.Close()

	for _, ann := range a.annotations {
		if _, err := stmt.Exec(ann.Timestamp.Unix(), ann.Key, ann.Value); err != nil {
			return fmt.Errorf("insert annotation: %w", err)
		}
	}

	return nil
}

func (a *Account) readAnnotations(db *sql.DB) error {
	rows, err := db.Query("SELECT timestamp, key, value FROM annotations ORDER BY timestamp, key")
	if err != nil {
		return fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var unixSecs int64

		var ann Annotation

		if err := rows.Scan(&unixSecs, &ann.Key, &ann.Value); err != nil {
			return fmt.Errorf("scan annotation: %w", err)
		}

		ann.Timestamp = time.Unix(unixSecs, 0).UTC()
		a.annotations = append(a.annotations, ann)
	}

	return rows.Err()
}

// FromSQLite restores an Account from a SQLite database at the given path.
// The database must have been created by ToSQLite with schema_version "3".
// Fields that require a live broker or price DataFrame (broker, prices,
// registeredMetrics) are not restored.
func FromSQLite(path string) (*Account, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer database.Close()

	// Ping to verify the file exists and is a valid database.
	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	acct := &Account{
		holdings: make(map[asset.Asset]float64),
		taxLots:  make(map[asset.Asset][]TaxLot),
		metadata: make(map[string]string),
	}

	// Read metadata.
	if err := acct.readMetadata(database); err != nil {
		return nil, err
	}

	// Verify schema version.
	if ver := acct.metadata["schema_version"]; ver != schemaVersion {
		return nil, fmt.Errorf("unsupported schema version: %q (expected %q)", ver, schemaVersion)
	}

	// Restore cash from metadata.
	if cashStr, ok := acct.metadata["cash"]; ok {
		acct.cash, err = strconv.ParseFloat(cashStr, 64)
		if err != nil {
			return nil, fmt.Errorf("parse cash: %w", err)
		}
	}

	// Restore benchmark and risk-free asset identity.
	if ticker, ok := acct.metadata["benchmark_ticker"]; ok {
		acct.benchmark = asset.Asset{
			Ticker:        ticker,
			CompositeFigi: acct.metadata["benchmark_figi"],
		}
	}

	// Remove internal metadata keys so they don't appear in user metadata.
	delete(acct.metadata, "schema_version")
	delete(acct.metadata, "cash")
	delete(acct.metadata, "benchmark_ticker")
	delete(acct.metadata, "benchmark_figi")
	delete(acct.metadata, "risk_free_ticker")
	delete(acct.metadata, "risk_free_figi")
	delete(acct.metadata, "perf_data_frequency")

	// Read perf data.
	if err := acct.readPerfData(database); err != nil {
		return nil, err
	}

	// Read transactions.
	if err := acct.readTransactions(database); err != nil {
		return nil, err
	}

	// Read holdings.
	if err := acct.readHoldings(database); err != nil {
		return nil, err
	}

	// Read tax lots.
	if err := acct.readTaxLots(database); err != nil {
		return nil, err
	}

	// Read metrics.
	if err := acct.readMetrics(database); err != nil {
		return nil, err
	}

	// Read annotations.
	if err := acct.readAnnotations(database); err != nil {
		return nil, err
	}

	return acct, nil
}

func (a *Account) readMetadata(db *sql.DB) error {
	rows, err := db.Query("SELECT key, value FROM metadata")
	if err != nil {
		return fmt.Errorf("query metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return fmt.Errorf("scan metadata: %w", err)
		}

		a.metadata[k] = v
	}

	return rows.Err()
}

func (a *Account) readPerfData(db *sql.DB) error {
	rows, err := db.Query("SELECT date, metric, value FROM perf_data ORDER BY date, metric")
	if err != nil {
		return fmt.Errorf("query perf_data: %w", err)
	}
	defer rows.Close()

	type entry struct {
		timestamp  time.Time
		metricName data.Metric
		value      float64
	}

	var entries []entry

	timeSet := make(map[time.Time]bool)

	for rows.Next() {
		var (
			dateStr, metric string
			metricValue     float64
		)

		if err := rows.Scan(&dateStr, &metric, &metricValue); err != nil {
			return fmt.Errorf("scan perf_data: %w", err)
		}

		parsedTime, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse perf_data date: %w", err)
		}

		entries = append(entries, entry{timestamp: parsedTime, metricName: data.Metric(metric), value: metricValue})
		timeSet[parsedTime] = true
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// Build sorted unique times.
	times := make([]time.Time, 0, len(timeSet))
	for ts := range timeSet {
		times = append(times, ts)
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	// Build time index for fast lookup.
	timeIndex := make(map[time.Time]int, len(times))
	for idx, ts := range times {
		timeIndex[ts] = idx
	}

	// Fixed metric order.
	metrics := []data.Metric{data.PortfolioEquity, data.PortfolioBenchmark, data.PortfolioRiskFree}
	assets := []asset.Asset{portfolioAsset}
	vals := make([]float64, len(times)*len(metrics))

	metricIndex := make(map[data.Metric]int, len(metrics))
	for i, m := range metrics {
		metricIndex[m] = i
	}

	for _, ent := range entries {
		mIdx, ok := metricIndex[ent.metricName]
		if !ok {
			continue
		}

		tIdx := timeIndex[ent.timestamp]
		// Column-major: offset = (assetIdx*len(metrics) + metricIdx) * len(times) + timeIdx
		// Only one asset, so assetIdx = 0.
		vals[mIdx*len(times)+tIdx] = ent.value
	}

	perfFreq := data.Daily

	if freqStr, ok := a.metadata["perf_data_frequency"]; ok {
		parsed, parseErr := data.ParseFrequency(freqStr)
		if parseErr == nil {
			perfFreq = parsed
		}
	}

	df, err := data.NewDataFrame(times, assets, metrics, perfFreq, vals)
	if err != nil {
		return fmt.Errorf("build perfData: %w", err)
	}

	a.perfData = df

	return nil
}

func (a *Account) readTransactions(db *sql.DB) error {
	rows, err := db.Query("SELECT date, type, ticker, figi, quantity, price, amount, qualified, justification FROM transactions ORDER BY date")
	if err != nil {
		return fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			dateStr, typStr    string
			ticker, figi       sql.NullString
			qty, price, amount sql.NullFloat64
			qualified          sql.NullInt64
			justification      sql.NullString
		)

		if err := rows.Scan(&dateStr, &typStr, &ticker, &figi, &qty, &price, &amount, &qualified, &justification); err != nil {
			return fmt.Errorf("scan transaction: %w", err)
		}

		parsedTime, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse transaction date: %w", err)
		}

		txType, err := stringToTransactionType(typStr)
		if err != nil {
			return err
		}

		txn := Transaction{
			Date:      parsedTime,
			Type:      txType,
			Qty:       qty.Float64,
			Price:     price.Float64,
			Amount:    amount.Float64,
			Qualified: qualified.Valid && qualified.Int64 == 1,
		}

		if ticker.Valid || figi.Valid {
			txn.Asset = asset.Asset{
				Ticker:        ticker.String,
				CompositeFigi: figi.String,
			}
		}

		if justification.Valid {
			txn.Justification = justification.String
		}

		a.transactions = append(a.transactions, txn)
	}

	return rows.Err()
}

func (a *Account) readHoldings(db *sql.DB) error {
	rows, err := db.Query("SELECT asset_ticker, asset_figi, quantity FROM holdings")
	if err != nil {
		return fmt.Errorf("query holdings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ticker, figi string
			qty          float64
		)

		if err := rows.Scan(&ticker, &figi, &qty); err != nil {
			return fmt.Errorf("scan holding: %w", err)
		}

		ast := asset.Asset{Ticker: ticker, CompositeFigi: figi}
		a.holdings[ast] = qty
	}

	return rows.Err()
}

func (a *Account) readTaxLots(db *sql.DB) error {
	rows, err := db.Query("SELECT asset_ticker, asset_figi, date, quantity, price FROM tax_lots ORDER BY date")
	if err != nil {
		return fmt.Errorf("query tax_lots: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ticker, figi, dateStr string
			qty, price            float64
		)

		if err := rows.Scan(&ticker, &figi, &dateStr, &qty, &price); err != nil {
			return fmt.Errorf("scan tax_lot: %w", err)
		}

		parsedTime, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse tax_lot date: %w", err)
		}

		ast := asset.Asset{Ticker: ticker, CompositeFigi: figi}
		a.taxLots[ast] = append(a.taxLots[ast], TaxLot{
			Date:  parsedTime,
			Qty:   qty,
			Price: price,
		})
	}

	return rows.Err()
}

func (a *Account) readMetrics(db *sql.DB) error {
	rows, err := db.Query("SELECT date, name, window, value FROM metrics ORDER BY date, name, window")
	if err != nil {
		return fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			dateStr, name, window string
			value                 sql.NullFloat64
		)

		if err := rows.Scan(&dateStr, &name, &window, &value); err != nil {
			return fmt.Errorf("scan metric: %w", err)
		}

		parsedTime, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse metric date: %w", err)
		}

		a.metrics = append(a.metrics, MetricRow{
			Date:   parsedTime,
			Name:   name,
			Window: window,
			Value:  value.Float64,
		})
	}

	return rows.Err()
}
