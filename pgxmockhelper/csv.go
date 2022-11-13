// Copyright 2021-2022
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

package pgxmockhelper

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/rs/zerolog/log"
)

type CSVRows struct {
	rows    [][]any
	header  []string
	dateCol int
}

func discoverTestDataPath(fn string) string {
	// try to figure out testdata path
	dataDir := os.Getenv("PVAPI_TEST_DATA_DIR")
	if dataDir != "" {
		return filepath.Join(dataDir, fn)
	}

	// try to guess based on PWD var
	pwdDir := os.Getenv("PWD")
	if pwdDir != "" {
		dataDir = filepath.Join(pwdDir, "testdata")
		// check if data dir exists, if it does use it
		_, err := os.Stat(dataDir)
		if !errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(dataDir, fn)
		}

		// try one directory up
		oneUpPwdDir := filepath.Dir(pwdDir)
		dataDir = filepath.Join(oneUpPwdDir, "testdata")
		// check if data dir exists, if it does use it
		_, err = os.Stat(dataDir)
		if !errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(dataDir, fn)
		}

		// try two directories up
		twoUpPwdDir := filepath.Dir(oneUpPwdDir)
		dataDir = filepath.Join(twoUpPwdDir, "testdata")
		// check if data dir exists, if it does use it
		_, err = os.Stat(dataDir)
		if !errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(dataDir, fn)
		}
	}

	log.Panic().Msg("could not resolve test data dir")
	return fn
}

func NewCSVRows(inputs []string, typeMap map[string]string) *CSVRows {
	rows := &CSVRows{
		dateCol: -1,
		rows:    make([][]any, 0),
	}

	for _, csvFn := range inputs {
		csvFn = discoverTestDataPath(csvFn)
		subLog := log.With().Str("CsvFn", csvFn).Logger()

		rawData, err := os.ReadFile(csvFn)
		if err != nil {
			subLog.Panic().Err(err).Msg("could not read file")
		}

		// break raw data into an array of lines
		lines := strings.Split(string(rawData), "\n")

		// sanity checks:
		// - array length is at least 3 (header + content + trailing newline)
		// - make sure last line ends in newline
		if len(lines) < 2 {
			subLog.Panic().Int("NumLines", len(lines)).Msg("input file does not have enough lines, need at least 2 (header + trailing new line)")
		}
		if lines[len(lines)-1] != "" {
			subLog.Panic().Msg("input file is missing a trailing new line")
		}

		// parse header
		headerRaw := lines[0]
		lines = lines[1 : len(lines)-1] // discard first and last rows
		rows.header = strings.Split(headerRaw, ",")

		// parse each line and create a row
		for _, ll := range lines {
			cols := make([]any, len(rows.header))
			parts := strings.Split(ll, ",")
			for idx, val := range parts {
				colName := rows.header[idx]
				if typeConv, ok := typeMap[colName]; ok {
					switch typeConv {
					case "date":
						parsed, err := time.Parse("2006-01-02", val)
						if err != nil {
							subLog.Panic().Err(err).Str("Val", val).Msg("could not convert val to datetime of format 2006-01-02")
						}
						// put in proper timezone
						res := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, common.GetTimezone())
						cols[idx] = res
						rows.dateCol = idx
					case "float64":
						parsed, err := strconv.ParseFloat(val, 64)
						if err != nil {
							subLog.Panic().Err(err).Str("Val", val).Msg("could not convert val to float64")
						}
						cols[idx] = parsed
					case "bool":
						parsed, err := strconv.ParseBool(val)
						if err != nil {
							subLog.Panic().Err(err).Str("Val", val).Msg("could not convert val to bool")
						}
						cols[idx] = parsed
					case "int":
						parsed, err := strconv.ParseInt(val, 10, 32)
						if err != nil {
							subLog.Panic().Err(err).Str("Val", val).Msg("could not convert val to int")
						}
						cols[idx] = int(parsed)
					default:
						// no type conversion specified - use as is
						cols[idx] = val
					}
				} else {
					cols[idx] = val
				}
			}
			rows.rows = append(rows.rows, cols)
		}
	}

	return rows
}

func (csvRows *CSVRows) Between(a time.Time, b time.Time) *CSVRows {
	newRows := make([][]any, 0, len(csvRows.rows))
	if len(csvRows.rows) == 0 {
		return csvRows
	}
	if csvRows.dateCol == -1 {
		log.Panic().Time("a", a).Time("b", b).Msg("no date column found")
	}
	for _, row := range csvRows.rows {
		t := row[csvRows.dateCol].(time.Time)
		if (t.Before(b) || t.Equal(b)) && (t.After(a) || t.Equal(a)) {
			newRows = append(newRows, row)
		}
	}
	csvRows.rows = newRows
	return csvRows
}

// Cols filters columns to only those specified in the cols array
func (csvRows *CSVRows) Cols(cols []string) *CSVRows {
	newRows := [][]any{}
	newHeader := make([]string, 0, len(csvRows.header))

	colsMap := make(map[string]bool, len(cols))
	for _, colName := range cols {
		colsMap[colName] = true
	}

	keep := make([]int, 0, len(csvRows.header))
	for idx, colName := range csvRows.header {
		if _, ok := colsMap[colName]; ok {
			newHeader = append(newHeader, colName)
			keep = append(keep, idx)
		}
	}

	for _, row := range csvRows.rows {
		newRow := []any{}
		for _, k := range keep {
			newRow = append(newRow, row[k])
		}
		newRows = append(newRows, newRow)
	}

	csvRows.header = newHeader
	csvRows.rows = newRows

	return csvRows
}

func (csvRows *CSVRows) Rows() *pgxmock.Rows {
	r := pgxmock.NewRows(csvRows.header)
	for _, row := range csvRows.rows {
		r.AddRow(row...)
	}
	return r
}

func MockManager(db pgxmock.PgxConnIface) {
	MockHolidays(db)
	MockAssets(db)
	MockTradingDays(db)
}

func MockHolidays(db pgxmock.PgxConnIface) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT event_date, early_close").WillReturnRows(
		NewCSVRows([]string{"market_holidays.csv"}, map[string]string{
			"event_date":  "date",
			"early_close": "bool",
			"close_time":  "int",
		}).Rows())
	db.ExpectCommit()
}

func MockTradingDays(db pgxmock.PgxConnIface) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT trading_day FROM trading_days").WillReturnRows(
		NewCSVRows([]string{"tradingdays.csv"}, map[string]string{
			"trade_day": "date",
		}).Between(time.Date(1980, 1, 1, 0, 0, 0, 0, common.GetTimezone()), time.Date(2022, 6, 17, 0, 0, 0, 0, common.GetTimezone())).Rows())
	db.ExpectCommit()
}

func MockAssets(db pgxmock.PgxConnIface) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT ticker, composite_figi, active FROM assets ORDER BY active").WillReturnRows(
		NewCSVRows([]string{"assets.csv"}, map[string]string{
			"active": "bool",
		}).Rows())
	db.ExpectCommit()
}

func MockDBEodQuery(db pgxmock.PgxConnIface, fn []string, a, b time.Time, cols ...string) {
	cols = append(cols, "event_date", "composite_figi")
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT event_date, composite_figi, (.+) FROM eod").WillReturnRows(
		NewCSVRows(fn, map[string]string{
			"event_date":   "date",
			"open":         "float64",
			"high":         "float64",
			"low":          "float64",
			"close":        "float64",
			"adj_close":    "float64",
			"split_factor": "float64",
			"dividend":     "float64",
		}).Between(a, b).Cols(cols).Rows())
	db.ExpectCommit()
}
