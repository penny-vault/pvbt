// Copyright 2021-2023
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
	"fmt"
	"strings"

	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// NOTE: This is a temporary extension of the data library until datasets are fully implemented

// GetFundamentals returns a dataframe with the requested fundamental values on the data specified
func GetFundamentals(ctx context.Context, year int, period ReportingPeriod, securities []*Security, metrics []FundamentalMetric) (*dataframe.DataFrame[string], error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "GetFundamentals")
	defer span.End()

	// ensure securities is a unique set
	securities = uniqueSecurities(securities)

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to load fundamentals -- could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		log.Warn().Stack().Err(err).Msg(msg)
		return nil, err
	}

	var calendarDate string
	var dim string
	switch period {
	case PeriodQ1:
		calendarDate = fmt.Sprintf("%d-03-31", year)
		dim = "As-Reported-Quarterly"
	case PeriodQ2:
		calendarDate = fmt.Sprintf("%d-06-30", year)
		dim = "As-Reported-Quarterly"
	case PeriodQ3:
		calendarDate = fmt.Sprintf("%d-09-30", year)
		dim = "As-Reported-Quarterly"
	case PeriodQ4:
		calendarDate = fmt.Sprintf("%d-12-31", year)
		dim = "As-Reported-Quarterly"
	case PeriodAnnual:
		calendarDate = fmt.Sprintf("%d-12-31", year)
		dim = "As-Reported-Annual"
	default:
		calendarDate = fmt.Sprintf("%d-03-31", year)
		dim = "As-Reported-Quarterly"
	}

	figis := make([]string, 0, len(securities))
	for _, security := range securities {
		figis = append(figis, fmt.Sprintf("'%s'", security.CompositeFigi))
	}
	figiStr := strings.Join(figis, ", ")

	fields := strings.Join(any(metrics).([]string), ", ")
	sql := fmt.Sprintf("SELECT composite_figi, %s FROM fundamentals WHERE calendar_date=$1 AND dim=$2 AND composite_figi IN (%s) ORDER BY event_date, ticker", fields, figiStr)

	df := &dataframe.DataFrame[string]{
		ColNames: any(metrics).([]string),
	}

	if rows, err := trx.Query(ctx, sql, calendarDate, dim); err != nil {
		for rows.Next() {
			var compositeFigi string

			args := []interface{}{&compositeFigi}
			metricVals := make([]*float64, len(metrics))
			for idx := range metricVals {
				var metricVal float64
				metricVals[idx] = &metricVal
				args = append(args, &metricVal)
			}

			if err := rows.Scan(args...); err != nil {
				log.Error().Err(err).Msg("error scanning value from query in GetFundamentals")
				return nil, err
			}

			// build map
			vals := make(map[string]float64)
			for idx, metric := range metrics {
				vals[string(metric)] = *metricVals[idx]
			}

			df.InsertMap(compositeFigi, vals)
		}
	} else {
		log.Error().Err(err).Str("CalendarDate", calendarDate).Str("SQL", sql).Msg("Query fundamentals database failed in GetFundamentals")
	}

	return df, nil
}
