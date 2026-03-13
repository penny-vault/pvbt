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
	"strconv"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	_ "modernc.org/sqlite"
)

const schemaVersion = "1"

const dateFormat = "2006-01-02"

const createSchema = `
CREATE TABLE metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE equity_curve (
    date  TEXT NOT NULL,
    value REAL NOT NULL
);

CREATE TABLE transactions (
    date      TEXT NOT NULL,
    type      TEXT NOT NULL,
    ticker    TEXT,
    figi      TEXT,
    quantity  REAL,
    price     REAL,
    amount    REAL,
    qualified INTEGER
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

CREATE TABLE price_series (
    series TEXT NOT NULL,
    date   TEXT NOT NULL,
    value  REAL NOT NULL
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
`

// transactionTypeToString maps a TransactionType to its lowercase string
// representation for storage.
func transactionTypeToString(t TransactionType) string {
	switch t {
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
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// stringToTransactionType maps a lowercase string back to a TransactionType.
func stringToTransactionType(s string) (TransactionType, error) {
	switch s {
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
		return 0, fmt.Errorf("unknown transaction type: %q", s)
	}
}

// ToSQLite serializes the account state to a SQLite database at the given path.
// The database is created fresh; if a file already exists at path it is
// overwritten.
func (a *Account) ToSQLite(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Create schema.
	if _, err := tx.Exec(createSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Write metadata.
	if err := a.writeMetadata(tx); err != nil {
		return err
	}

	// Write equity curve.
	if err := a.writeEquityCurve(tx); err != nil {
		return err
	}

	// Write transactions.
	if err := a.writeTransactions(tx); err != nil {
		return err
	}

	// Write holdings.
	if err := a.writeHoldings(tx); err != nil {
		return err
	}

	// Write tax lots.
	if err := a.writeTaxLots(tx); err != nil {
		return err
	}

	// Write price series.
	if err := a.writePriceSeries(tx); err != nil {
		return err
	}

	// Write metrics.
	if err := a.writeMetrics(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
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

	// Benchmark identity.
	if a.benchmark != (asset.Asset{}) {
		if _, err := stmt.Exec("benchmark_ticker", a.benchmark.Ticker); err != nil {
			return fmt.Errorf("insert benchmark_ticker: %w", err)
		}
		if _, err := stmt.Exec("benchmark_figi", a.benchmark.CompositeFigi); err != nil {
			return fmt.Errorf("insert benchmark_figi: %w", err)
		}
	}

	// Risk-free identity.
	if a.riskFree != (asset.Asset{}) {
		if _, err := stmt.Exec("risk_free_ticker", a.riskFree.Ticker); err != nil {
			return fmt.Errorf("insert risk_free_ticker: %w", err)
		}
		if _, err := stmt.Exec("risk_free_figi", a.riskFree.CompositeFigi); err != nil {
			return fmt.Errorf("insert risk_free_figi: %w", err)
		}
	}

	// User metadata.
	for k, v := range a.metadata {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("insert metadata %q: %w", k, err)
		}
	}

	return nil
}

func (a *Account) writeEquityCurve(tx *sql.Tx) error {
	if len(a.equityCurve) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO equity_curve (date, value) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare equity_curve: %w", err)
	}
	defer stmt.Close()

	for i, v := range a.equityCurve {
		d := a.equityTimes[i].Format(dateFormat)
		if _, err := stmt.Exec(d, v); err != nil {
			return fmt.Errorf("insert equity_curve: %w", err)
		}
	}

	return nil
}

func (a *Account) writeTransactions(tx *sql.Tx) error {
	if len(a.transactions) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO transactions (date, type, ticker, figi, quantity, price, amount, qualified) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare transactions: %w", err)
	}
	defer stmt.Close()

	for _, t := range a.transactions {
		d := t.Date.Format(dateFormat)
		typStr := transactionTypeToString(t.Type)
		qualified := 0
		if t.Qualified {
			qualified = 1
		}
		if _, err := stmt.Exec(d, typStr, t.Asset.Ticker, t.Asset.CompositeFigi, t.Qty, t.Price, t.Amount, qualified); err != nil {
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

func (a *Account) writePriceSeries(tx *sql.Tx) error {
	if len(a.benchmarkPrices) == 0 && len(a.riskFreePrices) == 0 {
		return nil
	}

	stmt, err := tx.Prepare("INSERT INTO price_series (series, date, value) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare price_series: %w", err)
	}
	defer stmt.Close()

	for i, v := range a.benchmarkPrices {
		if i < len(a.equityTimes) {
			d := a.equityTimes[i].Format(dateFormat)
			if _, err := stmt.Exec("benchmark", d, v); err != nil {
				return fmt.Errorf("insert benchmark price: %w", err)
			}
		}
	}

	for i, v := range a.riskFreePrices {
		if i < len(a.equityTimes) {
			d := a.equityTimes[i].Format(dateFormat)
			if _, err := stmt.Exec("risk_free", d, v); err != nil {
				return fmt.Errorf("insert risk_free price: %w", err)
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

// FromSQLite restores an Account from a SQLite database at the given path.
// The database must have been created by ToSQLite with schema_version "1".
// Fields that require a live broker or price DataFrame (broker, prices,
// registeredMetrics) are not restored.
func FromSQLite(path string) (*Account, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	// Ping to verify the file exists and is a valid database.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	a := &Account{
		holdings: make(map[asset.Asset]float64),
		taxLots:  make(map[asset.Asset][]TaxLot),
		metadata: make(map[string]string),
	}

	// Read metadata.
	if err := a.readMetadata(db); err != nil {
		return nil, err
	}

	// Verify schema version.
	if v := a.metadata["schema_version"]; v != schemaVersion {
		return nil, fmt.Errorf("unsupported schema version: %q (expected %q)", v, schemaVersion)
	}

	// Restore cash from metadata.
	if cashStr, ok := a.metadata["cash"]; ok {
		a.cash, err = strconv.ParseFloat(cashStr, 64)
		if err != nil {
			return nil, fmt.Errorf("parse cash: %w", err)
		}
	}

	// Restore benchmark and risk-free asset identity.
	if ticker, ok := a.metadata["benchmark_ticker"]; ok {
		a.benchmark = asset.Asset{
			Ticker:        ticker,
			CompositeFigi: a.metadata["benchmark_figi"],
		}
	}
	if ticker, ok := a.metadata["risk_free_ticker"]; ok {
		a.riskFree = asset.Asset{
			Ticker:        ticker,
			CompositeFigi: a.metadata["risk_free_figi"],
		}
	}

	// Remove internal metadata keys so they don't appear in user metadata.
	delete(a.metadata, "schema_version")
	delete(a.metadata, "cash")
	delete(a.metadata, "benchmark_ticker")
	delete(a.metadata, "benchmark_figi")
	delete(a.metadata, "risk_free_ticker")
	delete(a.metadata, "risk_free_figi")

	// Read equity curve.
	if err := a.readEquityCurve(db); err != nil {
		return nil, err
	}

	// Read transactions.
	if err := a.readTransactions(db); err != nil {
		return nil, err
	}

	// Read holdings.
	if err := a.readHoldings(db); err != nil {
		return nil, err
	}

	// Read tax lots.
	if err := a.readTaxLots(db); err != nil {
		return nil, err
	}

	// Read price series.
	if err := a.readPriceSeries(db); err != nil {
		return nil, err
	}

	// Read metrics.
	if err := a.readMetrics(db); err != nil {
		return nil, err
	}

	return a, nil
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

func (a *Account) readEquityCurve(db *sql.DB) error {
	rows, err := db.Query("SELECT date, value FROM equity_curve ORDER BY date")
	if err != nil {
		return fmt.Errorf("query equity_curve: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dateStr string
		var v float64
		if err := rows.Scan(&dateStr, &v); err != nil {
			return fmt.Errorf("scan equity_curve: %w", err)
		}
		t, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse equity_curve date: %w", err)
		}
		a.equityCurve = append(a.equityCurve, v)
		a.equityTimes = append(a.equityTimes, t)
	}

	return rows.Err()
}

func (a *Account) readTransactions(db *sql.DB) error {
	rows, err := db.Query("SELECT date, type, ticker, figi, quantity, price, amount, qualified FROM transactions ORDER BY date")
	if err != nil {
		return fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dateStr, typStr string
		var ticker, figi sql.NullString
		var qty, price, amount sql.NullFloat64
		var qualified sql.NullInt64
		if err := rows.Scan(&dateStr, &typStr, &ticker, &figi, &qty, &price, &amount, &qualified); err != nil {
			return fmt.Errorf("scan transaction: %w", err)
		}

		t, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse transaction date: %w", err)
		}

		txType, err := stringToTransactionType(typStr)
		if err != nil {
			return err
		}

		tx := Transaction{
			Date:      t,
			Type:      txType,
			Qty:       qty.Float64,
			Price:     price.Float64,
			Amount:    amount.Float64,
			Qualified: qualified.Valid && qualified.Int64 == 1,
		}

		if ticker.Valid || figi.Valid {
			tx.Asset = asset.Asset{
				Ticker:        ticker.String,
				CompositeFigi: figi.String,
			}
		}

		a.transactions = append(a.transactions, tx)
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
		var ticker, figi string
		var qty float64
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
		var ticker, figi, dateStr string
		var qty, price float64
		if err := rows.Scan(&ticker, &figi, &dateStr, &qty, &price); err != nil {
			return fmt.Errorf("scan tax_lot: %w", err)
		}
		t, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse tax_lot date: %w", err)
		}
		ast := asset.Asset{Ticker: ticker, CompositeFigi: figi}
		a.taxLots[ast] = append(a.taxLots[ast], TaxLot{
			Date:  t,
			Qty:   qty,
			Price: price,
		})
	}

	return rows.Err()
}

func (a *Account) readPriceSeries(db *sql.DB) error {
	rows, err := db.Query("SELECT series, value FROM price_series ORDER BY date")
	if err != nil {
		return fmt.Errorf("query price_series: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var series string
		var v float64
		if err := rows.Scan(&series, &v); err != nil {
			return fmt.Errorf("scan price_series: %w", err)
		}
		switch series {
		case "benchmark":
			a.benchmarkPrices = append(a.benchmarkPrices, v)
		case "risk_free":
			a.riskFreePrices = append(a.riskFreePrices, v)
		}
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
		var dateStr, name, window string
		var value sql.NullFloat64
		if err := rows.Scan(&dateStr, &name, &window, &value); err != nil {
			return fmt.Errorf("scan metric: %w", err)
		}
		t, err := time.Parse(dateFormat, dateStr)
		if err != nil {
			return fmt.Errorf("parse metric date: %w", err)
		}
		a.metrics = append(a.metrics, MetricRow{
			Date:   t,
			Name:   name,
			Window: window,
			Value:  value.Float64,
		})
	}

	return rows.Err()
}
