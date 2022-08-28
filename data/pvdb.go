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
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgtype"
	dataframe "github.com/jdfergason/dataframe-go"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/tradecron"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrUnsupportedMetric = errors.New("unsupported metric")
	ErrInvalidTimeRange  = errors.New("start must be before end")
	ErrNoTradingDays     = errors.New("no trading days available")
)

type Pvdb struct {
	cache     map[string]float64
	Dividends map[Security][]*Measurement
	Splits    map[Security][]*Measurement
	hashFunc  func(date time.Time, metric Metric, security *Security) string
}

// NewPVDB Create a new PVDB data provider
func NewPVDB(cache map[string]float64, hashFunc func(date time.Time, metric Metric, security *Security) string) *Pvdb {
	return &Pvdb{
		cache:     cache,
		hashFunc:  hashFunc,
		Dividends: make(map[Security][]*Measurement),
		Splits:    make(map[Security][]*Measurement),
	}
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
func (p *Pvdb) TradingDays(ctx context.Context, begin time.Time, end time.Time, frequency Frequency) ([]time.Time, error) {
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

// Provider functions

func (p *Pvdb) DataType() string {
	return "security"
}

// uniqueSecurities filters a list of Securities to only unique values
func uniqueSecurities(securities []*Security) []*Security {
	unique := make(map[string]*Security, len(securities))
	for _, v := range securities {
		unique[v.CompositeFigi] = v
	}
	uniqList := make([]*Security, len(unique))
	j := 0
	for _, v := range unique {
		uniqList[j] = v
		j++
	}
	return uniqList
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

func (p *Pvdb) GetDataForPeriod(ctx context.Context, securities []*Security, metric Metric, frequency Frequency, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
	defer span.End()
	tz := common.GetTimezone()
	subLog := log.With().Str("Metric", string(metric)).Str("Frequency", string(frequency)).Time("StartTime", begin).Time("EndTime", end).Logger()

	tradingDays, err := p.TradingDays(ctx, begin, end, frequency)
	if err != nil {
		return nil, err
	}

	// ensure securities is a unique set
	securities = uniqueSecurities(securities)

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Msg(msg)
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
	sql := fmt.Sprintf("SELECT event_date, composite_figi, close, adj_close::double precision FROM eod WHERE composite_figi IN (%s) AND event_date BETWEEN $1 AND $2 ORDER BY event_date DESC, composite_figi", figiArgs)

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Str("SQL", sql).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return nil, err
	}

	// parse database rows
	vals := make(map[int]map[string]float64, len(securities))

	var date time.Time
	var lastDate time.Time
	var compositeFigi string
	var close float64
	var adjClose pgtype.Float8

	securityCnt := len(securities)

	for rows.Next() {
		err = rows.Scan(&date, &compositeFigi, &close, &adjClose)
		s, err := SecurityFromFigi(compositeFigi)
		if err != nil {
			subLog.Error().Err(err).Str("CompositeFigi", compositeFigi).Msg("security does not exist")
			return nil, err
		}

		p.cache[p.hashFunc(date, MetricClose, s)] = close
		if adjClose.Status == pgtype.Present {
			p.cache[p.hashFunc(date, MetricAdjustedClose, s)] = adjClose.Float
		}

		if err != nil {
			subLog.Error().Stack().Err(err).Msg("failed to load eod prices -- db query scan failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
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

	// preload splits & divs
	p.preloadCorporateActions(ctx, securities, begin)

	// build dataframe
	df := buildDataFrame(vals, securities, tradingDays, lastDate)
	return df, nil
}

func (p *Pvdb) preloadCorporateActions(ctx context.Context, securities []*Security, start time.Time) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
	defer span.End()

	tz := common.GetTimezone()

	corporateFigiSet := make([]string, 0, len(securities))
	for _, security := range securities {
		if _, ok := p.Dividends[*security]; !ok {
			corporateFigiSet = append(corporateFigiSet, security.CompositeFigi)
			p.Dividends[*security] = make([]*Measurement, 0)
			p.Splits[*security] = make([]*Measurement, 0)
		}
	}

	if len(corporateFigiSet) == 0 {
		log.Debug().Msg("skipping preload of corporate actions because there are no additional securities to preload")
		return // nothing needs to be loaded
	}

	log.Debug().Time("Start", start).Strs("FIGI", corporateFigiSet).Msg("pre-load from corporate actions")

	subLog := log.With().Strs("Figis", corporateFigiSet).Time("StartTime", start).Logger()

	args := make([]interface{}, len(corporateFigiSet)+1)
	args[0] = start

	figiPlaceholders := make([]string, len(corporateFigiSet))
	for idx, figi := range corporateFigiSet {
		figiPlaceholders[idx] = fmt.Sprintf("$%d", idx+2)
		args[idx+1] = figi
	}
	corporateFigiArgs := strings.Join(figiPlaceholders, ", ")
	sql := fmt.Sprintf("SELECT event_date, composite_figi, dividend, split_factor FROM eod WHERE composite_figi IN (%s) AND event_date >= $1 AND (dividend != 0 OR split_factor != 1.0) ORDER BY event_date DESC, composite_figi", corporateFigiArgs)

	trx, err := database.TrxForUser(ctx, "pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to get transaction for preloading corporate actions"
		span.SetStatus(codes.Error, msg)
		log.Warn().Stack().Err(err).Msg(msg)
		return
	}

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Stack().Err(err).Str("SQL", sql).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return
	}

	var date time.Time
	var compositeFigi string
	var dividend float64
	var splitFactor float64

	for rows.Next() {
		err = rows.Scan(&date, &compositeFigi, &dividend, &splitFactor)
		if err != nil {
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			subLog.Panic().Stack().Err(err).Msg("failed to load corporate actions -- db query scan failed")
			return
		}

		security, err := SecurityFromFigi(compositeFigi)
		if err != nil {
			log.Panic().Err(err).Str("CompositeFigi", compositeFigi).Msg("asset map out of sync")
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
		divs := p.Dividends[*security]
		if dividend != 0.0 {
			divs = append(divs, &Measurement{
				Date:  date,
				Value: dividend,
			})
		}

		splits := p.Splits[*security]
		if splitFactor != 1.0 {
			splits = append(splits, &Measurement{
				Date:  date,
				Value: splitFactor,
			})
		}

		p.Dividends[*security] = divs
		p.Splits[*security] = splits
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("error committing transaction")
	}
}

func (p *Pvdb) GetLatestDataBefore(ctx context.Context, security *Security, metric Metric, before time.Time) (float64, error) {
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
