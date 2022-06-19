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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/dfextras"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/goccy/go-json"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	imports "github.com/rocketlaunchr/dataframe-go/imports"
	"github.com/rs/zerolog/log"
)

type tiingo struct {
	apikey string
	cache  map[string]*dataframe.DataFrame
	lock   sync.RWMutex
}

type tiingoJSONResponse struct {
	Date        string  `json:"date"`
	Close       float64 `json:"close"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Open        float64 `json:"open"`
	Volume      int64   `json:"volume"`
	AdjClose    float64 `json:"adjClose"`
	AdjHigh     float64 `json:"adjHigh"`
	AdjLow      float64 `json:"adjLow"`
	AdjOpen     float64 `json:"adjOpen"`
	AdjVolume   int64   `json:"adjVolume"`
	DivCash     float64 `json:"divCash"`
	SplitFactor float64 `json:"splitFactor"`
}

var tiingoAPI = "https://api.tiingo.com"

// NewTiingo Create a new Tiingo data provider
func NewTiingo(key string) *tiingo {
	return &tiingo{
		apikey: key,
		cache:  make(map[string]*dataframe.DataFrame),
	}
}

// Date provider functions

// LastTradingDay return the last trading day for the requested frequency
func (t *tiingo) LastTradingDay(ctx context.Context, forDate time.Time, frequency string) (time.Time, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "tiingo.LastTradingDay")
	defer span.End()

	subLog := log.With().Str("Frequency", frequency).Time("ForDate", forDate).Logger()

	symbol := "SPY"
	url := fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&resampleFreq=%s&token=%s", tiingoAPI, symbol, forDate.Format("2006-01-02"), forDate.Format("2006-01-02"), frequency, t.apikey)

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "Url",
			Value: attribute.StringValue(fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&resampleFreq=%s", tiingoAPI, symbol, forDate.Format("2006-01-02"), forDate.Format("2006-01-02"), frequency)),
		},
		attribute.KeyValue{
			Key:   "Symbol",
			Value: attribute.StringValue(symbol),
		},
	)

	resp, err := http.Get(url)
	if err != nil {
		span.RecordError(err)
		msg := "tiingo http request failed"
		span.SetStatus(codes.Error, msg)
		subLog.Error().Err(err).Msg(msg)
		return time.Time{}, err
	}

	if resp.StatusCode >= 400 {
		span.SetAttributes(attribute.KeyValue{
			Key:   "StatusCode",
			Value: attribute.IntValue(resp.StatusCode),
		})
		msg := "tiingo returned invalid response code"
		span.SetStatus(codes.Error, msg)
		subLog.Error().Int("HTTPResponseStatusCode", resp.StatusCode).Msg(msg)
		return time.Time{}, fmt.Errorf("HTTP request returned invalid status code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		msg := "could not read tiingo body"
		span.SetStatus(codes.Error, msg)
		subLog.Error().Bytes("Body", body).Err(err).Msg(msg)
		return time.Time{}, err
	}

	jsonResp := []tiingoJSONResponse{}
	err = json.Unmarshal(body, &jsonResp)
	if err != nil {
		span.RecordError(err)
		msg := "could not unmarshal json"
		span.SetStatus(codes.Error, msg)
		subLog.Error().Err(err).Bytes("Body", body).Msg(msg)
		return time.Time{}, err
	}

	tz, err := time.LoadLocation("America/New_York") // New York is the reference time
	if err != nil {
		subLog.Error().Err(err).Msg("could not load nyc timezone")
		return time.Time{}, err
	}

	if len(jsonResp) > 0 {
		dtParts := strings.Split(jsonResp[0].Date, "T")
		if len(dtParts) == 0 {
			msg := "invalid date format"
			span.SetStatus(codes.Error, msg)
			subLog.Error().Str("DateStr", jsonResp[0].Date).Msg(msg)
			return time.Time{}, errors.New(msg)
		}
		lastDay, err := time.ParseInLocation("2006-01-02", dtParts[0], tz)
		if err != nil {
			span.RecordError(err)
			msg := "cannot parse date string"
			span.SetStatus(codes.Error, msg)
			subLog.Error().Err(err).Int("HTTPResponseStatusCode", resp.StatusCode).Msg(msg)
			return time.Time{}, err
		}

		return lastDay, nil
	}

	return time.Time{}, errors.New("no data returned")
}

// LastTradingDayOfWeek return the last trading day of the week
func (t *tiingo) LastTradingDayOfWeek(ctx context.Context, forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(ctx, forDate, "weekly")
}

// LastTradingDayOfMonth return the last trading day of the month
func (t *tiingo) LastTradingDayOfMonth(ctx context.Context, forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(ctx, forDate, "monthly")
}

// LastTradingDayOfYear return the last trading day of the year
func (t *tiingo) LastTradingDayOfYear(ctx context.Context, forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(ctx, forDate, "annually")
}

// Provider functions

func (t *tiingo) DataType() string {
	return "security"
}

func (t *tiingo) GetLatestDataBefore(ctx context.Context, symbol string, metric string, before time.Time) (float64, error) {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "tiingo.GetLatestDataBefore")
	defer span.End()

	subLog := log.With().Str("Symbol", symbol).Str("Metric", metric).Time("Before", before).Logger()

	// build URL to get data
	url := fmt.Sprintf("%s/tiingo/daily/%s/prices?endDate=%s&token=%s", tiingoAPI, symbol, before.Format("2006-01-02"), t.apikey)
	// t1 = time.Now()
	resp, err := http.Get(url)
	// t2 = time.Now()

	if err != nil {
		subLog.Warn().Str("Url", url).Msg("failed to load eod prices")
		return math.NaN(), err
	}

	m := make([]tiingoJSONResponse, 0)
	err = json.NewDecoder(resp.Body).Decode(&m)
	if err != nil {
		span.RecordError(err)
		msg := "could not decode JSON"
		span.SetStatus(codes.Error, msg)
		subLog.Error().Err(err).Msg(msg)
		return math.NaN(), err
	}

	err = nil

	if len(m) == 0 {
		span.SetStatus(codes.Error, "no results returned")
		subLog.Error().Msg("no results returned")
		return math.NaN(), errors.New("no results returned")
	}

	last := m[len(m)-1]

	switch metric {
	case MetricOpen:
		return last.Open, nil
	case MetricHigh:
		return last.High, nil
	case MetricLow:
		return last.Low, nil
	case MetricClose:
		return last.Close, nil
	case MetricVolume:
		return float64(last.Volume), nil
	case MetricAdjustedClose:
		return last.AdjClose, nil
	case MetricDividendCash:
		return last.DivCash, nil
	case MetricSplitFactor:
		return last.SplitFactor, nil
	default:
		span.SetStatus(codes.Error, "un-supported metric")
		subLog.Error().Msg("un-supported metric")
		return math.NaN(), errors.New("un-supported metric")
	}
}

func (t *tiingo) GetDataForPeriod(ctx context.Context, symbols []string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "tiingo.GetDataForPeriod")
	defer span.End()

	subLog := log.With().Strs("Symbols", symbols).Str("Metric", metric).Str("Frequency", frequency).Time("Begin", begin).Time("End", end).Logger()

	res := make([]*dataframe.DataFrame, 0, len(symbols))
	errs := []error{}
	ch := make(chan quoteResult)

	for idx, chunk := range partitionArray(symbols, 10) {
		subLog.Info().Int("Chunk", idx).Int("TotalChunks", len(symbols)/10).Msg("GetMultipleData run chunk")
		for ii := range chunk {
			go tiingoDownloadWorker(ch, strings.ToUpper(chunk[ii]), metric, frequency, begin, end, t)
		}

		for range chunk {
			v := <-ch
			if v.Err == nil {
				res = append(res, v.Data)
			} else {
				subLog.Warn().Err(v.Err).Str("Ticker", v.Ticker).Msg("cannot download ticker data")
				errs = append(errs, v.Err)
			}
		}
	}

	if len(errs) != 0 {
		return nil, errs[0]
	}

	return dfextras.MergeAndFill(ctx, res...)
}

func tiingoDownloadWorker(result chan<- quoteResult, symbol string, metric string, frequency string, begin time.Time, end time.Time, t *tiingo) {
	df, err := t.loadDataForPeriod(symbol, metric, frequency, begin, end)
	res := quoteResult{
		Ticker: symbol,
		Data:   df,
		Err:    err,
	}
	result <- res
}

func (t *tiingo) loadDataForPeriod(symbol string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	subLog := log.With().Str("Symbol", symbol).Str("Metric", metric).Str("Frequency", frequency).Time("Begin", begin).Time("End", end).Logger()
	validFrequencies := map[string]bool{
		FrequencyDaily:   true,
		FrequencyWeekly:  true,
		FrequencyMonthly: true,
		FrequencyAnnualy: true,
	}

	if _, ok := validFrequencies[frequency]; !ok {
		subLog.Debug().Msg("invalid frequency provided")
		return nil, fmt.Errorf("invalid frequency '%s'", frequency)
	}

	// build URL to get data
	var url string
	nullTime := time.Time{}
	if begin.Equal(nullTime) || end.Equal(nullTime) {
		url = fmt.Sprintf("%s/tiingo/daily/%s/prices?format=csv&resampleFreq=%s&token=%s", tiingoAPI, symbol, frequency, t.apikey)
	} else {
		url = fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&format=csv&resampleFreq=%s&token=%s", tiingoAPI, symbol, begin.Format("2006-01-02"), end.Format("2006-01-02"), frequency, t.apikey)
	}

	var res *dataframe.DataFrame
	t.lock.RLock()
	res, ok := t.cache[url]
	t.lock.RUnlock()

	subLog.Debug().Bool("Cached", ok).Msg("load data from tiingo")

	if !ok {
		resp, err := http.Get(url)

		if err != nil {
			subLog.Warn().Err(err).Str("Url", url).Msg("failed to load eod prices")
			return nil, err
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			subLog.Warn().Err(err).Str("Url", url).Int("HTTPResponseStatusCode", resp.StatusCode).Msg("read eod price body failed")
			return nil, err
		}

		if resp.StatusCode >= 400 {
			subLog.Warn().Err(err).Str("Url", url).Int("HTTPResponseStatusCode", resp.StatusCode).Bytes("Body", body).Msg("tiingo request failed")
			return nil, fmt.Errorf("HTTP request returned invalid status code: %d", resp.StatusCode)
		}

		floatConverter := imports.Converter{
			ConcreteType: float64(0),
			ConverterFunc: func(in interface{}) (interface{}, error) {
				v, err := strconv.ParseFloat(in.(string), 64)
				if err != nil {
					return math.NaN(), nil
				}
				return v, nil
			},
		}

		tz, err := time.LoadLocation("America/New_York") // New York is the reference time
		if err != nil {
			return nil, err
		}

		res, err = imports.LoadFromCSV(context.TODO(), bytes.NewReader(body), imports.CSVLoadOptions{
			DictateDataType: map[string]interface{}{
				"date": imports.Converter{
					ConcreteType: time.Time{},
					ConverterFunc: func(in interface{}) (interface{}, error) {
						dt, err := time.ParseInLocation("2006-01-02", in.(string), tz)
						if err != nil {
							return nil, err
						}
						dt = dt.Add(time.Hour * 16)
						return dt, nil
					},
				},
				"open":        floatConverter,
				"high":        floatConverter,
				"low":         floatConverter,
				"close":       floatConverter,
				"volume":      floatConverter,
				"adjOpen":     floatConverter,
				"adjHigh":     floatConverter,
				"adjLow":      floatConverter,
				"adjClose":    floatConverter,
				"adjVolume":   floatConverter,
				"divCash":     floatConverter,
				"splitFactor": floatConverter,
			},
		})

		if err != nil {
			return nil, err
		}

		t.lock.Lock()
		t.cache[url] = res
		t.lock.Unlock()
	}

	err = nil
	var timeSeries dataframe.Series
	var valueSeries dataframe.Series

	timeSeriesIdx, err := res.NameToColumn("date")
	if err != nil {
		return nil, errors.New("cannot find time series")
	}

	timeSeries = res.Series[timeSeriesIdx].Copy()
	timeSeries.Rename(common.DateIdx)

	switch metric {
	case MetricOpen:
		valueSeriesIdx, err := res.NameToColumn("open")
		if err != nil {
			return nil, errors.New("open metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricHigh:
		valueSeriesIdx, err := res.NameToColumn("high")
		if err != nil {
			return nil, errors.New("high metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricLow:
		valueSeriesIdx, err := res.NameToColumn("low")
		if err != nil {
			return nil, errors.New("low metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricClose:
		valueSeriesIdx, err := res.NameToColumn("close")
		if err != nil {
			return nil, errors.New("close metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricVolume:
		valueSeriesIdx, err := res.NameToColumn("volume")
		if err != nil {
			return nil, errors.New("volume metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricAdjustedClose:
		valueSeriesIdx, err := res.NameToColumn("adjClose")
		if err != nil {
			return nil, errors.New("adjusted close metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricDividendCash:
		valueSeriesIdx, err := res.NameToColumn("divCash")
		if err != nil {
			return nil, errors.New("dividend metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricSplitFactor:
		valueSeriesIdx, err := res.NameToColumn("splitFactor")
		if err != nil {
			return nil, errors.New("split factor metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	default:
		return nil, errors.New("un-supported metric")
	}

	if err != nil {
		return nil, err
	}

	valueSeries.Rename(symbol)
	df := dataframe.NewDataFrame(timeSeries, valueSeries)

	return df, err
}
