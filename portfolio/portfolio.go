package portfolio

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"main/dfextras"
	"math"
	"strings"
	"time"

	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

const (
	TickerName = "TICKER"
)

const (
	SellTransaction = "SELL"
	BuyTransaction  = "BUY"
)

type Transaction struct {
	Date          time.Time
	Ticker        string
	Kind          string
	PricePerShare float64
	Shares        float64
	TotalValue    float64
}

// Portfolio manage a portfolio
type Portfolio struct {
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Transactions []Transaction
	dataProxy    *data.Manager
	securities   map[string]bool
}

type PerformanceMeasurement struct {
	Time          int64   `json:"time"`
	Value         float64 `json:"value"`
	Holdings      string  `json:"holdings"`
	PercentReturn float64 `json:"percentReturn"`
}

// Performance of portfolio
type Performance struct {
	PeriodStart        int64                    `json:"periodStart"`
	PeriodEnd          int64                    `json:"periodEnd"`
	Value              []PerformanceMeasurement `json:"value"`
	CagrSinceInception float64                  `json:"cagrSinceInception"`
	YTDReturn          float64                  `json:"ytdReturn"`
	CurrentAsset       string                   `json:"currentAsset"`
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, manager *data.Manager) Portfolio {
	return Portfolio{
		Name:      name,
		dataProxy: manager,
	}
}

// Performance calculate performance of portfolio
func (p *Portfolio) Performance(through time.Time) (Performance, error) {
	if len(p.Transactions) == 0 {
		return Performance{}, errors.New("Cannot calculate performance for portfolio with no transactions")
	}

	perf := Performance{
		PeriodStart: p.StartTime.Unix(),
		PeriodEnd:   through.Unix(),
	}

	// Calculate performance
	symbols := []string{}
	for k := range p.securities {
		symbols = append(symbols, k)
	}

	p.dataProxy.Begin = p.StartTime
	p.dataProxy.End = through
	quotes, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) > 0 {
		return Performance{}, errors.New("Failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, val := range quotes {
		eod = append(eod, val)
	}

	eodQuotes, err := dfextras.Merge(context.TODO(), data.DateIdx, eod...)
	if err != nil {
		return perf, err
	}

	dfextras.DropNA(context.TODO(), eodQuotes, dataframe.FilterOptions{
		InPlace: true,
	})

	iterator := eodQuotes.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	trxIdx := 0
	numTrxs := len(p.Transactions)
	holdings := make(map[string]float64)
	var startVal float64 = 0
	var cagrSinceInception float64 = 0
	valueOverTime := []PerformanceMeasurement{}
	var prevVal float64 = -1
	today := time.Now()
	currYear := today.Year()
	var totalVal float64
	var currYearStartValue float64 = -1.0
	for {
		row, quotes, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}
		date := quotes[data.DateIdx].(time.Time)

		// check if this is the current year
		if date.Year() == currYear && currYearStartValue == -1.0 {
			currYearStartValue = prevVal
		}

		// update holdings?
		for ; trxIdx < numTrxs; trxIdx++ {
			trx := p.Transactions[trxIdx]
			// process transactions up to this point in time
			if date.Equal(trx.Date) || date.After(trx.Date) {
				shares := 0.0
				if val, ok := holdings[trx.Ticker]; ok {
					shares = val
				}
				switch trx.Kind {
				case BuyTransaction:
					shares += trx.Shares
					log.Debugf("on %s buy %.2f shares of %s for %.2f @ %.2f per share\n", trx.Date, trx.Shares, trx.Ticker, trx.TotalValue, trx.PricePerShare)
				case SellTransaction:
					shares -= trx.Shares
					log.Debugf("on %s sell %.2f shares of %s for %.2f @ %.2f per share\n", trx.Date, trx.Shares, trx.Ticker, trx.TotalValue, trx.PricePerShare)
				default:
					return Performance{}, errors.New("unrecognized transaction type")
				}
				holdings[trx.Ticker] = shares
			} else {
				break
			}
		}

		// iterate through each holding and add value to get total return
		totalVal = 0.0
		var tickers string
		for symbol, qty := range holdings {
			if val, ok := quotes[symbol]; ok {
				price := val.(float64)
				totalVal += price * qty
				if qty > 0 {
					tickers += symbol + " "
				}
			} else {
				return Performance{}, fmt.Errorf("no quote for symbol: %s", symbol)
			}
		}

		if prevVal == -1 {
			prevVal = totalVal
			startVal = totalVal
		}

		tickers = strings.Trim(tickers, " ")
		ret := totalVal/prevVal - 1
		duration := date.Sub(p.StartTime).Hours() / (24 * 365.25)
		cagrSinceInception = math.Pow(totalVal/startVal, 1.0/duration) - 1
		prevVal = totalVal

		valueOverTime = append(valueOverTime, PerformanceMeasurement{
			Time:          date.Unix(),
			Value:         totalVal,
			Holdings:      tickers,
			PercentReturn: ret,
		})

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAsset = tickers
		}
	}

	perf.Value = valueOverTime
	perf.CagrSinceInception = cagrSinceInception
	perf.YTDReturn = totalVal/currYearStartValue - 1.0

	return perf, nil
}

// TargetPortfolio invest target portfolio
func (p *Portfolio) TargetPortfolio(initial float64, target *dataframe.DataFrame) error {
	p.Transactions = []Transaction{}
	timeIdx, err := target.NameToColumn(data.DateIdx)
	if err != nil {
		return err
	}

	timeSeries := target.Series[timeIdx]

	// Set time range of portfolio
	p.StartTime = timeSeries.Value(0).(time.Time)
	p.EndTime = timeSeries.Value(timeSeries.NRows() - 1).(time.Time)

	// Get price data
	p.securities = make(map[string]bool)
	iterator := target.Series[1].ValuesIterator()
	for {
		row, val, _ := iterator()
		if row == nil {
			break
		}

		p.securities[val.(string)] = true
	}

	symbols := []string{}
	for k := range p.securities {
		symbols = append(symbols, k)
	}

	prices, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) != 0 {
		return errors.New("Failed loading data for tickers")
	}

	// Create transactions
	targetIter := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	value := initial
	var lastTransaction *Transaction
	var lastSymbol string
	for {
		row, val, _ := targetIter(dataframe.SeriesName)
		if row == nil {
			break
		}

		// Get next transaction symbol
		date := val[data.DateIdx].(time.Time)
		symbol := val[TickerName].(string)

		if lastSymbol != symbol {
			// Sell previous transaction
			if lastTransaction != nil {
				eod := prices[lastTransaction.Ticker]
				res, err := dfextras.FindTime(context.TODO(), eod, date, data.DateIdx)
				if err != nil {
					return err
				}
				price := res[lastTransaction.Ticker].(float64)
				value = lastTransaction.Shares * price
				t := Transaction{
					Date:          date,
					Ticker:        lastTransaction.Ticker,
					Kind:          SellTransaction,
					PricePerShare: price,
					Shares:        lastTransaction.Shares,
					TotalValue:    value,
				}
				p.Transactions = append(p.Transactions, t)
			}

			// Buy new stock if it doesn't match the previous one
			eod := prices[symbol]
			res, err := dfextras.FindTime(context.TODO(), eod, date, data.DateIdx)
			if err != nil {
				return err
			}
			price := res[symbol].(float64)
			shares := value / price

			t := Transaction{
				Date:          date,
				Ticker:        symbol,
				Kind:          BuyTransaction,
				PricePerShare: price,
				Shares:        shares,
				TotalValue:    value,
			}

			lastTransaction = &t
			lastSymbol = symbol

			p.Transactions = append(p.Transactions, t)
		}
	}

	return nil
}
