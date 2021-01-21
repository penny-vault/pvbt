package data

import (
	"errors"
	"log"
	"strings"
	"time"

	dataframe "github.com/rocketlaunchr/dataframe-go"
)

// Asset A struct storing information about a financial asset
type Asset struct {
	Ticker    string
	Exchange  string
	Kind      string
	Currency  string
	StartDate time.Time
}

// Provider interface for retrieving quotes
type Provider interface {
	DataType() string
	GetDataForPeriod(symbol string, frequency string, begin time.Time, end time.Time) (*dataframe.DataFrame, error)
}

// Manager data manager type
type Manager struct {
	Begin       time.Time
	End         time.Time
	Frequency   string
	credentials map[string]string
	providers   map[string]Provider
}

// NewManager create a new data manager
func NewManager(credentials map[string]string) Manager {
	var m = Manager{
		credentials: credentials,
		providers:   map[string]Provider{},
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

	if strings.HasPrefix(symbol, "$rates.") {
		kind = "rate"
		symbol = strings.TrimPrefix(symbol, "$rates.")
	}

	if provider, ok := m.providers[kind]; ok {
		return provider.GetDataForPeriod(symbol, m.Frequency, m.Begin, m.End)
	}

	return nil, errors.New("Specified kind '" + kind + "' is not recognized")
}
