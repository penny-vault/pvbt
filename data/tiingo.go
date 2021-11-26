// Copyright 2021 JD Fergason
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
	"main/common"
	"main/dfextras"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	imports "github.com/rocketlaunchr/dataframe-go/imports"
	log "github.com/sirupsen/logrus"
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
func (t *tiingo) LastTradingDay(forDate time.Time, frequency string) (time.Time, error) {
	symbol := "SPY"
	url := fmt.Sprintf("%s/tiingo/daily/%s/prices?startDate=%s&endDate=%s&resampleFreq=%s&token=%s", tiingoAPI, symbol, forDate.Format("2006-01-02"), forDate.Format("2006-01-02"), frequency, t.apikey)

	resp, err := http.Get(url)
	if err != nil {
		log.WithFields(log.Fields{
			"Function":  "data/tiingo.go:LastTradingDay",
			"ForDate":   forDate,
			"Frequency": frequency,
			"Error":     err,
		}).Error("HTTP error response")
		return time.Time{}, err
	}

	if resp.StatusCode >= 400 {
		log.WithFields(log.Fields{
			"Function":   "data/tiingo.go:LastTradingDay",
			"ForDate":    forDate,
			"Frequency":  frequency,
			"StatusCode": resp.StatusCode,
			"Error":      err,
		}).Error("HTTP error response")
		return time.Time{}, fmt.Errorf("HTTP request returned invalid status code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.WithFields(log.Fields{
			"Function":  "data/tiingo.go:LastTradingDay",
			"ForDate":   forDate,
			"Frequency": frequency,
			"Body":      string(body),
			"Error":     err,
		}).Error("Failed to read HTTP body")
		return time.Time{}, err
	}

	jsonResp := []tiingoJSONResponse{}
	err = json.Unmarshal(body, &jsonResp)
	if err != nil {
		log.WithFields(log.Fields{
			"Function":  "data/tiingo.go:LastTradingDay",
			"ForDate":   forDate,
			"Frequency": frequency,
			"Body":      string(body),
			"Error":     err,
		}).Error("Failed to parse JSON")
		return time.Time{}, err
	}

	tz, err := time.LoadLocation("America/New_York") // New York is the reference time
	if err != nil {
		return time.Time{}, err
	}

	if len(jsonResp) > 0 {
		dtParts := strings.Split(jsonResp[0].Date, "T")
		if len(dtParts) == 0 {
			log.WithFields(log.Fields{
				"Function":  "data/tiingo.go:LastTradingDay",
				"ForDate":   forDate,
				"Frequency": frequency,
				"DateStr":   jsonResp[0].Date,
				"Error":     err,
			}).Error("Invalid date format")
			return time.Time{}, errors.New("invalid date format")
		}
		lastDay, err := time.ParseInLocation("2006-01-02", dtParts[0], tz)
		if err != nil {
			log.WithFields(log.Fields{
				"Function":   "data/tiingo.go:LastTradingDay",
				"ForDate":    forDate,
				"Frequency":  frequency,
				"StatusCode": resp.StatusCode,
				"Error":      err,
			}).Error("Cannot parse date")
			return time.Time{}, err
		}

		return lastDay, nil
	}

	return time.Time{}, errors.New("no data returned")
}

// LastTradingDayOfWeek return the last trading day of the week
func (t *tiingo) LastTradingDayOfWeek(forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(forDate, "weekly")
}

// LastTradingDayOfMonth return the last trading day of the month
func (t *tiingo) LastTradingDayOfMonth(forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(forDate, "monthly")
}

// LastTradingDayOfYear return the last trading day of the year
func (t *tiingo) LastTradingDayOfYear(forDate time.Time) (time.Time, error) {
	return t.LastTradingDay(forDate, "annually")
}

// Provider functions

func (t *tiingo) DataType() string {
	return "security"
}

func (t *tiingo) GetDataForPeriod(symbols []string, metric string, frequency string, begin time.Time, end time.Time) (data *dataframe.DataFrame, err error) {
	res := make([]*dataframe.DataFrame, 0, len(symbols))
	errs := []error{}
	ch := make(chan quoteResult)

	for idx, chunk := range partitionArray(symbols, 10) {
		log.Info("GetMultipleData run chunk %d of %d at %s\n", idx, len(symbols)/10, time.Now().Format("15:04:05"))
		for ii := range chunk {
			go tiingoDownloadWorker(ch, strings.ToUpper(chunk[ii]), metric, frequency, begin, end, t)
		}

		for range chunk {
			v := <-ch
			if v.Err == nil {
				res = append(res, v.Data)
			} else {
				log.WithFields(log.Fields{
					"Ticker": v.Ticker,
					"Error":  v.Err,
				}).Warn("Cannot download ticker data")
				errs = append(errs, v.Err)
			}
		}
	}

	if len(errs) != 0 {
		return nil, errs[0]
	}

	return dfextras.MergeAndFill(context.Background(), res...)
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
	validFrequencies := map[string]bool{
		FrequencyDaily:   true,
		FrequencyWeekly:  true,
		FrequencyMonthly: true,
		FrequencyAnnualy: true,
	}

	// var t1, t2, t3, t4 time.Time

	if _, ok := validFrequencies[frequency]; !ok {
		log.WithFields(log.Fields{
			"Frequency": frequency,
			"Symbol":    symbol,
			"Metric":    metric,
			"StartTime": begin.String(),
			"EndTime":   end.String(),
		}).Debug("Invalid frequency provided")
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

	log.WithFields(log.Fields{
		"symbol":    symbol,
		"metric":    metric,
		"frequency": frequency,
		"begin":     begin,
		"end":       end,
		"cached":    ok,
	}).Debug("load data from tiingo")

	if !ok {
		// t1 = time.Now()
		resp, err := http.Get(url)
		// t2 = time.Now()

		if err != nil {
			log.WithFields(log.Fields{
				"Url":       url,
				"Symbol":    symbol,
				"Metric":    metric,
				"Frequency": frequency,
				"StartTime": begin.String(),
				"EndTime":   end.String(),
				"Error":     err,
			}).Warn("Failed to load eod prices")
			return nil, err
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.WithFields(log.Fields{
				"Url":        url,
				"Symbol":     symbol,
				"Metric":     metric,
				"Frequency":  frequency,
				"StartTime":  begin.String(),
				"EndTime":    end.String(),
				"Error":      err,
				"StatusCode": resp.StatusCode,
			}).Warn("Failed to load eod prices -- reading body failed")
			return nil, err
		}

		if resp.StatusCode >= 400 {
			log.WithFields(log.Fields{
				"Url":        url,
				"Symbol":     symbol,
				"Metric":     metric,
				"Frequency":  frequency,
				"StartTime":  begin.String(),
				"EndTime":    end.String(),
				"Body":       string(body),
				"StatusCode": resp.StatusCode,
			}).Warn("Failed to load eod prices")
			return nil, fmt.Errorf("HTTP request returned invalid status code: %d", resp.StatusCode)
		}
		// t3 = time.Now()

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
		// t4 = time.Now()

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
	case MetricAdjustedOpen:
		valueSeriesIdx, err := res.NameToColumn("adjOpen")
		if err != nil {
			return nil, errors.New("adjusted open metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricAdjustedHigh:
		valueSeriesIdx, err := res.NameToColumn("adjHigh")
		if err != nil {
			return nil, errors.New("adjusted high metric not found")
		}
		valueSeries = res.Series[valueSeriesIdx].Copy()
	case MetricAdjustedLow:
		valueSeriesIdx, err := res.NameToColumn("adjLow")
		if err != nil {
			return nil, errors.New("adjusted low metric not found")
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

	/*
		log.WithFields(log.Fields{
			"HttpRequest": t2.Sub(t1).Round(time.Millisecond),
			"ParseCSV":    t4.Sub(t3).Round(time.Millisecond),
			"Symbol":      symbol,
			"Frequency":   frequency,
		}).Debug("TargetPortfolio runtimes")
	*/

	return df, err
}
