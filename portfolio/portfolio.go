package portfolio

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"main/dfextras"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/rocketlaunchr/dataframe-go"
	log "github.com/sirupsen/logrus"
)

const (
	TickerName = "TICKER"
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
	Kind          string                 `json:"kind"`
	PricePerShare float64                `json:"pricePerShare"`
	Shares        float64                `json:"shares"`
	TotalValue    float64                `json:"totalValue"`
	Justification map[string]interface{} `json:"justification"`
}

type Holding struct {
	Date   time.Time
	Ticker string
	Shares float64
}

// Portfolio manage a portfolio
type Portfolio struct {
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Transactions []Transaction
	Holdings     map[string]float64
	dataProxy    *data.Manager
	securities   map[string]bool
	priceData    map[string]*dataframe.DataFrame
}

type PerformanceMeasurement struct {
	Time          int64                  `json:"time"`
	Value         float64                `json:"value"`
	RiskFreeValue float64                `json:"riskFreeValue"`
	Holdings      string                 `json:"holdings"`
	PercentReturn float64                `json:"percentReturn"`
	Justification map[string]interface{} `json:"justification"`
}

// Performance of portfolio
type Performance struct {
	PeriodStart        int64                    `json:"periodStart"`
	PeriodEnd          int64                    `json:"periodEnd"`
	Measurements       []PerformanceMeasurement `json:"measurements"`
	Transactions       []Transaction            `json:"transactions"`
	CagrSinceInception float64                  `json:"cagrSinceInception"`
	YTDReturn          float64                  `json:"ytdReturn"`
	CurrentAsset       string                   `json:"currentAsset"`
	TotalDeposited     float64                  `json:"totalDeposited"`
	TotalWithdrawn     float64                  `json:"totalWithdrawn"`
	MetricsBundle      MetricsBundle            `json:"metrics"`
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, manager *data.Manager) Portfolio {
	return Portfolio{
		Name:      name,
		dataProxy: manager,
	}
}

// ValueAsOf return the value of the portfolio for the given date
func (p *Portfolio) ValueAsOf(d time.Time) (float64, error) {
	// Get last 7 days of values, in case 'd' isn't a market day
	s := d.AddDate(0, 0, -7)
	value, err := p.valueOverPeriod(s, d)
	sz := len(value)
	if sz <= 0 {
		return 0, errors.New("Failed to compute value for date")
	}
	return value[sz-1].Value, err
}

func (p *Portfolio) valueOverPeriod(s time.Time, e time.Time) ([]*PerformanceMeasurement, error) {
	if len(p.Transactions) == 0 {
		return nil, errors.New("Cannot calculate performance for portfolio with no transactions")
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
		return nil, errors.New("Failed to download data for tickers")
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
	for {
		row, quotes, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}
		date := quotes[data.DateIdx].(time.Time)
		year, month, day := date.Date()
		date = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

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
				return nil, errors.New("Transactions are out of order")
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
func (p *Portfolio) CalculatePerformance(through time.Time) (Performance, error) {
	if len(p.Transactions) == 0 {
		return Performance{}, errors.New("Cannot calculate performance for portfolio with no transactions")
	}

	perf := Performance{
		PeriodStart:  p.StartTime.Unix(),
		PeriodEnd:    through.Unix(),
		Transactions: p.Transactions,
	}

	// Calculate performance
	symbols := []string{}
	for k := range p.securities {
		symbols = append(symbols, k)
	}

	p.dataProxy.Begin = p.StartTime
	p.dataProxy.End = through
	p.dataProxy.Frequency = data.FrequencyMonthly

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
				return Performance{}, errors.New("unrecognized transaction type")
			}

			// Protect against floating point noise
			if shares <= 1.0e-5 {
				shares = 0
			}

			holdings[trx.Ticker] = shares
		}

		// iterate through each holding and add value to get total return
		totalVal = 0.0
		var tickers []string
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
				return Performance{}, fmt.Errorf("no quote for symbol: %s", symbol)
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
		duration := date.Sub(p.StartTime).Hours() / (24 * 365.25)
		cagrSinceInception = math.Pow(totalVal/startVal, 1.0/duration) - 1
		prevVal = totalVal

		valueOverTime = append(valueOverTime, PerformanceMeasurement{
			Time:          date.Unix(),
			Value:         totalVal,
			RiskFreeValue: riskFreeValue,
			Holdings:      holdingStr,
			PercentReturn: ret,
			Justification: lastJustification,
		})

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAsset = holdingStr
		}
	}

	perf.Measurements = valueOverTime
	perf.CagrSinceInception = cagrSinceInception

	if currYearStartValue <= 0 {
		perf.YTDReturn = 0.0
	} else {
		perf.YTDReturn = totalVal/currYearStartValue - 1.0
	}

	return perf, nil
}

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (p *Portfolio) RebalanceTo(date time.Time, target map[string]float64, justification map[string]interface{}) error {
	nTrx := len(p.Transactions)
	if nTrx > 0 {
		lastDate := p.Transactions[nTrx-1].Date
		if lastDate.After(date) {
			return fmt.Errorf("Cannot rebalance portfolio on date %s when last existing transaction date is %s", date.String(), lastDate.String())
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
		return fmt.Errorf("Rebalance percent total does not equal 1.0, it is %.2f", total)
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
			res, err := dfextras.FindTime(context.TODO(), eod, date, data.DateIdx)
			if err != nil {
				return err
			}

			var price float64
			if tmp, ok := res[k]; ok {
				price = tmp.(float64)
			} else {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
				}).Debug("Security purchased before security price was available")
				return fmt.Errorf("Security %s price data not available for date %s", k, date.String())
			}

			securityValue += v * price
			priceMap[k] = price
		}
	}

	// get any prices that we haven't already loaded
	for k := range target {
		if _, ok := priceMap[k]; !ok {
			eod := p.priceData[k]
			res, err := dfextras.FindTime(context.TODO(), eod, date, data.DateIdx)
			if err != nil {
				return err
			}

			var price float64
			if tmp, ok := res[k]; ok {
				price = tmp.(float64)
			} else {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
				}).Debug("Security purchased before security price was available")
				return fmt.Errorf("Security %s price data not available for date %s", k, date.String())
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
		if _, ok := target[k]; !ok {
			t := Transaction{
				Date:          date,
				Ticker:        k,
				Kind:          SellTransaction,
				PricePerShare: priceMap[k],
				Shares:        v,
				TotalValue:    v * priceMap[k],
				Justification: justification,
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
			newHoldings[k] = targetDollars / priceMap[k]
			if targetDollars < currentDollars {
				// Need to sell to target amount
				toSellDollars := currentDollars - targetDollars
				toSellShares := toSellDollars / priceMap[k]
				t := Transaction{
					Date:          date,
					Ticker:        k,
					Kind:          SellTransaction,
					PricePerShare: priceMap[k],
					Shares:        toSellShares,
					TotalValue:    toSellDollars,
					Justification: justification,
				}
				sells = append(sells, t)
			}
			if targetDollars > currentDollars {
				// Need to buy to target amount
				toBuyDollars := targetDollars - currentDollars
				toBuyShares := toBuyDollars / priceMap[k]
				t := Transaction{
					Date:          date,
					Ticker:        k,
					Kind:          BuyTransaction,
					PricePerShare: priceMap[k],
					Shares:        toBuyShares,
					TotalValue:    toBuyDollars,
					Justification: justification,
				}
				buys = append(buys, t)
			}
		} else {
			// this is a new position
			value := investable * v
			shares := value / priceMap[k]
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
			buys = append(buys, t)
		}
	}
	p.Transactions = append(p.Transactions, sells...)
	p.Transactions = append(p.Transactions, buys...)
	p.Holdings = newHoldings

	return nil
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

	p.securities = make(map[string]bool)
	tickerSeriesIdx, err := target.NameToColumn(TickerName)
	if err != nil {
		return fmt.Errorf("Missing required column: %s", TickerName)
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
		return errors.New("Failed loading data for tickers")
	}
	p.priceData = prices

	// Create transactions
	targetIter := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	var first bool = true
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
			} else if idx == TickerName {
				symbol = val[TickerName]
			} else {
				justification[idx] = v
			}
		}

		if first {
			first = false
			// Create initial deposit
			p.Transactions = append(p.Transactions, Transaction{
				Date:          date,
				Ticker:        "$CASH",
				Kind:          DepositTransaction,
				PricePerShare: 1.0,
				Shares:        initial,
				TotalValue:    initial,
				Justification: justification,
			})
			p.Holdings = map[string]float64{
				"$CASH": initial,
			}
		}

		var rebalance map[string]float64
		if isSingleAsset {
			strSymbol := symbol.(string)
			rebalance = map[string]float64{}
			rebalance[strSymbol] = 1.0
		} else {
			rebalance = symbol.(map[string]float64)
		}

		p.Transactions = append(p.Transactions, Transaction{
			Date:          date,
			Kind:          MarkerTransaction,
			Justification: justification,
		})
		err = p.RebalanceTo(date, rebalance, justification)
		if err != nil {
			return err
		}
	}

	return nil
}
