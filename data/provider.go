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
	"fmt"
	"main/common"
	"math"
	"strings"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

// Provider interface for retrieving quotes
type Provider interface {
	DataType() string
	GetDataForPeriod(symbols []string, metric string, frequency string, begin time.Time, end time.Time) (*dataframe.DataFrame, error)
	GetLatestDataBefore(symbol string, metric string, before time.Time) (float64, error)
}

type DateProvider interface {
	TradingDays(begin time.Time, end time.Time, frequency string) []time.Time
}

const (
	FrequencyDaily   = "Daily"
	FrequencyWeekly  = "Weekly"
	FrequencyMonthly = "Monthly"
	FrequencyAnnualy = "Annualy"
)

const (
	MetricOpen           = "Open"
	MetricLow            = "Low"
	MetricHigh           = "High"
	MetricClose          = "Close"
	MetricVolume         = "Volume"
	MetricAdjustedOpen   = "AdjustedOpen"
	MetricAdjustedLow    = "AdjustedLow"
	MetricAdjustedHigh   = "AdjustedHigh"
	MetricAdjustedClose  = "AdjustedClose"
	MetricAdjustedVolume = "AdjustedVolume"
	MetricDividendCash   = "DividendCash"
	MetricSplitFactor    = "SplitFactor"
)

// Manager data manager type
type Manager struct {
	Begin           time.Time
	End             time.Time
	Frequency       string
	cache           map[string]float64
	lastCache       map[string]float64
	credentials     map[string]string
	providers       map[string]Provider
	dateProvider    DateProvider
	lastRiskFreeIdx int
}

var riskFreeRate *dataframe.DataFrame

// InitializeDataManager download risk free data
func InitializeDataManager() {
	fred := NewFred()
	var err error
	riskFreeRate, err = fred.GetDataForPeriod([]string{"DGS3MO"}, MetricClose, FrequencyDaily,
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Now())
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Fatal("Cannot load risk free rate")
	}

	// schedule a timer to update riskFreeRate in 24 hours
	refreshTimer := time.NewTimer(24 * time.Hour)
	go func() {
		<-refreshTimer.C
		log.Info("Refreshing risk free rate")
		InitializeDataManager()
	}()
}

// NewManager create a new data manager
func NewManager(credentials map[string]string) Manager {
	var m = Manager{
		Frequency:   FrequencyMonthly,
		cache:       make(map[string]float64, 1_000_000),
		lastCache:   make(map[string]float64, 10_000),
		credentials: credentials,
		providers:   map[string]Provider{},
	}

	// Create Tiingo API
	if val, ok := credentials["tiingo"]; ok {
		tiingo := NewTiingo(val)
		m.RegisterDataProvider(tiingo)
	} else {
		log.Warn("No tiingo API key provided")
	}

	pvdb := NewPVDB()
	m.RegisterDataProvider(pvdb)
	m.dateProvider = pvdb

	// Create FRED API
	fred := NewFred()
	m.RegisterDataProvider(fred)

	return m
}

// RegisterDataProvider add a data provider to the system
func (m *Manager) RegisterDataProvider(p Provider) {
	m.providers[p.DataType()] = p
}

// RiskFreeRate Get the risk free rate for given date
func (m *Manager) RiskFreeRate(t time.Time) float64 {
	start := m.lastRiskFreeIdx
	row := riskFreeRate.Row(m.lastRiskFreeIdx, true, dataframe.SeriesName)
	currDate := row[common.DateIdx].(time.Time)
	// check if the requestsed date is before the last requested date
	if t.Before(currDate) {
		start = 0
	}

	var ret float64
	iterator := riskFreeRate.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   start,
		Step:         1,
		DontReadLock: true})
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		if vals["DGS3MO"] != nil && !math.IsNaN(vals["DGS3MO"].(float64)) {
			m.lastRiskFreeIdx = *row
			ret = vals["DGS3MO"].(float64)
		}

		dt := vals[common.DateIdx].(time.Time)
		if dt.Equal(t) || dt.After(t) {
			break
		}
	}

	return ret
}

// GetDataFrame get a dataframe for the requested symbol
func (m *Manager) GetDataFrame(metric string, symbols ...string) (*dataframe.DataFrame, error) {
	res, err := m.providers["security"].GetDataForPeriod(symbols, metric, m.Frequency, m.Begin, m.End)
	return res, err
}

func (m *Manager) Fetch(begin time.Time, end time.Time, metric string, symbols ...string) error {
	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
	begin = time.Date(begin.Year(), begin.Month(), begin.Day(), 0, 0, 0, 0, tz)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, tz)
	res, err := m.providers["security"].GetDataForPeriod(symbols, metric, FrequencyDaily, begin, end)
	if err != nil {
		log.Warn(err)
		return err
	}

	iterator := res.ValuesIterator(dataframe.ValuesOptions{
		InitialRow:   0,
		Step:         1,
		DontReadLock: true})

	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		d := vals[common.DateIdx].(time.Time)
		for _, s := range symbols {
			key := buildHashKey(d, metric, s)
			if vals[s] != nil {
				m.cache[key] = vals[s].(float64)
			} else {
				m.cache[key] = math.NaN()
			}
		}
	}
	return nil
}

func (m *Manager) Get(date time.Time, metric string, symbol string) (float64, error) {
	symbol = strings.ToUpper(symbol)
	key := buildHashKey(date, metric, symbol)
	val, ok := m.cache[key]
	if !ok {
		tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
		end := time.Date(date.Year(), date.Month()+6, date.Day(), 0, 0, 0, 0, tz)
		err := m.Fetch(date, end, metric, symbol)
		if err != nil {
			return 0, err
		}
		val, ok = m.cache[key]
		if !ok {
			return 0, fmt.Errorf("could not load %s for symbol %s on %s", metric, symbol, date)
		}
	}
	return val, nil
}

func (m *Manager) GetLatestDataBefore(symbol string, metric string, before time.Time) (float64, error) {
	symbol = strings.ToUpper(symbol)
	var err error
	val, ok := m.lastCache[symbol]
	if !ok {
		val, err = m.providers["security"].GetLatestDataBefore(symbol, metric, before)
		if err != nil {
			log.Warn(err)
			return math.NaN(), err
		}
		m.lastCache[symbol] = val
	}
	return val, nil
}

func (m *Manager) TradingDays(since time.Time, through time.Time) []time.Time {
	return m.dateProvider.TradingDays(since, through, FrequencyDaily)
}

func buildHashKey(date time.Time, metric string, symbol string) string {
	// Hash key like 2021340:split:VFINX
	return fmt.Sprintf("%d%d:%s:%s", date.Year(), date.YearDay(), metric, symbol)
}
