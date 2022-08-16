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
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	dataframe "github.com/jdfergason/dataframe-go"
	"github.com/rs/zerolog/log"
)

const (
	CashAsset = "$CASH"
)

var (
	CashSecurity = Security{
		Ticker: CashAsset,
	}
)

// Provider interface for retrieving quotes
type Provider interface {
	DataType() string
	GetDataForPeriod(ctx context.Context, securities []*Security, metric Metric, frequency Frequency, begin time.Time, end time.Time) (*dataframe.DataFrame, error)
	GetLatestDataBefore(ctx context.Context, security *Security, metric Metric, before time.Time) (float64, error)
}

type DateProvider interface {
	TradingDays(ctx context.Context, begin time.Time, end time.Time, frequency Frequency) ([]time.Time, error)
}

type Frequency string

const (
	FrequencyDaily    Frequency = "Daily"
	FrequencyWeekly   Frequency = "Weekly"
	FrequencyMonthly  Frequency = "Monthly"
	FrequencyAnnually Frequency = "Annually"
)

type Metric string

const (
	MetricOpen          Metric = "Open"
	MetricLow           Metric = "Low"
	MetricHigh          Metric = "High"
	MetricClose         Metric = "Close"
	MetricVolume        Metric = "Volume"
	MetricAdjustedClose Metric = "AdjustedClose"
	MetricDividendCash  Metric = "DividendCash"
	MetricSplitFactor   Metric = "SplitFactor"
)

type Measurement struct {
	Date  time.Time
	Value float64
}

// Manager data manager type
type Manager struct {
	Begin           time.Time
	End             time.Time
	Frequency       Frequency
	cache           map[string]float64
	lastCache       map[string]float64
	providers       map[string]Provider
	dateProvider    DateProvider
	lastRiskFreeIdx int
}

var (
	ErrMetricDoesNotExist = errors.New("requested metric does not exist for symbol on date")
)

var riskFreeRate *dataframe.DataFrame

// InitializeDataManager download risk free data
func InitializeDataManager() {
	pvdb := NewPVDB(map[string]float64{}, buildHashKey)
	var err error
	dgs3mo := []*Security{
		{
			CompositeFigi: "PVGG06TNP6J8",
			Ticker:        "DGS3MO",
		},
	}
	riskFreeRate, err = pvdb.GetDataForPeriod(context.Background(), dgs3mo, MetricClose, FrequencyDaily,
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Now())
	if err != nil {
		log.Panic().Err(err).Msg("cannot load risk free rate")
	}

	// schedule a timer to update riskFreeRate in 24 hours
	refreshTimer := time.NewTimer(24 * time.Hour)
	go func() {
		<-refreshTimer.C
		log.Info().Msg("refreshing risk free rate")
		InitializeDataManager()
	}()
}

// NewManager create a new data manager
func NewManager() *Manager {
	var m = Manager{
		Frequency: FrequencyMonthly,
		cache:     make(map[string]float64, 1_000_000),
		lastCache: make(map[string]float64, 10_000),
		providers: map[string]Provider{},
	}

	pvdb := NewPVDB(m.cache, buildHashKey)
	m.RegisterDataProvider(pvdb)
	m.dateProvider = pvdb

	return &m
}

// Get dividend map
func (m *Manager) GetDividends() map[Security][]*Measurement {
	pvdb := m.providers["security"].(*Pvdb)
	return pvdb.Dividends
}

// Get splits map
func (m *Manager) GetSplits() map[Security][]*Measurement {
	pvdb := m.providers["security"].(*Pvdb)
	return pvdb.Splits
}

// RegisterDataProvider add a data provider to the system
func (m *Manager) RegisterDataProvider(p Provider) {
	m.providers[p.DataType()] = p
}

// RiskFreeRate Get the risk free rate for given date, if the specific date requested
// is not available then the closest available value is returned
func (m *Manager) RiskFreeRate(ctx context.Context, t time.Time) float64 {
	start := m.lastRiskFreeIdx
	row := riskFreeRate.Row(m.lastRiskFreeIdx, true, dataframe.SeriesName)
	currDate := row[common.DateIdx].(time.Time)
	// check if the requested date is before the last requested date
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

		if vals["PVGG06TNP6J8"] != nil && !math.IsNaN(vals["PVGG06TNP6J8"].(float64)) {
			m.lastRiskFreeIdx = *row
			ret = vals["PVGG06TNP6J8"].(float64)
		}

		dt := vals[common.DateIdx].(time.Time)
		if dt.Equal(t) || dt.After(t) {
			break
		}
	}

	return ret
}

// GetDataFrame get a dataframe for the requested symbol
func (m *Manager) GetDataFrame(ctx context.Context, metric Metric, securities ...*Security) (*dataframe.DataFrame, error) {
	res, err := m.providers["security"].GetDataForPeriod(ctx, securities, metric, m.Frequency, m.Begin, m.End)
	return res, err
}

func (m *Manager) Fetch(ctx context.Context, begin time.Time, end time.Time, metric Metric, securities ...*Security) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "provider.Fetch")
	defer span.End()

	tz := common.GetTimezone()
	begin = time.Date(begin.Year(), begin.Month(), begin.Day(), 0, 0, 0, 0, tz)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, tz)
	res, err := m.providers["security"].GetDataForPeriod(ctx, securities, metric, FrequencyDaily, begin, end)
	if err != nil {
		return err
	}

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "Begin",
			Value: attribute.StringValue(begin.Format("2006-01-02")),
		},
		attribute.KeyValue{
			Key:   "End",
			Value: attribute.StringValue(end.Format("2006-01-02")),
		},
		attribute.KeyValue{
			Key:   "Metric",
			Value: attribute.StringValue(string(metric)),
		},
	)

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
		for _, s := range securities {
			key := buildHashKey(d, metric, s)
			if vals[s.CompositeFigi] != nil {
				m.cache[key] = vals[s].(float64)
			} else {
				span.SetStatus(codes.Error, fmt.Sprintf("no value for %s on %s", s, d.Format("2006-01-02")))
				log.Warn().Stack().Time("Date", d).Str("Metric", string(metric)).Str("Symbol", s.Ticker).Str("Key", key).Msg("setting cache key to NaN")
				m.cache[key] = math.NaN()
			}
		}
	}

	return nil
}

func (m *Manager) Get(ctx context.Context, date time.Time, metric Metric, security *Security) (float64, error) {
	key := buildHashKey(date, metric, security)
	val, ok := m.cache[key]
	if !ok {
		tz := common.GetTimezone()
		end := time.Date(date.Year(), date.Month()+6, date.Day(), 0, 0, 0, 0, tz)
		err := m.Fetch(ctx, date, end, metric, security)
		if err != nil {
			return 0, err
		}
		val, ok = m.cache[key]
		if !ok {
			log.Error().Stack().Str("Metric", string(metric)).Str("Symbol", security.Ticker).Time("Date", date).Msg("could not load metric")
			return 0, ErrMetricDoesNotExist
		}
	}
	return val, nil
}

func (m *Manager) GetLatestDataBefore(ctx context.Context, security *Security, metric Metric, before time.Time) (float64, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "fred.GetLatestDataBefore")
	defer span.End()

	var err error
	val, ok := m.lastCache[security.CompositeFigi]
	if !ok {
		val, err = m.providers["security"].GetLatestDataBefore(ctx, security, metric, before)
		if err != nil {
			log.Warn().Stack().Err(err).Msg("get latest data before failed")
			return math.NaN(), err
		}
		m.lastCache[security.CompositeFigi] = val
	}
	return val, nil
}

func (m *Manager) TradingDays(ctx context.Context, since time.Time, through time.Time, frequency Frequency) ([]time.Time, error) {
	return m.dateProvider.TradingDays(ctx, since, through, frequency)
}

func (m *Manager) HashLen() int {
	return len(m.cache)
}

func (m *Manager) HashSize() int {
	keySize := 19
	valSize := 8
	return (len(m.cache) * keySize) + (len(m.cache) * valSize)
}

func buildHashKey(date time.Time, metric Metric, security *Security) string {
	// Hash key like 2021340:split:VFINX
	return fmt.Sprintf("%d%d:%s:%s", date.Year(), date.YearDay(), metric, security.CompositeFigi)
}
