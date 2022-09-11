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

	"github.com/georgysavva/scany/pgxscan"
	dataframe "github.com/jdfergason/dataframe-go"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/tradecron"
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

// Date provider functions

func adjustSearchDates(frequency Frequency, begin, end time.Time) (time.Time, time.Time) {
	switch frequency {
	case FrequencyMonthly:
		begin = begin.AddDate(0, -1, 0)
		end = end.AddDate(0, 1, 0)
	case FrequencyAnnually:
		begin = begin.AddDate(-1, 0, 0)
		end = end.AddDate(1, 0, 0)
	default:
		begin = begin.AddDate(0, 0, -7)
		end = end.AddDate(0, 0, 7)
	}
	return begin, end
}

func filterDays(frequency Frequency, res []time.Time) []time.Time {
	days := make([]time.Time, 0, 252)

	var schedule *tradecron.TradeCron
	var err error

	switch frequency {
	case FrequencyDaily:
		schedule, err = tradecron.New("@close * * *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close * * *").Msg("could not build tradecron schedule")
		}
	case FrequencyWeekly:
		schedule, err = tradecron.New("@close @weekend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @weekend").Msg("could not build tradecron schedule")
		}
	case FrequencyMonthly:
		schedule, err = tradecron.New("@close @monthend", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend").Msg("could not build tradecron schedule")
		}
	case FrequencyAnnually:
		schedule, err = tradecron.New("@close @monthend 12 *", tradecron.RegularHours)
		if err != nil {
			log.Panic().Err(err).Str("Schedule", "@close @monthend 12 *").Msg("could not build tradecron schedule")
		}
	}

	for _, xx := range res {
		if schedule.IsTradeDay(xx) {
			days = append(days, xx)
		}
	}
	return days
}

// TradingDays returns a list of trading days between begin and end at the desired frequency
func (p *PvDb) TradingDays(ctx context.Context, begin time.Time, end time.Time, frequency Frequency) ([]time.Time, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.TradingDays")
	defer span.End()

	tz := common.GetTimezone()

	subLog := log.With().Time("Begin", begin).Time("End", end).Str("Frequency", string(frequency)).Logger()
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
	searchBegin, searchEnd = adjustSearchDates(frequency, searchBegin, searchEnd)

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

	cnt := len(res) - 1

	if len(res) == 0 {
		span.SetStatus(codes.Error, "no trading days found")
		subLog.Error().Stack().Msg("could not load trading days")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return res, ErrNoTradingDays
	}

	days := filterDays(frequency, res)

	daysFiltered := make([]time.Time, 0, 252)
	lastDay := res[cnt]
	if len(days) == 0 {
		subLog.Error().Stack().Msg("days array is empty")
		if err := trx.Rollback(ctx); err != nil {
			subLog.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return daysFiltered, ErrNoTradingDays
	}

	if !lastDay.Equal(days[len(days)-1]) {
		days = append(days, res[cnt])
	}

	// final filter to actual days
	beginFull := time.Date(begin.Year(), begin.Month(), begin.Day(), 0, 0, 0, 0, begin.Location())
	endFull := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999_999_999, begin.Location())
	for _, d := range days {
		if (d.Before(endFull) || d.Equal(endFull)) && (d.After(beginFull) || d.Equal(beginFull)) {
			daysFiltered = append(daysFiltered, d)
		}
	}

	if err := trx.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not commit transaction")
	}
	return daysFiltered, nil
}

func buildDataFrame(vals map[int]map[string]float64, securities []*Security, tradingDays []time.Time, lastDate time.Time) *dataframe.DataFrame {
	securityCnt := len(securities)
	series := make([]dataframe.Series, 0, securityCnt+1)
	series = append(series, dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Capacity: len(tradingDays)}, tradingDays))

	// build series
	vals2 := make(map[string][]float64, securityCnt)
	for _, security := range securities {
		vals2[security.CompositeFigi] = make([]float64, len(tradingDays))
	}

	for idx, k := range tradingDays {
		dayData, ok := vals[k.Year()*1000+k.YearDay()]
		if !ok {
			dayData = vals[lastDate.Year()*1000+k.YearDay()]
		}

		for _, security := range securities {
			v, ok := dayData[security.CompositeFigi]
			if !ok {
				vals2[security.CompositeFigi][idx] = math.NaN()
			} else {
				vals2[security.CompositeFigi][idx] = v
			}
		}
	}

	// break arrays out of map in order to build the dataframe
	for _, security := range securities {
		arr := vals2[security.CompositeFigi]
		series = append(series, dataframe.NewSeriesFloat64(security.CompositeFigi, &dataframe.SeriesInit{Capacity: len(arr)}, arr))
	}

	df := dataframe.NewDataFrame(series...)
	return df
}

// TODO
func (p *PvDb) GetMeasurements(ctx context.Context, securities []*Security, metrics []Metric, dates []time.Time) (map[SecurityMetric][]float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
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
	args[0] = dates[0]
	args[1] = dates[len(dates)-1]

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
	vals := make(map[int]map[string]float64, len(securities))

	type Measurement struct {
		EventDate     time.Time
		CompositeFigi string
		Open          float64
		High          float64
		Low           float64
		Close         float64
		AdjClose      float64
	}

	securityCnt := len(securities)

	for rows.Next() {
		meas := Measurement{}

		if err := pgxscan.ScanRow(&meas, rows); err != nil {
			log.Error().Stack().Err(err).Msg("failed to load eod prices -- db query scan failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		meas.EventDate = time.Date(meas.EventDate.Year(), meas.EventDate.Month(), meas.EventDate.Day(), 16, 0, 0, 0, tz)
		dateHash := date.Year()*1000 + date.YearDay()
		valMap, ok := vals[dateHash]
		if !ok {
			valMap = make(map[string]float64, securityCnt)
			vals[dateHash] = valMap
		}

		switch metric {
		case MetricClose:
			valMap[compositeFigi] = close
		case MetricAdjustedClose:
			valMap[compositeFigi] = adjClose.Float
		default:
			span.SetStatus(codes.Error, "un-supported metric")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			log.Panic().Str("Metric", string(metric)).Msg("Unsupported metric type")
			return nil, ErrUnsupportedMetric
		}

		lastDate = date
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Stack().Err(err).Msg("error committing transaction")
	}

	return df, nil
}

func (p *PvDb) GetLatestDataBefore(ctx context.Context, security *Security, metric Metric, before time.Time) (float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetLatestDataBefore")
	defer span.End()
	subLog := log.With().Str("Symbol", security.Ticker).Str("Metric", string(metric)).Time("Before", before).Logger()

	tz := common.GetTimezone()

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Msg(msg)
		return math.NaN(), err
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
		return math.NaN(), ErrUnsupportedMetric
	}

	sql := fmt.Sprintf("SELECT event_date, %s FROM eod WHERE composite_figi=$1 AND event_date <= $2 ORDER BY event_date DESC LIMIT 1", columns)

	// execute the query
	rows, err := trx.Query(ctx, sql, security.CompositeFigi, before)
	if err != nil {
		span.RecordError(err)
		msg := "db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return math.NaN(), err
	}

	var date time.Time
	var val float64

	for rows.Next() {
		err = rows.Scan(&date, &val)
		if err != nil {
			span.RecordError(err)
			msg := "db scan failed"
			span.SetStatus(codes.Error, msg)
			subLog.Warn().Stack().Err(err).Msg(msg)
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return math.NaN(), err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction")
	}
	return val, err
}
