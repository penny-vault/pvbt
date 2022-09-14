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

package data

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

type PvDb struct {
}

// NewPVDB Create a new PVDB data provider
func NewPvDb() *PvDb {
	return &PvDb{}
}

// TradingDays returns a list of trading days between begin and end at the desired frequency
func (p *PvDb) TradingDays(ctx context.Context, begin time.Time, end time.Time) ([]time.Time, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.TradingDays")
	defer span.End()

	tz := common.GetTimezone()

	subLog := log.With().Time("Begin", begin).Time("End", end).Logger()
	subLog.Debug().Msg("getting trading days")

	res := make([]time.Time, 0, 252)
	if end.Before(begin) {
		subLog.Warn().Stack().Msg("end before begin in call to TradingDays")
		return res, ErrInvalidTimeRange
	}

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("could not get transaction when querying trading days")
		return res, err
	}

	searchBegin := begin
	searchEnd := end
	rows, err := trx.Query(ctx, "SELECT trading_day FROM trading_days WHERE market='us' AND trading_day BETWEEN $1 and $2 ORDER BY trading_day", searchBegin, searchEnd)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "database query failed")
		subLog.Error().Stack().Err(err).Msg("could not query trading days")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return res, err
	}

	for rows.Next() {
		var dt time.Time
		if err = rows.Scan(&dt); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not SCAN DB result")
			if err := trx.Rollback(ctx); err != nil {
				subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return res, err
		}

		dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
		res = append(res, dt)
	}

	if len(res) == 0 {
		span.SetStatus(codes.Error, "no trading days found")
		subLog.Error().Stack().Msg("could not load trading days")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return res, ErrNoTradingDays
	}

	if len(res) == 0 {
		subLog.Error().Stack().Msg("days array is empty")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return res, ErrNoTradingDays
	}

	if err := trx.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return res, nil
}

// GetEOD fetches EOD metrics from the database
func (p *PvDb) GetEOD(ctx context.Context, securities []*Security, metrics []Metric, begin, end time.Time) (map[SecurityMetric][]float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetEOD")
	defer span.End()
	tz := common.GetTimezone()

	// ensure securities is a unique set
	securities = uniqueSecurities(securities)

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		log.Warn().Stack().Err(err).Msg(msg)
		return nil, err
	}

	// build SQL query
	args := make([]interface{}, len(securities)+2)
	args[0] = begin
	args[1] = end

	figiSet := make([]string, len(securities))
	for idx, security := range securities {
		figiSet[idx] = fmt.Sprintf("$%d", idx+3)
		args[idx+2] = security.CompositeFigi
	}
	figiArgs := strings.Join(figiSet, ", ")
	metricColumns := metricsToColumns(metrics)
	sql := fmt.Sprintf("SELECT event_date, composite_figi, %s FROM eod WHERE composite_figi IN (%s) AND event_date BETWEEN $1 AND $2 ORDER BY event_date DESC, composite_figi", metricColumns, figiArgs)

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- db query failed"
		span.SetStatus(codes.Error, msg)
		log.Warn().Stack().Err(err).Str("SQL", sql).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return nil, err
	}

	// parse database rows
	vals := make(map[SecurityMetric][]float64, len(securities))

	for rows.Next() {
		var eventDate time.Time
		var compositeFigi string

		args := []interface{}{&eventDate, &compositeFigi}
		metricVals := make([]*float64, len(metricColumns))
		for _, metricVal := range metricVals {
			args = append(args, metricVal)
		}

		if err := rows.Scan(args...); err != nil {
			log.Error().Stack().Err(err).Msg("failed to load eod prices -- db query scan failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		eventDate = time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 16, 0, 0, 0, tz)
		security, err := SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Error().Err(err).Msg("cannot lookup security in local security cache")
		}

		for idx, metric := range metrics {
			securityMetric := SecurityMetric{
				SecurityObject: *security,
				MetricObject:   metric,
			}
			if metricArray, ok := vals[securityMetric]; ok {
				metricArray = append(metricArray, *metricVals[idx])
				vals[securityMetric] = metricArray
			} else {
				vals[securityMetric] = []float64{*metricVals[idx]}
			}
		}
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("error committing transaction")
	}

	return vals, nil
}

func (p *PvDb) GetEODOnOrBefore(ctx context.Context, security *Security, metric Metric, date time.Time) (float64, time.Time, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetEODOnOrBefore")
	defer span.End()
	subLog := log.With().Str("Security.Ticker", security.Ticker).Str("Security.Figi", security.CompositeFigi).Str("Metric", string(metric)).Time("Date", date).Logger()

	tz := common.GetTimezone()

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Msg(msg)
		return math.NaN(), time.Time{}, err
	}

	// build SQL query
	var columns string
	switch metric {
	case MetricOpen:
		columns = "open AS val"
	case MetricHigh:
		columns = "high AS val"
	case MetricLow:
		columns = "low AS val"
	case MetricClose:
		columns = "close AS val"
	case MetricVolume:
		columns = "(volume::double precision) AS val"
	case MetricAdjustedClose:
		columns = "(adj_close::double precision) AS val"
	case MetricDividendCash:
		columns = "dividend AS val"
	case MetricSplitFactor:
		columns = "split_factor AS val"
	default:
		span.SetStatus(codes.Error, "un-supported metric")
		subLog.Error().Stack().Msg("un-supported metric requested")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return math.NaN(), time.Time{}, ErrUnsupportedMetric
	}

	sql := fmt.Sprintf("SELECT event_date, %s FROM eod WHERE composite_figi=$1 AND event_date <= $2 ORDER BY event_date DESC, ticker LIMIT 1", columns)

	// execute the query
	rows, err := trx.Query(ctx, sql, security.CompositeFigi, date)
	if err != nil {
		span.RecordError(err)
		msg := "db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return math.NaN(), time.Time{}, err
	}

	var eventDate time.Time
	var val float64

	for rows.Next() {
		err = rows.Scan(&eventDate, &val)
		if err != nil {
			span.RecordError(err)
			msg := "db scan failed"
			span.SetStatus(codes.Error, msg)
			subLog.Warn().Stack().Err(err).Msg(msg)
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return math.NaN(), time.Time{}, err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction")
	}
	return val, eventDate, err
}

func metricsToColumns(metrics []Metric) string {
	metricCols := make([]string, len(metrics))
	for idx, metric := range metrics {
		switch metric {
		case MetricOpen:
			metricCols[idx] = "open"
		case MetricLow:
			metricCols[idx] = "low"
		case MetricHigh:
			metricCols[idx] = "high"
		case MetricClose:
			metricCols[idx] = "close"
		case MetricAdjustedClose:
			metricCols[idx] = "adj_close"
		case MetricSplitFactor:
			metricCols[idx] = "split_factor"
		case MetricDividendCash:
			metricCols[idx] = "dividend"
		case MetricVolume:
			metricCols[idx] = "volume"
		}
	}
	return strings.Join(metricCols, ", ")
}
