package data

import (
	"errors"
	"log"
	"strings"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
)

// Provider interface for retrieving quotes
type Provider interface {
	DataType() string
	GetDataForPeriod(symbol string, metric string, frequency string, begin time.Time, end time.Time) (*dataframe.DataFrame, error)
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
	Begin       time.Time
	End         time.Time
	Frequency   string
	Metric      string
	credentials map[string]string
	providers   map[string]Provider
}

// NewManager create a new data manager
func NewManager(credentials map[string]string) Manager {
	var m = Manager{
		credentials: credentials,
		providers:   map[string]Provider{},
		Metric:      MetricAdjustedClose,
	}

	// Create Tiingo API
	if val, ok := credentials["tiingo"]; ok {
		tiingo := NewTiingo(val)
		m.RegisterDataProvider(tiingo)
	} else {
		log.Println("No tiingo API key provided")
	}

	// Create FRED API
	fred := NewFred()
	m.RegisterDataProvider(fred)

	return m
}

// RegisterDataProvider add a data provider to the system
func (m Manager) RegisterDataProvider(p Provider) {
	m.providers[p.DataType()] = p
}

// GetData get a dataframe for the requested symbol
func (m Manager) GetData(symbol string) (*dataframe.DataFrame, error) {
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
func (m Manager) GetMultipleData(symbols ...string) (map[string]*dataframe.DataFrame, []error) {
	res := make(map[string]*dataframe.DataFrame)
	ch := make(chan quoteResult)
	for ii := range symbols {
		go downloadWorker(ch, symbols[ii], &m)
	}

	errs := []error{}
	for range symbols {
		v := <-ch
		if v.Err == nil {
			res[v.Ticker] = v.Data
		} else {
			log.Println(v.Err)
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
