package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"main/data"
	"main/database"
	"main/dfextras"
	"main/strategies"
	"main/util"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

const (
	SourceName = "PV"
)

const (
	SellTransaction     = "SELL"
	BuyTransaction      = "BUY"
	DepositTransaction  = "DEPOSIT"
	WithdrawTransaction = "WITHDRAW"
	MarkerTransaction   = "MARKER"
)

type Transaction struct {
	Date          time.Time              `json:"date"`
	Ticker        string                 `json:"ticker"`
	CompositeFIGI string                 `json:"compositeFIGI"`
	Kind          string                 `json:"kind"`
	PricePerShare float64                `json:"pricePerShare"`
	Shares        float64                `json:"shares"`
	Source        string                 `json:"source"`
	SourceID      []byte                 `json:"sourceID"`
	TotalValue    float64                `json:"totalValue"`
	Justification map[string]interface{} `json:"justification"`
}

type Holding struct {
	Date   time.Time
	Ticker string
	Shares float64
}

type ReportableHolding struct {
	Ticker           string  `json:"ticker"`
	Shares           float64 `json:"shares"`
	PercentPortfolio float64 `json:"percentPortfolio"`
	Value            float64 `json:"value"`
}

// Portfolio manage a portfolio
type Portfolio struct {
	ID                uuid.UUID
	UserID            string
	Name              string
	StartDate         time.Time
	EndDate           time.Time
	StrategyShortcode string
	StrategyArguments map[string]json.RawMessage
	Notifications     int
	Transactions      []Transaction
	Measurements      []PerformanceMeasurement
	Holdings          map[string]float64

	// private
	dataProxy  *data.Manager
	securities map[string]bool
	priceData  map[string]*dataframe.DataFrame
}

type PerformanceMeasurement struct {
	Time           int64                  `json:"time"`
	Value          float64                `json:"value"`
	RiskFreeValue  float64                `json:"riskFreeValue"`
	Holdings       []ReportableHolding    `json:"holdings"`
	PercentReturn  float64                `json:"percentReturn"`
	TotalDeposited float64                `json:"totalDeposited"`
	TotalWithdrawn float64                `json:"totalWithdrawn"`
	Justification  map[string]interface{} `json:"justification"`
}

// Performance of portfolio
type Performance struct {
	PeriodStart        int64                    `json:"periodStart"`
	PeriodEnd          int64                    `json:"periodEnd"`
	ComputedOn         int64                    `json:"computedOn"`
	Measurements       []PerformanceMeasurement `json:"measurements"`
	Transactions       []Transaction            `json:"transactions"`
	CagrSinceInception float64                  `json:"cagrSinceInception"`
	YTDReturn          float64                  `json:"ytdReturn"`
	CurrentAsset       string                   `json:"currentAsset"` // deprecated
	CurrentAssets      []ReportableHolding      `json:"currentAssets"`
	TotalDeposited     float64                  `json:"totalDeposited"`
	TotalWithdrawn     float64                  `json:"totalWithdrawn"`
	MetricsBundle      MetricsBundle            `json:"metrics"`
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, startDate time.Time, initial float64, manager *data.Manager) *Portfolio {
	p := Portfolio{
		Name:         name,
		Transactions: []Transaction{},
		dataProxy:    manager,
		StartDate:    startDate,
		securities:   make(map[string]bool),
	}

	// Create initial deposit
	t := Transaction{
		Date:          startDate,
		Ticker:        "$CASH",
		Kind:          DepositTransaction,
		PricePerShare: 1.0,
		Shares:        initial,
		TotalValue:    initial,
		Justification: nil,
	}

	err := computeTransactionSourceID(&t)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":             err,
			"TransactionDate":   startDate,
			"TransactionTicker": "$CASH",
			"TransactionType":   DepositTransaction,
		}).Warn("couldn't compute SourceID for initial deposit")
	}
	p.Transactions = append(p.Transactions, t)
	p.Holdings = map[string]float64{
		"$CASH": initial,
	}

	return &p
}

// ValueAsOf return the value of the portfolio for the given date
func (p *Portfolio) ValueAsOf(d time.Time) (float64, error) {
	// Get last 7 days of values, in case 'd' isn't a market day
	s := d.AddDate(0, 0, -7)
	value, err := p.valueOverPeriod(s, d)
	sz := len(value)
	if sz <= 0 {
		return 0, errors.New("failed to compute value for date")
	}
	return value[sz-1].Value, err
}

func (p *Portfolio) valueOverPeriod(s time.Time, e time.Time) ([]*PerformanceMeasurement, error) {
	if len(p.Transactions) == 0 {
		return nil, errors.New("cannot calculate performance for portfolio with no transactions")
	}

	p.dataProxy.Begin = s
	p.dataProxy.End = e
	p.dataProxy.Frequency = data.FrequencyDaily

	// Get holdings over period
	holdings, err := p.holdingsOverPeriod(s, e)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	// Get a list of all tickers
	tickerSet := map[string]bool{}
	for _, v := range holdings {
		for _, h := range v {
			tickerSet[h.Ticker] = true
		}
	}

	symbols := []string{}
	for k := range tickerSet {
		symbols = append(symbols, k)
	}

	// get quote data
	quotes, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) > 0 {
		return nil, errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, val := range quotes {
		eod = append(eod, val)
	}

	eodQuotes, err := dfextras.Merge(context.TODO(), data.DateIdx, eod...)
	if err != nil {
		return nil, err
	}

	// compute value over period
	values := []*PerformanceMeasurement{}
	currHoldings := holdings[s]
	iterator := eodQuotes.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})

	tz, _ := time.LoadLocation("America/New_York") // New York is the reference time

	for {
		row, quotes, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}
		date := quotes[data.DateIdx].(time.Time)
		year, month, day := date.Date()
		date = time.Date(year, month, day, 0, 0, 0, 0, tz)

		if v, ok := holdings[date]; ok {
			currHoldings = v
		}

		// iterate through each holding and add value to get total return
		totalVal := 0.0
		for _, holding := range currHoldings {
			if holding.Ticker == "$CASH" {
				totalVal += holding.Shares
			} else if val, ok := quotes[holding.Ticker]; ok {
				price := val.(float64)
				totalVal += price * holding.Shares
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", holding.Ticker)
			}
		}
		values = append(values, &PerformanceMeasurement{
			Time:  date.Unix(),
			Value: totalVal,
		})
	}

	return values, nil
}

func (p *Portfolio) holdingsOverPeriod(s time.Time, e time.Time) (map[time.Time][]Holding, error) {
	currHoldings := map[string]Holding{}
	periodHoldings := map[time.Time][]Holding{}

	for _, t := range p.Transactions {
		if t.Kind == DepositTransaction || t.Kind == WithdrawTransaction || t.Kind == MarkerTransaction {
			continue
		}

		if t.Date.After(e) && len(periodHoldings) == 0 {
			holdings := make([]Holding, 0, len(currHoldings))
			for _, v := range currHoldings {
				holdings = append(holdings, v)
			}
			periodHoldings[s] = holdings
			return periodHoldings, nil
		}

		if t.Date.After(e) {
			return periodHoldings, nil
		}

		if h, ok := currHoldings[t.Ticker]; ok {
			switch t.Kind {
			case BuyTransaction:
				h.Shares += t.Shares
			case SellTransaction:
				h.Shares -= t.Shares
			}
			if h.Shares <= 1e-5 {
				delete(currHoldings, h.Ticker)
			} else {
				currHoldings[t.Ticker] = h
			}
		} else {
			if t.Kind != BuyTransaction {
				log.Error("Transactions are out of order")
				return nil, errors.New("transactions are out of order")
			}
			currHoldings[t.Ticker] = Holding{
				Ticker: t.Ticker,
				Shares: t.Shares,
			}
		}

		if (t.Date.After(s) || t.Date.Equal(s)) && (t.Date.Before(e) || t.Date.Equal(e)) {
			holdings := make([]Holding, 0, len(currHoldings))
			for _, v := range currHoldings {
				holdings = append(holdings, v)
			}
			periodHoldings[t.Date] = holdings
		}
	}

	holdings := make([]Holding, 0, len(currHoldings))
	for _, v := range currHoldings {
		holdings = append(holdings, v)
	}
	periodHoldings[s] = holdings

	return periodHoldings, nil
}

// CalculatePerformance calculate performance of portfolio
func (p *Portfolio) CalculatePerformance(through time.Time) (*Performance, error) {
	if len(p.Transactions) == 0 {
		return nil, errors.New("cannot calculate performance for portfolio with no transactions")
	}

	perf := Performance{
		PeriodStart:  p.StartDate.Unix(),
		PeriodEnd:    through.Unix(),
		ComputedOn:   time.Now().Unix(),
		Transactions: p.Transactions,
	}

	// Calculate performance
	symbols := []string{}
	for k := range p.securities {
		symbols = append(symbols, k)
	}

	p.dataProxy.Begin = p.StartDate
	p.dataProxy.End = through
	p.dataProxy.Frequency = data.FrequencyMonthly

	quotes, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) > 0 {
		return nil, errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, val := range quotes {
		eod = append(eod, val)
	}

	eodQuotes, err := dfextras.Merge(context.TODO(), data.DateIdx, eod...)
	if err != nil {
		return nil, err
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
	var riskFreeValue float64 = 0

	var lastJustification map[string]interface{}

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
			// test if date is Before the trx.Date - if it is then break
			if date.Before(trx.Date) {
				break
			}

			lastJustification = trx.Justification

			if trx.Kind == MarkerTransaction {
				continue
			}

			if trx.Kind == DepositTransaction || trx.Kind == WithdrawTransaction {
				switch trx.Kind {
				case DepositTransaction:
					perf.TotalDeposited += trx.TotalValue
					riskFreeValue += trx.TotalValue
				case WithdrawTransaction:
					perf.TotalWithdrawn += trx.TotalValue
					riskFreeValue -= trx.TotalValue
				}
				continue
			}

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
				return nil, errors.New("unrecognized transaction type")
			}

			// Protect against floating point noise
			if shares <= 1.0e-5 {
				shares = 0
			}

			holdings[trx.Ticker] = shares
		}

		// iterate through each holding and add value to get total return
		totalVal = 0.0
		tickers := make([]string, 0, len(holdings))
		for symbol, qty := range holdings {
			if symbol == "$CASH" {
				totalVal += qty
			} else if val, ok := quotes[symbol]; ok {
				price := val.(float64)
				totalVal += price * qty
				if qty > 1.0e-5 {
					tickers = append(tickers, symbol)
				}
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", symbol)
			}
		}

		// this is done as a second loop because we need totalVal to be set for
		// percent calculation
		currentAssets := make([]ReportableHolding, 0, len(holdings))
		for symbol, qty := range holdings {
			var value float64
			if symbol == "$CASH" {
				value = qty
			} else if val, ok := quotes[symbol]; ok {
				price := val.(float64)
				if qty > 1.0e-5 {
					value = price * qty
				}
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", symbol)
			}
			if qty > 1.0e-5 {
				currentAssets = append(currentAssets, ReportableHolding{
					Ticker:           symbol,
					Shares:           qty,
					PercentPortfolio: value / totalVal,
					Value:            value,
				})
			}
		}

		if prevVal == -1 {
			prevVal = totalVal
			startVal = totalVal
		} else {
			// update riskFreeValue
			rawRate := p.dataProxy.RiskFreeRate(date)
			riskFreeRate := rawRate / 100.0 / 12.0
			riskFreeValue *= (1 + riskFreeRate)
		}

		sort.Strings(tickers)
		holdingStr := strings.Join(tickers, " ")
		ret := totalVal/prevVal - 1
		duration := date.Sub(p.StartDate).Hours() / (24 * 365.25)
		cagrSinceInception = math.Pow(totalVal/startVal, 1.0/duration) - 1
		prevVal = totalVal

		valueOverTime = append(valueOverTime, PerformanceMeasurement{
			Time:          date.Unix(),
			Value:         totalVal,
			RiskFreeValue: riskFreeValue,
			Holdings:      currentAssets,
			PercentReturn: ret,
			Justification: lastJustification,
		})

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAsset = holdingStr
			perf.CurrentAssets = currentAssets
		}
	}

	perf.Measurements = valueOverTime
	perf.CagrSinceInception = cagrSinceInception

	if currYearStartValue <= 0 {
		perf.YTDReturn = 0.0
	} else {
		perf.YTDReturn = totalVal/currYearStartValue - 1.0
	}

	return &perf, nil
}

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (p *Portfolio) RebalanceTo(date time.Time, target map[string]float64, justification map[string]interface{}) error {
	nTrx := len(p.Transactions)
	if nTrx > 0 {
		lastDate := p.Transactions[nTrx-1].Date
		if lastDate.After(date) {
			return fmt.Errorf("cannot rebalance portfolio on date %s when last existing transaction date is %s", date.String(), lastDate.String())
		}
	}

	// check that target sums to 1.0
	var total float64
	for _, v := range target {
		total += v
	}

	// Allow for floating point error
	diff := math.Abs(1.0 - total)
	if diff > 1.0e-11 {
		return fmt.Errorf("rebalance percent total does not equal 1.0, it is %.2f for date %s", total, date)
	}

	// get the cash position of the portfolio
	var cash float64
	if currCash, ok := p.Holdings["$CASH"]; ok {
		cash += currCash
	}

	// get the current value of non-cash holdings
	var securityValue float64
	priceMap := map[string]float64{
		"$CASH": 1.0,
	}
	for k, v := range p.Holdings {
		if k != "$CASH" {
			eod := p.priceData[k]
			res := dfextras.FindNearestTime(eod, date, data.DateIdx, time.Hour*24)

			var price float64
			if tmp, ok := res[k]; ok {
				price = tmp.(float64)
			} else {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
				}).Debug("Security purchased before security price was available")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}

			securityValue += v * price
			priceMap[k] = price
		}
	}

	// get any prices that we haven't already loaded
	for k := range target {
		if _, ok := priceMap[k]; !ok {
			eod := p.priceData[k]
			res := dfextras.FindNearestTime(eod, date, data.DateIdx, time.Hour*24)

			var price float64
			if tmp, ok := res[k]; ok {
				price = tmp.(float64)
			} else {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
				}).Debug("Security purchased before security price was available")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}

			priceMap[k] = price
		}
	}

	investable := cash + securityValue

	// process all targets
	sells := []Transaction{}
	buys := []Transaction{}

	// sell any holdings that we no longer want
	for k, v := range p.Holdings {
		if k == "$CASH" {
			continue
		}

		if v <= 1.0e-5 {
			log.WithFields(log.Fields{
				"Ticker":   k,
				"Kind":     "SellTransaction",
				"Shares":   v,
				"Holdings": p.Holdings,
			}).Warn("holdings are out of sync")
			return errors.New("holdings are out of sync, cannot rebalance portfolio")
		}

		if _, ok := target[k]; !ok {
			t := Transaction{
				Date:          date,
				Ticker:        k,
				Kind:          SellTransaction,
				PricePerShare: priceMap[k],
				Shares:        v,
				TotalValue:    v * priceMap[k],
				Justification: justification,
				Source:        SourceName,
			}

			err := computeTransactionSourceID(&t)
			if err != nil {
				log.WithFields(log.Fields{
					"Error":             err,
					"TransactionDate":   date,
					"TransactionTicker": k,
					"TransactionType":   SellTransaction,
				}).Warn("couldn't compute SourceID for transaction")
			}

			sells = append(sells, t)
		}
	}

	newHoldings := make(map[string]float64)
	for k, v := range target {
		// is this security currently held and should we sell it?
		if holding, ok := p.Holdings[k]; ok {
			targetDollars := investable * v
			currentDollars := holding * priceMap[k]
			if (targetDollars / priceMap[k]) > 1.0e-5 {
				newHoldings[k] = targetDollars / priceMap[k]
			}
			if targetDollars < currentDollars {
				// Need to sell to target amount
				toSellDollars := currentDollars - targetDollars
				toSellShares := toSellDollars / priceMap[k]
				if toSellDollars <= 1.0e-5 {
					log.WithFields(log.Fields{
						"Ticker": k,
						"Kind":   "SellTransaction",
						"Shares": toSellShares,
						"Date":   date,
					}).Warn("holdings are out of sync - refusing to sell 0 shares")
					return errors.New("holdings are out of sync, cannot rebalance portfolio")
				}

				t := Transaction{
					Date:          date,
					Ticker:        k,
					Kind:          SellTransaction,
					PricePerShare: priceMap[k],
					Shares:        toSellShares,
					TotalValue:    toSellDollars,
					Justification: justification,
				}

				err := computeTransactionSourceID(&t)
				if err != nil {
					log.WithFields(log.Fields{
						"Error":             err,
						"TransactionDate":   date,
						"TransactionTicker": k,
						"TransactionType":   SellTransaction,
					}).Warn("couldn't compute SourceID for transaction")
				}

				sells = append(sells, t)
			}
			if targetDollars > currentDollars {
				// Need to buy to target amount
				toBuyDollars := targetDollars - currentDollars
				toBuyShares := toBuyDollars / priceMap[k]

				if toBuyShares <= 1.0e-5 {
					log.WithFields(log.Fields{
						"Ticker":     k,
						"Kind":       "BuyTransaction",
						"Shares":     v,
						"TotalValue": toBuyDollars,
						"Date":       date,
					}).Warn("Refusing to buy 0 shares")
				} else {

					t := Transaction{
						Date:          date,
						Ticker:        k,
						Kind:          BuyTransaction,
						PricePerShare: priceMap[k],
						Shares:        toBuyShares,
						TotalValue:    toBuyDollars,
						Justification: justification,
					}

					err := computeTransactionSourceID(&t)
					if err != nil {
						log.WithFields(log.Fields{
							"Error":             err,
							"TransactionDate":   date,
							"TransactionTicker": k,
							"TransactionType":   BuyTransaction,
						}).Warn("couldn't compute SourceID for transaction")
					}

					buys = append(buys, t)
				}
			}
		} else {
			// this is a new position
			value := investable * v
			shares := value / priceMap[k]

			if shares <= 1.0e-5 {
				log.WithFields(log.Fields{
					"Ticker":     k,
					"Kind":       "BuyTransaction",
					"Shares":     v,
					"TotalValue": value,
					"Date":       date,
				}).Warn("Refusing to buy 0 shares")
			} else {
				newHoldings[k] = shares

				t := Transaction{
					Date:          date,
					Ticker:        k,
					Kind:          BuyTransaction,
					PricePerShare: priceMap[k],
					Shares:        shares,
					TotalValue:    value,
					Justification: justification,
				}

				err := computeTransactionSourceID(&t)
				if err != nil {
					log.WithFields(log.Fields{
						"Error":             err,
						"TransactionDate":   date,
						"TransactionTicker": k,
						"TransactionType":   BuyTransaction,
					}).Warn("couldn't compute SourceID for transaction")
				}

				buys = append(buys, t)
			}
		}
	}
	p.Transactions = append(p.Transactions, sells...)
	p.Transactions = append(p.Transactions, buys...)
	p.Holdings = newHoldings

	return nil
}

// TargetPortfolioFromDataFrame invest's the portfolio in the ratios specified by the dataframe `target`
//   at each date period in the dataframe. The dataframe must have a date column and a float64
//   column for each ticker to invest in. Column values are the target percentage during that
//   time period. All columns in a row (excluding the date column) must sum to 1.0
//
//   E.g.,
//
//   | date       | VFINX | PRIDX |
//   |------------|-------|-------|
//   | 2021-01-01 | .5    | .5    |
//   | 2021-02-01 | .25   | .75   |
//   | 2021-03-01 | 0     | 1.0   |
//
// TODO

// TargetPortfolio invests the portfolio in the ratios specified by the dataframe `target`.
//   `target` must have a column named `data.DateIdx` (DATE) and either a string column
//   or MixedAsset column of map[string]float64 where the keys are the tickers and values are
//   the percentages of portfolio to hold
func (p *Portfolio) TargetPortfolio(target *dataframe.DataFrame) error {
	timeIdx, err := target.NameToColumn(data.DateIdx)
	if err != nil {
		return err
	}

	timeSeries := target.Series[timeIdx]

	// Set time range of portfolio
	p.EndDate = timeSeries.Value(timeSeries.NRows() - 1).(time.Time)

	// Adjust first transaction to the target portfolio's first date if
	// there are no other transactions in the portfolio
	if len(p.Transactions) == 1 {
		p.StartDate = timeSeries.Value(0).(time.Time)
		p.Transactions[0].Date = p.StartDate
	}

	tickerSeriesIdx, err := target.NameToColumn(util.TickerName)
	if err != nil {
		return fmt.Errorf("missing required column: %s", util.TickerName)
	}

	// check series type
	isSingleAsset := false
	series := target.Series[tickerSeriesIdx]
	if series.Type() == "string" {
		isSingleAsset = true
	}

	// Get price data
	iterator := target.Series[tickerSeriesIdx].ValuesIterator()
	for {
		row, val, _ := iterator()
		if row == nil {
			break
		}

		if isSingleAsset {
			p.securities[val.(string)] = true
		} else {
			// it's multi-asset which means a map of tickers
			for ticker := range val.(map[string]float64) {
				p.securities[ticker] = true
			}
		}
	}

	symbols := []string{}
	for k := range p.securities {
		symbols = append(symbols, k)
	}

	prices, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) != 0 {
		errorMsgs := make([]string, len(errs))
		for ii, xx := range errs {
			errorMsgs[ii] = xx.Error()
		}
		log.WithFields(log.Fields{
			"Error": strings.Join(errorMsgs, ", "),
		}).Warn("Failed to load data for tickers")
		return errors.New("failed loading data for tickers")
	}
	p.priceData = prices

	// Create transactions
	targetIter := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	for {
		row, val, _ := targetIter(dataframe.SeriesName)
		if row == nil {
			break
		}

		// Get next transaction symbol
		var date time.Time
		var symbol interface{}
		justification := make(map[string]interface{})

		for k, v := range val {
			idx := k.(string)
			if idx == data.DateIdx {
				date = val[data.DateIdx].(time.Time)
			} else if idx == util.TickerName {
				symbol = val[util.TickerName]
			} else {
				justification[idx] = v
			}
		}

		// HACK - if the date is Midnight adjust to market close (i.e. 4pm EST)
		// This should really be set correctly for the day. The problem is if the
		// transaction is on a day where the market closes early (either because of
		// a holiday or because a "circuit-breaker" threshold was reached) we do
		// not have a reliable datasource that tells us the time the market closed.
		//
		// Generally speaking getting the time slightly off here is immaterial so this
		// is an OK hack
		if date.Hour() == 0 && date.Minute() == 0 && date.Second() == 0 {
			date = date.Add(time.Hour * 16)
		}

		var rebalance map[string]float64
		if isSingleAsset {
			strSymbol := symbol.(string)
			rebalance = map[string]float64{}
			rebalance[strSymbol] = 1.0
		} else {
			rebalance = symbol.(map[string]float64)
		}

		t := Transaction{
			Date:          date,
			Kind:          MarkerTransaction,
			Justification: justification,
		}
		err := computeTransactionSourceID(&t)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":           err,
				"TransactionType": MarkerTransaction,
			}).Warn("couldn't compute SourceID for transaction")
		}

		p.Transactions = append(p.Transactions, t)
		err = p.RebalanceTo(date, rebalance, justification)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateTransactions calculates new transactions based on the portfolio strategy
// from the portfolio end date to `through`
func (p *Portfolio) UpdateTransactions(manager *data.Manager, through time.Time) error {
	manager.Begin = p.EndDate
	manager.End = through
	manager.Frequency = data.FrequencyMonthly

	if strategy, ok := strategies.StrategyMap[p.StrategyShortcode]; ok {
		stratObject, err := strategy.Factory(p.StrategyArguments)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":     err,
				"Portfolio": p.ID,
				"Strategy":  p.StrategyShortcode,
			}).Error("failed to initialize portfolio strategy")
			return err
		}

		targetPortfolio, err := stratObject.Compute(manager)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":     err,
				"Portfolio": p.ID,
				"Strategy":  p.StrategyShortcode,
			}).Error("failed to run portfolio strategy")
			return err
		}

		err = p.TargetPortfolio(targetPortfolio)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":     err,
				"Portfolio": p.ID,
				"Strategy":  p.StrategyShortcode,
			}).Error("failed to apply target portfolio")
			return err
		}
	} else {
		log.WithFields(log.Fields{
			"Portfolio": p.ID,
			"Strategy":  p.StrategyShortcode,
		}).Error("portfolio strategy not found")
		return errors.New("strategy not found")
	}

	return nil
}

func (p *Portfolio) loadMeasurementsFromDB() error {
	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.ID,
			"UserID":      p.UserID,
		}).Error("unable to get database transaction object")
		return nil
	}

	measurementSQL := "SELECT extract(epoch from event_date), value, risk_free_value, holdings, percent_return, justification FROM portfolio_measurement_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY event_date"
	rows, err := trx.Query(context.Background(), measurementSQL, p.ID, p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.ID,
			"UserID":      p.UserID,
			"Query":       measurementSQL,
		}).Warn("failed executing measurement query")
		trx.Rollback(context.Background())
		return err
	}

	measurements := make([]PerformanceMeasurement, 0, 1000)
	for rows.Next() {
		m := PerformanceMeasurement{}
		err := rows.Scan(&m.Time, &m.Value, &m.RiskFreeValue, &m.Holdings, &m.PercentReturn, &m.Justification)
		if err != nil {
			log.WithFields(log.Fields{
				"Endpoint":    "GetPortfolioPerformance:Measurement-Iterator",
				"Error":       err,
				"PortfolioID": p.ID,
				"UserID":      p.UserID,
				"Query":       measurementSQL,
			}).Warn("GetPortfolioPerformance failed")
			trx.Rollback(context.Background())
			return err
		}
		measurements = append(measurements, m)
	}
	p.Measurements = measurements
	return nil
}

func (p *Portfolio) loadTransactionsFromDB() error {
	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.ID,
			"UserID":      p.UserID,
		}).Error("unable to get database transaction object")
		return nil
	}

	// Get portfolio transactions
	transactionSQL := "SELECT event_date, ticker, transaction_type, price_per_share, num_shares, total_value, justification FROM portfolio_transaction_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY sequence_num"
	rows, err := trx.Query(context.Background(), transactionSQL, p.ID, p.UserID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.ID,
			"UserID":      p.UserID,
			"Query":       transactionSQL,
		}).Warn("could not load portfolio transactions from database")
		trx.Rollback(context.Background())
		return err
	}

	transactions := make([]Transaction, 0, 1000)
	for rows.Next() {
		t := Transaction{}
		err := rows.Scan(&t.Date, &t.Ticker, &t.Kind, &t.PricePerShare, &t.Shares, &t.TotalValue, &t.Justification)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":       err,
				"PortfolioID": p.ID,
				"UserID":      p.UserID,
				"Query":       transactionSQL,
			}).Warn("failed scanning row into transaction fields")
			trx.Rollback(context.Background())
			return err
		}
		transactions = append(transactions, t)
	}
	p.Transactions = transactions
	return nil
}

func LoadFromDB(portfolioID uuid.UUID, userID string) (*Portfolio, error) {
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("failed to create database transaction for user")
		return nil, err
	}

	p := Portfolio{}
	portfolioSQL := `SELECT name, strategy_shortcode, arguments, start_date, end_date, notifications FROM portfolio_v1 WHERE id=$1 AND user_id=$2`
	err = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID).Scan(&p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &p.Notifications)
	if err != nil {
		log.WithFields(log.Fields{
			"UserID":      userID,
			"PortfolioID": portfolioID,
			"Query":       portfolioSQL,
			"Error":       err,
		}).Warn("could not load portfolio from database")
		trx.Rollback(context.Background())
		return nil, err
	}
	trx.Commit(context.Background())

	if err := p.loadTransactionsFromDB(); err != nil {
		// Error is logged by loadTransactionsFromDB
		return nil, err
	}

	if err := p.loadMeasurementsFromDB(); err != nil {
		// Error is logged by loadMeasurementsFromDB
		return nil, err
	}

	return &p, nil
}

func LoadPerformanceFromDB(portfolioID uuid.UUID, userID string) (*Performance, error) {
	perfTable := make(map[string]time.Duration)
	s := time.Now()
	portfolioSQL := `SELECT performance_json FROM portfolio_v1 WHERE id=$1 AND user_id=$2`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolioPerformance",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return nil, fiber.ErrServiceUnavailable
	}

	p := Performance{}
	err = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID).Scan(&p)

	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolioPerformance",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("GetPortfolioPerformance failed")
		trx.Rollback(context.Background())
		return nil, fiber.ErrNotFound
	}
	e := time.Now()
	perfTable["portfolio"] = e.Sub(s)

	// Get transactions
	s = time.Now()
	transactionSQL := "SELECT event_date, ticker, transaction_type, price_per_share, num_shares, total_value, justification FROM portfolio_transaction_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY sequence_num"
	rows, err := trx.Query(context.Background(), transactionSQL, portfolioID, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolioPerformance:Transaction",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
			"Query":       transactionSQL,
		}).Warn("GetPortfolioPerformance failed")
		trx.Rollback(context.Background())
		return nil, fiber.ErrInternalServerError
	}

	transactions := make([]Transaction, 0, 1000)
	for rows.Next() {
		t := Transaction{}
		err := rows.Scan(&t.Date, &t.Ticker, &t.Kind, &t.PricePerShare, &t.Shares, &t.TotalValue, &t.Justification)
		if err != nil {
			log.WithFields(log.Fields{
				"Endpoint":    "GetPortfolioPerformance:Transaction-Iterator",
				"Error":       err,
				"PortfolioID": portfolioID,
				"UserID":      userID,
				"Query":       transactionSQL,
			}).Warn("GetPortfolioPerformance failed")
			trx.Rollback(context.Background())
			return nil, fiber.ErrInternalServerError
		}
		transactions = append(transactions, t)
	}
	p.Transactions = transactions
	e = time.Now()
	perfTable["transactions"] = e.Sub(s)

	// load measurements
	s = time.Now()
	measurementSQL := "SELECT extract(epoch from event_date), value, risk_free_value, holdings, percent_return, justification FROM portfolio_measurement_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY event_date"
	rows, err = trx.Query(context.Background(), measurementSQL, portfolioID, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Endpoint":    "GetPortfolioPerformance:Measurement",
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
			"Query":       measurementSQL,
		}).Warn("GetPortfolioPerformance failed")
		trx.Rollback(context.Background())
		return nil, fiber.ErrInternalServerError
	}

	measurements := make([]PerformanceMeasurement, 0, 1000)
	for rows.Next() {
		m := PerformanceMeasurement{}
		err := rows.Scan(&m.Time, &m.Value, &m.RiskFreeValue, &m.Holdings, &m.PercentReturn, &m.Justification)
		if err != nil {
			log.WithFields(log.Fields{
				"Endpoint":    "GetPortfolioPerformance:Measurement-Iterator",
				"Error":       err,
				"PortfolioID": portfolioID,
				"UserID":      userID,
				"Query":       measurementSQL,
			}).Warn("GetPortfolioPerformance failed")
			trx.Rollback(context.Background())
			return nil, fiber.ErrInternalServerError
		}
		measurements = append(measurements, m)
	}
	p.Measurements = measurements
	e = time.Now()

	perfTable["measurements"] = e.Sub(s)

	fmt.Println("PerfTable:")
	fmt.Println(perfTable)

	return &p, nil
}

// Private API

// computeTransactionSourceID calculates a 16-byte blake3 hash using the date, source,
//   composite figi, ticker, kind, price per share, shares, and total value
func computeTransactionSourceID(t *Transaction) error {
	h := blake3.New()

	// Date as UTC unix timestamp (second precision)
	// NOTE: casting to uint64 doesn't change the sign bit here
	d, err := t.Date.UTC().MarshalText()
	if err != nil {
		return err
	}
	h.Write(d)

	h.Write([]byte(t.Source))
	h.Write([]byte(t.CompositeFIGI))
	h.Write([]byte(t.Ticker))
	h.Write([]byte(t.Kind))
	h.Write([]byte(fmt.Sprintf("%.5f", t.PricePerShare)))
	h.Write([]byte(fmt.Sprintf("%.5f", t.Shares)))
	h.Write([]byte(fmt.Sprintf("%.5f", t.TotalValue)))

	digest := h.Digest()
	buf := make([]byte, 16)
	n, err := digest.Read(buf)
	if err != nil {
		return err
	}
	if n != 16 {
		return errors.New("generate hash failed -- couldn't read 16 bytes from digest")
	}

	t.SourceID = buf
	return nil
}
