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
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

type pvdb struct {
	cache    map[string]float64
	hashFunc func(date time.Time, metric string, symbol string) string
}

// NewPVDB Create a new PVDB data provider
func NewPVDB(cache map[string]float64, hashFunc func(date time.Time, metric string, symbol string) string) *pvdb {
	return &pvdb{
		cache:    cache,
		hashFunc: hashFunc,
	}
}

// Date provider functions

// TradingDays returns a list of trading days between begin and end
func (p *pvdb) TradingDays(ctx context.Context, begin time.Time, end time.Time, frequency string) []time.Time {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.TradingDays")
	defer span.End()

	res := make([]time.Time, 0, 252)
	if end.Before(begin) {
		log.WithFields(log.Fields{
			"Begin":     begin,
			"End":       end,
			"Frequency": frequency,
		}).Warn("end before begin in call to TradingDays")
		return res
	}

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("could not get transaction when querying trading days")
	}

	searchBegin := begin
	searchEnd := end

	switch frequency {
	case FrequencyMonthly:
		searchBegin = searchBegin.AddDate(0, -1, 0)
		searchEnd = searchEnd.AddDate(0, 1, 0)
	case FrequencyAnnualy:
		searchBegin = searchBegin.AddDate(-1, 0, 0)
		searchEnd = searchEnd.AddDate(1, 0, 0)
	default:
		searchBegin = searchBegin.AddDate(0, 0, -7)
		searchEnd = searchEnd.AddDate(0, 0, 7)
	}

	rows, err := trx.Query(ctx, "SELECT trading_day FROM trading_days WHERE market='us' AND trading_day BETWEEN $1 and $2 ORDER BY trading_day", searchBegin, searchEnd)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "database query failed")
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("could not query trading days")
	}
	for rows.Next() {
		var dt time.Time
		if err = rows.Scan(&dt); err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Error("could not scan trading day value")
		} else {
			dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
			res = append(res, dt)
		}
	}

	cnt := len(res) - 1
	days := make([]time.Time, 0, 252)

	if len(res) == 0 {
		span.SetStatus(codes.Error, "no trading days found")
		log.Error("Could not load trading days")
		debug.PrintStack()
		os.Exit(-1)
	}

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
		case FrequencyAnnualy:
			if xx.Year() != next.Year() {
				days = append(days, xx)
			}
		}
	}

	lastDay := res[cnt]
	if !lastDay.Equal(days[len(days)-1]) {
		days = append(days, res[cnt])
	}

	// final filter to actual days
	daysFiltered := make([]time.Time, 0, 252)
	for _, d := range days {
		if d.Equal(begin) || d.Equal(end) || (d.Before(end) && d.After(begin)) {
			daysFiltered = append(daysFiltered, d)
		}
	}

	trx.Commit(ctx)
	return daysFiltered
}

// Provider functions

func (p *pvdb) DataType() string {
	return "security"
}

func (p *pvdb) GetDataForPeriod(ctx context.Context, symbols []string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetDataForPeriod")
	defer span.End()

	tradingDays := p.TradingDays(ctx, begin, end, frequency)

	// ensure symbols is a unique set
	uniqueSymbols := make(map[string]bool, len(symbols))
	for _, v := range symbols {
		uniqueSymbols[v] = true
	}
	symbols = make([]string, len(uniqueSymbols))
	j := 0
	for k := range uniqueSymbols {
		symbols[j] = k
		j++
	}

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to load eod prices -- could not get a database transaction")
		log.WithFields(log.Fields{
			"Symbol":    symbols,
			"Metric":    metric,
			"Frequency": frequency,
			"StartTime": begin.String(),
			"EndTime":   end.String(),
			"Error":     err,
		}).Warn("Failed to load eod prices -- could not get a database transaction")
		return nil, err
	}

	// build SQL query
	var columns string
	var adjusted bool
	rawMetric := metric
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
	case MetricAdjustedOpen:
		columns = "open AS val, dividend, split_factor"
		adjusted = true
		rawMetric = MetricOpen
	case MetricAdjustedHigh:
		columns = "high AS val, dividend, split_factor"
		adjusted = true
		rawMetric = MetricHigh
	case MetricAdjustedLow:
		columns = "low AS val, dividend, split_factor"
		adjusted = true
		rawMetric = MetricLow
	case MetricAdjustedClose:
		columns = "close AS val, dividend, split_factor"
		adjusted = true
		rawMetric = MetricClose
	case MetricDividendCash:
		columns = "dividend AS val"
	case MetricSplitFactor:
		columns = "split_factor AS val"
	default:
		span.SetStatus(codes.Error, "un-supported metric")
		trx.Rollback(ctx)
		return nil, errors.New("un-supported metric")
	}

	args := make([]interface{}, len(symbols)+2)
	args[0] = begin
	args[1] = end

	tickerSet := make([]string, len(symbols))
	for idx, ticker := range symbols {
		tickerSet[idx] = fmt.Sprintf("$%d", idx+3)
		args[idx+2] = ticker
	}
	tickerArgs := strings.Join(tickerSet, ", ")

	sql := fmt.Sprintf("SELECT event_date, ticker, %s FROM eod WHERE ticker IN (%s) AND event_date BETWEEN $1 AND $2 ORDER BY event_date DESC, ticker", columns, tickerArgs)

	// execute the query
	rows, err := trx.Query(ctx, sql, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to load eod prices -- db query failed")
		log.WithFields(log.Fields{
			"Symbol":    symbols,
			"Metric":    metric,
			"Frequency": frequency,
			"StartTime": begin.String(),
			"EndTime":   end.String(),
			"Error":     err,
		}).Warn("Failed to load eod prices -- db query failed")
		trx.Rollback(context.Background())
		return nil, err
	}

	// build the dataframe
	vals := make(map[int]map[string]float64, len(symbols))
	adjustFactor := make(map[string]float64, len(symbols))
	for _, s := range symbols {
		adjustFactor[s] = 1.0
	}

	var date time.Time
	var lastDate time.Time
	var ticker string
	var val float64
	var div float64
	var split float64

	symbolCnt := len(symbols)

	for rows.Next() {
		if adjusted {
			err = rows.Scan(&date, &ticker, &val, &div, &split)
		} else {
			err = rows.Scan(&date, &ticker, &val)
		}

		p.cache[p.hashFunc(date, rawMetric, ticker)] = val

		if err != nil {
			log.WithFields(log.Fields{
				"Query":     sql,
				"Symbol":    symbols,
				"Metric":    metric,
				"Frequency": frequency,
				"StartTime": begin.String(),
				"EndTime":   end.String(),
				"Error":     err,
			}).Warn("Failed to load eod prices -- db query scan failed")
			trx.Rollback(context.Background())
			return nil, err
		}

		v2 := val / adjustFactor[ticker]
		if adjusted {
			// CRSP adjustment calculations
			// see: http://crsp.org/products/documentation/crsp-calculations
			if val > 0 {
				adjustFactor[ticker] *= (1 + (div / val)) * split
			} else {
				adjustFactor[ticker] = 1
			}
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
		dateHash := date.Year()*1000 + date.YearDay()
		valMap, ok := vals[dateHash]
		if !ok {
			valMap = make(map[string]float64, symbolCnt)
			vals[dateHash] = valMap
		}

		valMap[ticker] = v2
		if adjusted {
			p.cache[p.hashFunc(date, metric, ticker)] = v2
		}

		lastDate = date
	}

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
	trx.Commit(ctx)

	return df, err
}

func (p *pvdb) GetLatestDataBefore(ctx context.Context, symbol string, metric string, before time.Time) (float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "pvdb.GetLatestDataBefore")
	defer span.End()

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	trx, err := database.TrxForUser("pvuser")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not get a database transaction")
		log.WithFields(log.Fields{
			"Symbol": symbol,
			"Metric": metric,
			"Error":  err,
		}).Warn("Failed to load eod prices -- could not get a database transaction")
		return math.NaN(), err
	}

	// build SQL query
	var columns string
	var adjusted bool
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
	case MetricAdjustedOpen:
		columns = "open AS val, dividend, split_factor"
		adjusted = true
	case MetricAdjustedHigh:
		columns = "high AS val, dividend, split_factor"
		adjusted = true
	case MetricAdjustedLow:
		columns = "low AS val, dividend, split_factor"
		adjusted = true
	case MetricAdjustedClose:
		columns = "close AS val, dividend, split_factor"
		adjusted = true
	case MetricDividendCash:
		columns = "dividend AS val"
	case MetricSplitFactor:
		columns = "split_factor AS val"
	default:
		span.SetStatus(codes.Error, "un-supported metric")
		trx.Rollback(ctx)
		return math.NaN(), errors.New("un-supported metric")
	}

	sql := fmt.Sprintf("SELECT event_date, ticker, %s FROM eod WHERE ticker=$1 AND event_date <= $2 ORDER BY event_date DESC, ticker LIMIT 1", columns)

	// execute the query
	rows, err := trx.Query(ctx, sql, symbol, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "db query failed")
		log.WithFields(log.Fields{
			"Symbol": symbol,
			"Metric": metric,
			"Error":  err,
		}).Warn("Failed to load eod prices -- db query failed")
		trx.Rollback(ctx)
		return math.NaN(), err
	}

	var date time.Time
	var ticker string
	var val float64
	var div float64
	var split float64

	for rows.Next() {
		if adjusted {
			err = rows.Scan(&date, &ticker, &val, &div, &split)
		} else {
			err = rows.Scan(&date, &ticker, &val)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "db scan failed")
			log.WithFields(log.Fields{
				"Symbol": symbol,
				"Metric": metric,
				"Error":  err,
			}).Warn("Failed to load eod prices -- db query scan failed")
			trx.Rollback(context.Background())
			return math.NaN(), err
		}

		date = time.Date(date.Year(), date.Month(), date.Day(), 16, 0, 0, 0, tz)
	}

	trx.Commit(ctx)
	return val, err
}
