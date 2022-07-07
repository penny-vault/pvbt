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
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrUnsupportedMetric = errors.New("unsupported metric")
)

type Pvdb struct {
	cache     map[string]float64
	Dividends map[string][]*Measurement
	Splits    map[string][]*Measurement
	hashFunc  func(date time.Time, metric Metric, symbol string) string
}

// NewPVDB Create a new PVDB data provider
func NewPVDB(cache map[string]float64, hashFunc func(date time.Time, metric Metric, symbol string) string) *Pvdb {
	return &Pvdb{
		cache:     cache,
		hashFunc:  hashFunc,
		Dividends: make(map[string][]*Measurement),
		Splits:    make(map[string][]*Measurement),
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
	for idx, xx := range res[:len(res)-1] {
		next := res[idx+1]

		switch frequency {
		case FrequencyDaily:
			days = append(days, xx)
		case FrequencyWeekly:
			_, week := xx.ISOWeek()
			_, nextWeek := next.ISOWeek()
			if week != nextWeek {
				days = append(days, xx)
			}
		case FrequencyMonthly:
			if xx.Month() != next.Month() {
				days = append(days, xx)
			}
		case FrequencyAnnually:
			if xx.Year() != next.Year() {
				days = append(days, xx)
			}
		}
	}
	return days
}

// TradingDays returns a list of trading days between begin and end at the desired frequency
func (p *Pvdb) TradingDays(ctx context.Context, begin time.Time, end time.Time, frequency Frequency) []time.Time {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.TradingDays")
	defer span.End()

	tz := common.GetTimezone()

	subLog := log.With().Time("Begin", begin).Time("End", end).Str("Frequency", string(frequency)).Logger()

	res := make([]time.Time, 0, 252)
	if end.Before(begin) {
		subLog.Warn().Msg("end before begin in call to TradingDays")
		return res
	}

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		subLog.Error().Err(err).Stack().Msg("could not get transaction when querying trading days")
		return res
	}

	searchBegin := begin
	searchEnd := end
	searchBegin, searchEnd = adjustSearchDates(frequency, searchBegin, searchEnd)

	rows, err := trx.Query(ctx, "SELECT trading_day FROM trading_days WHERE market='us' AND trading_day BETWEEN $1 and $2 ORDER BY trading_day", searchBegin, searchEnd)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "database query failed")
		subLog.Error().Err(err).Stack().Msg("could not query trading days")
	}

	for rows.Next() {
		var dt time.Time
		if err = rows.Scan(&dt); err != nil {
			log.Error().Err(err).Stack().Msg("could not SCAN DB result")
		} else {
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
			res = append(res, dt)
		}
	}

	cnt := len(res) - 1

	if len(res) == 0 {
		span.SetStatus(codes.Error, "no trading days found")
		log.Panic().Stack().Msg("could not load trading days")
	}

	days := filterDays(frequency, res)

	daysFiltered := make([]time.Time, 0, 252)
	lastDay := res[cnt]
	if len(days) == 0 {
		subLog.Error().Msg("days array is empty")
		return daysFiltered
	}

	if !lastDay.Equal(days[len(days)-1]) {
		days = append(days, res[cnt])
	}

	// final filter to actual days
	for _, d := range days {
		if d.Equal(begin) || d.Equal(end) || (d.Before(end) && d.After(begin)) {
			daysFiltered = append(daysFiltered, d)
		}
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("could not commit transaction")
	}
	return daysFiltered
}

// Provider functions

func (p *Pvdb) DataType() string {
	return "security"
}

// uniqueStrings filters a list of string to only unique values
func uniqueStrings(strs []string) []string {
	unique := make(map[string]bool, len(strs))
	for _, v := range strs {
		unique[v] = true
	}
	uniqStrs := make([]string, len(unique))
	j := 0
	for k := range unique {
		uniqStrs[j] = k
		j++
	}
	return uniqStrs
}

func buildDataFrame(vals map[int]map[string]float64, symbols []string, tradingDays []time.Time, lastDate time.Time) *dataframe.DataFrame {
	symbolCnt := len(symbols)
	series := make([]dataframe.Series, 0, symbolCnt+1)
	series = append(series, dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Capacity: len(tradingDays)}, tradingDays))

	// build series
	vals2 := make(map[string][]float64, symbolCnt)
	for _, symbol := range symbols {
		vals2[symbol] = make([]float64, len(tradingDays))
	}

	for idx, k := range tradingDays {
		dayData, ok := vals[k.Year()*1000+k.YearDay()]
		if !ok {
			dayData = vals[lastDate.Year()*1000+k.YearDay()]
		}

		for _, symbol := range symbols {
			v, ok := dayData[symbol]
			if !ok {
				vals2[symbol][idx] = math.NaN()
			} else {
				vals2[symbol][idx] = v
			}
		}
	}

	// break arrays out of map in order to build the dataframe
	for _, symbol := range symbols {
		arr := vals2[symbol]
		series = append(series, dataframe.NewSeriesFloat64(symbol, &dataframe.SeriesInit{Capacity: len(arr)}, arr))
	}

	df := dataframe.NewDataFrame(series...)
	return df
}

func (p *Pvdb) GetDataForPeriod(ctx context.Context, symbols []string, metric Metric, frequency Frequency, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
	defer span.End()
	tz := common.GetTimezone()
	subLog := log.With().Strs("Symbols", symbols).Str("Metric", string(metric)).Str("Frequency", string(frequency)).Time("StartTime", begin).Time("EndTime", end).Logger()

	tradingDays := p.TradingDays(ctx, begin, end, frequency)

	// ensure symbols is a unique set
	symbols = uniqueStrings(symbols)

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Err(err).Msg(msg)
		return nil, err
	}

	// build SQL query
	args := make([]interface{}, len(symbols)+2)
	args[0] = begin
	args[1] = end

	tickerSet := make([]string, len(symbols))
	for idx, ticker := range symbols {
		tickerSet[idx] = fmt.Sprintf("$%d", idx+3)
		args[idx+2] = ticker
	}
	tickerArgs := strings.Join(tickerSet, ", ")
	sql := fmt.Sprintf("SELECT event_date, ticker, close, adj_close::double precision FROM eod WHERE ticker IN (%s) AND event_date BETWEEN $1 AND $2 ORDER BY event_date DESC, ticker", tickerArgs)

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Err(err).Str("SQL", sql).Msg(msg)
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return nil, err
	}

	// parse database rows
	vals := make(map[int]map[string]float64, len(symbols))

	var date time.Time
	var lastDate time.Time
	var ticker string
	var close float64
	var adjClose pgtype.Float8

	symbolCnt := len(symbols)

	for rows.Next() {
		err = rows.Scan(&date, &ticker, &close, &adjClose)

		p.cache[p.hashFunc(date, MetricClose, ticker)] = close
		if adjClose.Status == pgtype.Present {
			p.cache[p.hashFunc(date, MetricAdjustedClose, ticker)] = adjClose.Float
		}

		if err != nil {
			subLog.Error().Err(err).Msg("failed to load eod prices -- db query scan failed")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
		dateHash := date.Year()*1000 + date.YearDay()
		valMap, ok := vals[dateHash]
		if !ok {
			valMap = make(map[string]float64, symbolCnt)
			vals[dateHash] = valMap
		}

		switch metric {
		case MetricClose:
			valMap[ticker] = close
		case MetricAdjustedClose:
			valMap[ticker] = adjClose.Float
		default:
			span.SetStatus(codes.Error, "un-supported metric")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}

			log.Panic().Str("Metric", string(metric)).Msg("Unsupported metric type")
			return nil, ErrUnsupportedMetric
		}

		lastDate = date
	}

	if err := trx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("error committing transaction")
	}

	// preload splits & divs
	p.preloadCorporateActions(ctx, symbols, begin)

	// build dataframe
	df := buildDataFrame(vals, symbols, tradingDays, lastDate)
	return df, nil
}

func (p *Pvdb) preloadCorporateActions(ctx context.Context, tickerSet []string, start time.Time) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
	defer span.End()

	tz := common.GetTimezone()

	corporateTickerSet := make([]string, 0, len(tickerSet))
	for _, ticker := range tickerSet {
		if _, ok := p.Dividends[ticker]; !ok {
			corporateTickerSet = append(corporateTickerSet, ticker)
			p.Dividends[ticker] = make([]*Measurement, 0)
			p.Splits[ticker] = make([]*Measurement, 0)
		}
	}

	if len(corporateTickerSet) == 0 {
		log.Debug().Strs("Tickers", tickerSet).Msg("skipping preload of corporate actions because there are no additional tickers to preload")
		return // nothing needs to be loaded
	}

	log.Debug().Time("Start", start).Strs("Tickers", corporateTickerSet).Msg("pre-load from corporate actions")

	subLog := log.With().Strs("Symbols", corporateTickerSet).Time("StartTime", start).Logger()

	args := make([]interface{}, len(corporateTickerSet)+1)
	args[0] = start

	tickerPlaceholders := make([]string, len(corporateTickerSet))
	for idx, ticker := range corporateTickerSet {
		tickerPlaceholders[idx] = fmt.Sprintf("$%d", idx+2)
		args[idx+1] = ticker
	}
	corporateTickerArgs := strings.Join(tickerPlaceholders, ", ")
	sql := fmt.Sprintf("SELECT event_date, ticker, dividend, split_factor FROM eod WHERE ticker IN (%s) AND event_date >= $1 AND (dividend != 0 OR split_factor != 1.0) ORDER BY event_date DESC, ticker", corporateTickerArgs)

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "failed to get transaction for preloading corporate actions"
		span.SetStatus(codes.Error, msg)
		log.Warn().Err(err).Msg(msg)
		return
	}

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		msg := "failed to load eod prices -- db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Err(err).Str("SQL", sql).Msg(msg)
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}
		return
	}

	var date time.Time
	var ticker string
	var dividend float64
	var splitFactor float64

	for rows.Next() {
		err = rows.Scan(&date, &ticker, &dividend, &splitFactor)
		if err != nil {
			subLog.Error().Err(err).Msg("failed to load corporate actions -- db query scan failed")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}

			return
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
		divs := p.Dividends[ticker]
		if dividend != 0.0 {
			divs = append(divs, &Measurement{
				Date:  date,
				Value: dividend,
			})
		}

		splits := p.Splits[ticker]
		if splitFactor != 1.0 {
			splits = append(splits, &Measurement{
				Date:  date,
				Value: splitFactor,
			})
		}

		p.Dividends[ticker] = divs
		p.Splits[ticker] = splits
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("error committing transaction")
	}
}

func (p *Pvdb) GetLatestDataBefore(ctx context.Context, symbol string, metric Metric, before time.Time) (float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetLatestDataBefore")
	defer span.End()
	subLog := log.With().Str("Symbol", symbol).Str("Metric", string(metric)).Time("Before", before).Logger()

	tz := common.GetTimezone()

	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		span.RecordError(err)
		msg := "could not get a database transaction"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Err(err).Msg(msg)
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
		subLog.Error().Msg("un-supported metric requested")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}
		return math.NaN(), ErrUnsupportedMetric
	}

	sql := fmt.Sprintf("SELECT event_date, ticker, %s FROM eod WHERE ticker=$1 AND event_date <= $2 ORDER BY event_date DESC, ticker LIMIT 1", columns)

	// execute the query
	rows, err := trx.Query(ctx, sql, symbol, before)
	if err != nil {
		span.RecordError(err)
		msg := "db query failed"
		span.SetStatus(codes.Error, msg)
		subLog.Warn().Err(err).Msg(msg)
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return math.NaN(), err
	}

	var date time.Time
	var ticker string
	var val float64

	for rows.Next() {
		err = rows.Scan(&date, &ticker, &val)
		if err != nil {
			span.RecordError(err)
			msg := "db scan failed"
			span.SetStatus(codes.Error, msg)
			subLog.Warn().Err(err).Msg(msg)
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}
			return math.NaN(), err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
	}

	if err := trx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("could not commit transaction")
	}
	return val, err
}
