package data

import (
	"time"

	"github.com/tobgu/qframe"
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
	HasKey(symbol string) bool
	GetDataForPeriod(symbols []string, frequency string, begin time.Time, end time.Time) qframe.QFrame
}
