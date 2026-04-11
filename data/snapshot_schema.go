package data

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// CreateSnapshotSchema creates all tables needed by a snapshot database.
func CreateSnapshotSchema(db *sql.DB) error {
	fundamentalCols := buildFundamentalColumns()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS assets (
			composite_figi TEXT PRIMARY KEY,
			ticker TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			asset_type TEXT NOT NULL DEFAULT '',
			primary_exchange TEXT NOT NULL DEFAULT '',
			sector TEXT NOT NULL DEFAULT '',
			industry TEXT NOT NULL DEFAULT '',
			sic_code INTEGER NOT NULL DEFAULT 0,
			cik TEXT NOT NULL DEFAULT '',
			listed TEXT NOT NULL DEFAULT '',
			delisted TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE IF NOT EXISTS eod (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			open REAL,
			high REAL,
			low REAL,
			close REAL,
			adj_close REAL,
			volume REAL,
			dividend REAL,
			split_factor REAL,
			PRIMARY KEY (composite_figi, event_date)
		)`,

		`CREATE TABLE IF NOT EXISTS metrics (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			market_cap INTEGER,
			ev INTEGER,
			pe REAL,
			pb REAL,
			ps REAL,
			ev_ebit REAL,
			ev_ebitda REAL,
			pe_forward REAL,
			peg REAL,
			price_to_cash_flow REAL,
			beta REAL,
			PRIMARY KEY (composite_figi, event_date)
		)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS fundamentals (
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			event_date TEXT NOT NULL,
			date_key TEXT,
			report_period TEXT,
			dimension TEXT NOT NULL,
			%s,
			PRIMARY KEY (composite_figi, event_date, dimension)
		)`, fundamentalCols),

		`CREATE TABLE IF NOT EXISTS ratings (
			analyst TEXT NOT NULL,
			filter_values TEXT NOT NULL,
			event_date TEXT NOT NULL,
			composite_figi TEXT NOT NULL,
			ticker TEXT NOT NULL,
			PRIMARY KEY (analyst, filter_values, event_date, composite_figi)
		)`,

		`CREATE TABLE IF NOT EXISTS index_members (
			index_name TEXT NOT NULL,
			event_date TEXT NOT NULL,
			composite_figi TEXT NOT NULL REFERENCES assets(composite_figi),
			ticker TEXT NOT NULL,
			weight REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (index_name, event_date, composite_figi)
		)`,

		`CREATE TABLE IF NOT EXISTS market_holidays (
			event_date TEXT NOT NULL PRIMARY KEY,
			early_close INTEGER NOT NULL DEFAULT 0,
			close_time INTEGER NOT NULL DEFAULT 0
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("snapshot schema: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// buildFundamentalColumns generates the SQL column definitions for the
// fundamentals table from the metricColumn map. Columns are sorted
// alphabetically for deterministic DDL output.
func buildFundamentalColumns() string {
	seen := make(map[string]bool)

	var cols []string

	for _, colName := range metricColumn {
		if seen[colName] {
			continue
		}

		seen[colName] = true
		cols = append(cols, colName)
	}

	sort.Strings(cols)

	defs := make([]string, len(cols))
	for idx, col := range cols {
		defs[idx] = fmt.Sprintf("%s REAL", col)
	}

	return strings.Join(defs, ",\n\t\t\t")
}
