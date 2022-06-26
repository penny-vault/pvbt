// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pgxmockhelper

import (
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/pashagolub/pgxmock"
	"github.com/rs/zerolog/log"
)

type CSVRows struct {
	rows    [][]any
	header  []string
	dateCol int
}

func NewCSVRows(csvFn string, typeMap map[string]string) *CSVRows {
	subLog := log.With().Str("CsvFn", csvFn).Logger()

	rows := &CSVRows{
		dateCol: -1,
		rows:    make([][]any, 0),
	}
	rawData, err := ioutil.ReadFile(csvFn)
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
					cols[idx] = parsed
					rows.dateCol = idx
				case "float64":
					parsed, err := strconv.ParseFloat(val, 64)
					if err != nil {
						subLog.Panic().Err(err).Str("Val", val).Msg("could not convert val to float64")
					}
					cols[idx] = parsed
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

func (csvRows *CSVRows) Rows() *pgxmock.Rows {
	r := pgxmock.NewRows(csvRows.header)
	for _, row := range csvRows.rows {
		r.AddRow(row...)
	}
	return r
}

func MockDBEodQuery(db pgxmock.PgxConnIface, fn string, d1, d2, d3, d4 time.Time) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT trading_day FROM trading_days").WillReturnRows(
		NewCSVRows("../testdata/tradingdays.csv", map[string]string{
			"trade_day": "date",
		}).Between(d1, d2).Rows())
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT event_date, ticker, close, adj_close FROM eod").WillReturnRows(
		NewCSVRows(fn, map[string]string{
			"event_date": "date",
			"close":      "float64",
			"adj_close":  "float64",
		}).Between(d3, d4).Rows())
}

func MockDBCorporateQuery(db pgxmock.PgxConnIface, fn string, d1, d2 time.Time) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT event_date, ticker, dividend, split_factor FROM eod").WillReturnRows(
		NewCSVRows(fn, map[string]string{
			"event_date":   "date",
			"dividend":     "float64",
			"split_factor": "float64",
		}).Between(d1, d2).Rows())
}

func CheckDBQuery1(db pgxmock.PgxConnIface, fn string, d1, d2, d3, d4 time.Time) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
}

func CheckDBQuery2(db pgxmock.PgxConnIface, fn string, d1, d2, d3, d4 time.Time) {
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
	db.ExpectQuery("SELECT trading_day FROM trading_days").WillReturnRows(
		NewCSVRows("../testdata/tradingdays.csv", map[string]string{
			"trade_day": "date",
		}).Between(d1, d2).Rows())
	db.ExpectBegin()
	db.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
}
