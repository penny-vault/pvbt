package data

import (
	"errors"
	"math"
	"strings"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

// Provider interface for retrieving quotes
type Provider interface {
	DataType() string
	GetDataForPeriod(symbol string, metric string, frequency string, begin time.Time, end time.Time) (*dataframe.DataFrame, error)
}

type DateProvider interface {
	LastTradingDayOfWeek(t time.Time) (time.Time, error)
	LastTradingDayOfMonth(t time.Time) (time.Time, error)
	LastTradingDayOfYear(t time.Time) (time.Time, error)
}

const (
	FrequencyDaily   = "Daily"
	FrequencyWeekly  = "Weekly"
	FrequencyMonthly = "Monthly"
	FrequencyAnnualy = "Annualy"
)

const (
	DateIdx = "DATE"
)

const (
	MetricOpen          = "Open"
	MetricLow           = "Low"
	MetricHigh          = "High"
	MetricClose         = "Close"
	MetricVolume        = "Volume"
	MetricAdjustedOpen  = "AdjustedOpen"
	MetricAdjustedLow   = "AdjustedLow"
	MetricAdjustedHigh  = "AdjustedHigh"
	MetricAdjustedClose = "AdjustedClose"
)

// Manager data manager type
type Manager struct {
	Begin           time.Time
	End             time.Time
	Frequency       string
	Metric          string
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
	riskFreeRate, err = fred.GetDataForPeriod("DTB3", FrequencyDaily, MetricClose,
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
		credentials: credentials,
		providers:   map[string]Provider{},
		Metric:      MetricAdjustedClose,
	}

	// Create Tiingo API
	if val, ok := credentials["tiingo"]; ok {
		tiingo := NewTiingo(val)
		m.RegisterDataProvider(tiingo)
		m.dateProvider = tiingo
	} else {
		log.Warn("No tiingo API key provided")
	}

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
	currDate := row[DateIdx].(time.Time)
	// check if the requestsed date is before the last requested date
	if t.Before(currDate) {
		start = 0
	}

	var ret float64
	iterator := riskFreeRate.ValuesIterator(dataframe.ValuesOptions{start, 1, true})
	for {
		row, vals, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		if !math.IsNaN(vals["DTB3"].(float64)) {
			m.lastRiskFreeIdx = *row
			ret = vals["DTB3"].(float64)
		}

		dt := vals[DateIdx].(time.Time)
		if dt.Equal(t) || dt.After(t) {
			break
		}
	}

	log.Debugf("lad idx: %d", m.lastRiskFreeIdx)
	return ret
}

// LastTradingDayOfWeek Get the last trading day of the specified month
func (m *Manager) LastTradingDayOfWeek(t time.Time) (time.Time, error) {
	return m.dateProvider.LastTradingDayOfWeek(t)
}

// LastTradingDayOfMonth Get the last trading day of the specified month
func (m *Manager) LastTradingDayOfMonth(t time.Time) (time.Time, error) {
	return m.dateProvider.LastTradingDayOfMonth(t)
}

// LastTradingDayOfYear Get the last trading day of the specified year
func (m *Manager) LastTradingDayOfYear(t time.Time) (time.Time, error) {
	return m.dateProvider.LastTradingDayOfYear(t)
}

// GetData get a dataframe for the requested symbol
func (m *Manager) GetData(symbol string) (*dataframe.DataFrame, error) {
	kind := "security"

	symbol = strings.ToUpper(symbol)

	if strings.HasPrefix(symbol, "$RATE.") {
		kind = "rate"
		symbol = strings.TrimPrefix(symbol, "$RATE.")
	}

	if provider, ok := m.providers[kind]; ok {
		return provider.GetDataForPeriod(symbol, m.Metric, m.Frequency, m.Begin, m.End)
	}

	return nil, errors.New("Specified kind '" + kind + "' is not supported")
}

// GetMultipleData get multiple quotes simultaneously
func (m *Manager) GetMultipleData(symbols ...string) (map[string]*dataframe.DataFrame, []error) {
	res := make(map[string]*dataframe.DataFrame)
	ch := make(chan quoteResult)
	for ii := range symbols {
		go downloadWorker(ch, strings.ToUpper(symbols[ii]), m)
	}

	errs := []error{}
	for range symbols {
		v := <-ch
		if v.Err == nil {
			res[v.Ticker] = v.Data
		} else {
			log.WithFields(log.Fields{
				"Ticker": v.Ticker,
				"Error":  v.Err,
			}).Warn("Cannot download ticker data")
			errs = append(errs, v.Err)
		}
	}

	return res, errs
}

type quoteResult struct {
	Ticker string
	Data   *dataframe.DataFrame
	Err    error
}

func downloadWorker(result chan<- quoteResult, symbol string, manager *Manager) {
	df, err := manager.GetData(symbol)
	res := quoteResult{
		Ticker: symbol,
		Data:   df,
		Err:    err,
	}
	result <- res
}
