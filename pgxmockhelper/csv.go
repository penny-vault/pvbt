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
	"errors"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/pashagolub/pgxmock"
	"github.com/rs/zerolog/log"
)

var (
	ErrInsufficientLines    error = errors.New("input files must have at least 3 lines (header + content + trailing new line)")
	ErrNoTrailingNewLine    error = errors.New("missing trailing newline")
	ErrTypeConversionFailed error = errors.New("type conversion failed")
)

// RowsFromCSV reads the specified CSV file and returns pgxrows. `typeMap` allows optional type
// conversions otherwise all values are returned as strings. CSV parsing is simple - first row
// must be a header, columns are not surrounded by quotes,
func RowsFromCSV(csvFn string, typeMap map[string]string) (*pgxmock.Rows, error) {
	rawData, err := ioutil.ReadFile(csvFn)
	if err != nil {
		return nil, err
	}

	subLog := log.With().Str("CsvFn", csvFn).Logger()

	// break raw data into an array of lines
	lines := strings.Split(string(rawData), "\n")

	// sanity checks:
	// - array length is at least 3 (header + content + trailing newline)
	// - make sure last line ends in newline
	if len(lines) < 3 {
		subLog.Error().Int("NumLines", len(lines)).Msg("input file does not have enough lines, need at least 3 (header + content + trailing new line)")
		return nil, ErrInsufficientLines
	}
	if lines[len(lines)-1] != "" {
		subLog.Error().Msg("input file is missing a trailing new line")
		return nil, ErrNoTrailingNewLine
	}

	// parse header
	headerRaw := lines[0]
	lines = lines[1 : len(lines)-1] // discard first and last rows
	header := strings.Split(headerRaw, ",")

	// create rows structure
	rows := pgxmock.NewRows(header)

	// parse each line and create a row
	for _, ll := range lines {
		cols := make([]any, len(header))
		parts := strings.Split(ll, ",")
		for idx, val := range parts {
			colName := header[idx]
			if typeConv, ok := typeMap[colName]; ok {
				switch typeConv {
				case "date":
					parsed, err := time.Parse("2006-01-02", val)
					if err != nil {
						log.Error().Err(err).Str("Val", val).Msg("could not convert val to datetime of format 2006-01-02")
						return nil, ErrTypeConversionFailed
					}
					cols[idx] = parsed
				case "float64":
					parsed, err := strconv.ParseFloat(val, 64)
					if err != nil {
						log.Error().Err(err).Str("Val", val).Msg("could not convert val to float64")
						return nil, ErrTypeConversionFailed
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
		rows = rows.AddRow(cols...)
	}

	return rows, nil
}
