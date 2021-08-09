package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"main/common"
	"main/data"
	"main/database"
	"main/dfextras"
	"main/strategies"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
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
	DividendTransaction = "DIVIDEND"
	SplitTransaction    = "SPLIT"
	DepositTransaction  = "DEPOSIT"
	WithdrawTransaction = "WITHDRAW"
)

type Transaction struct {
	ID             uuid.UUID              `json:"id"`
	Cleared        bool                   `json:"cleared"`
	Commission     float64                `json:"commission"`
	CompositeFIGI  string                 `json:"compositeFIGI"`
	Date           time.Time              `json:"date"`
	Justification  map[string]interface{} `json:"justification"`
	Kind           string                 `json:"kind"`
	Memo           string                 `json:"memo"`
	PricePerShare  float64                `json:"pricePerShare"`
	Shares         float64                `json:"shares"`
	Source         string                 `json:"source"`
	SourceID       []byte                 `json:"sourceID"`
	Tags           []string               `json:"tags"`
	TaxDisposition string                 `json:"taxDisposition"`
	Ticker         string                 `json:"ticker"`
	TotalValue     float64                `json:"totalValue"`
}

type Holding struct {
	Date   time.Time
	Ticker string
	Shares float64
}

// Portfolio manage a portfolio
type Portfolio struct {
	ID                uuid.UUID
	UserID            string
	Name              string
	StartDate         time.Time
	EndDate           time.Time
	Benchmark         string
	StrategyShortcode string
	StrategyArguments map[string]json.RawMessage
	Notifications     int
	Transactions      []Transaction
	Holdings          map[string]float64

	// private
	dataProxy    *data.Manager
	priceData    map[string]*dataframe.DataFrame
	dividendData map[string]*dataframe.DataFrame
	splitData    map[string]*dataframe.DataFrame
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, startDate time.Time, initial float64, manager *data.Manager) *Portfolio {
	p := Portfolio{
		ID:           uuid.New(),
		Name:         name,
		Benchmark:    "VFINX",
		Transactions: []Transaction{},
		dataProxy:    manager,
		StartDate:    startDate,
	}

	p.dataProxy.Begin = startDate
	p.dataProxy.End = time.Now()

	// Create initial deposit
	t := Transaction{
		ID:            uuid.New(),
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

	p.dividendData = make(map[string]*dataframe.DataFrame)
	p.splitData = make(map[string]*dataframe.DataFrame)

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
			Time:  date,
			Value: totalVal,
		})
	}

	return values, nil
}

func (p *Portfolio) holdingsOverPeriod(s time.Time, e time.Time) (map[time.Time][]Holding, error) {
	currHoldings := map[string]Holding{}
	periodHoldings := map[time.Time][]Holding{}

	for _, t := range p.Transactions {
		if t.Kind == DepositTransaction || t.Kind == WithdrawTransaction {
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

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (p *Portfolio) RebalanceTo(date time.Time, target map[string]float64, justification map[string]interface{}) error {
	err := p.FillCorporateActions(date)
	if err != nil {
		return err
	}

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

			log.Debugf("[Retrieve price data] Date = %s, Ticker = %s, Price = %.5f, Value = %.5f", date, k, price, v*price)

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
				ID:            uuid.New(),
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
					ID:            uuid.New(),
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
						ID:            uuid.New(),
						Date:          date,
						Ticker:        k,
						Kind:          BuyTransaction,
						PricePerShare: priceMap[k],
						Shares:        toBuyShares,
						TotalValue:    toBuyDollars,
						Justification: justification,
					}

					log.Debugf("[Buy additional shares] Date = %s, Ticker = %s, Price = %.5f, Shares = %.5f, Value = %.5f", date, k, priceMap[k], toBuyShares, toBuyDollars)

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
					ID:            uuid.New(),
					Date:          date,
					Ticker:        k,
					Kind:          BuyTransaction,
					PricePerShare: priceMap[k],
					Shares:        shares,
					TotalValue:    value,
					Justification: justification,
				}

				log.Debugf("[Buy new holding] Date = %s, Ticker = %s, Price = %.5f, Shares = %.5f, Value = %.5f", date, k, priceMap[k], shares, value)

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

	tickerSeriesIdx, err := target.NameToColumn(common.TickerName)
	if err != nil {
		return fmt.Errorf("missing required column: %s", common.TickerName)
	}

	// check series type
	isSingleAsset := false
	series := target.Series[tickerSeriesIdx]
	if series.Type() == "string" {
		isSingleAsset = true
	}

	// Get price data
	iterator := target.Series[tickerSeriesIdx].ValuesIterator()
	assetMap := make(map[string]bool)
	for {
		row, val, _ := iterator()
		if row == nil {
			break
		}

		if isSingleAsset {
			assetMap[val.(string)] = true
		} else {
			// it's multi-asset which means a map of tickers
			for ticker := range val.(map[string]float64) {
				assetMap[ticker] = true
			}
		}
	}

	symbols := []string{}
	for k := range assetMap {
		symbols = append(symbols, k)
	}

	t1 := time.Now()
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
	t2 := time.Now()

	// Create transactions
	t3 := time.Now()
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
			switch idx {
			case data.DateIdx:
				date = val[data.DateIdx].(time.Time)
			case common.TickerName:
				symbol = val[common.TickerName]
			default:
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

		err = p.RebalanceTo(date, rebalance, justification)
		if err != nil {
			return err
		}
	}
	t4 := time.Now()

	log.WithFields(log.Fields{
		"QuoteDownload":      t2.Sub(t1).Round(time.Millisecond),
		"CreateTransactions": t4.Sub(t3).Round(time.Millisecond),
		"NumRebalances":      target.NRows(),
	}).Info("TargetPortfolio runtimes (s)")

	return nil
}

// FillCorporateActions finds any corporate actions and creates transactions for them. The
// search occurs from the date of the last transaction to `through`
func (p *Portfolio) FillCorporateActions(through time.Time) error {
	// nothing to do if there are no transactions
	n := len(p.Transactions)
	if n == 0 {
		return nil
	}

	dt := p.Transactions[n-1].Date
	tz, err := time.LoadLocation("America/New_York")
	if err != nil {
		return err
	}

	// remove time and move to midnight the next day... this
	// ensures that the search is inclusive of through
	through = time.Date(through.Year(), through.Month(), through.Day(), 0, 0, 0, 0, tz)
	through = through.AddDate(0, 0, 1)
	from := time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
	if from.After(through) {
		return errors.New("start date occurs after through date")
	}

	// Load split & dividend history
	symbols := []string{}
	cnt := 0
	for k := range p.Holdings {
		if k != "$CASH" {
			cnt++
			// NOTE: splitData and dividendData are always updated in sync so they
			// always match
			if _, ok := p.dividendData[k]; !ok {
				symbols = append(symbols, k)
			}
		}
	}

	if cnt == 0 {
		return nil // nothing to do
	}

	if len(symbols) > 0 {
		metric := p.dataProxy.Metric
		p.dataProxy.Metric = data.MetricDividendCash
		divMap, errs := p.dataProxy.GetMultipleData(symbols...)
		if len(errs) > 0 {
			log.Error(errs)
			return errors.New("failed to download dividend data for tickers")
		}
		for k, v := range divMap {
			// filter to non 0 values
			filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
				if vals[k].(float64) == 0.0 {
					return dataframe.DROP, nil
				}
				return dataframe.KEEP, nil
			})

			dataframe.Filter(context.Background(), v, filterFn, dataframe.FilterOptions{
				InPlace: true,
			})
			p.dividendData[k] = v
		}

		p.dataProxy.Metric = data.MetricSplitFactor
		splitMap, errs := p.dataProxy.GetMultipleData(symbols...)
		if len(errs) > 0 {
			log.Error(errs)
			return errors.New("failed to download split data for tickers")
		}
		for k, v := range splitMap {
			// filter to non 1 values
			filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
				if vals[k].(float64) == 1.0 {
					return dataframe.DROP, nil
				}
				return dataframe.KEEP, nil
			})

			dataframe.Filter(context.Background(), v, filterFn, dataframe.FilterOptions{
				InPlace: true,
			})
			p.splitData[k] = v
		}

		p.dataProxy.Metric = metric
	}

	addTransactions := make([]Transaction, 0, 10)

	for k := range p.Holdings {
		if k == "$CASH" {
			continue
		}

		// do dividends
		iterator := p.dividendData[k].ValuesIterator(
			dataframe.ValuesOptions{
				InitialRow:   0,
				Step:         1,
				DontReadLock: false,
			},
		)

		for {
			row, divs, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}

			date := divs[data.DateIdx].(time.Time)
			if date.After(from) && date.Before(through) {
				// it's in range
				dividend := divs[k].(float64)
				nShares := p.Holdings[k]
				totalValue := nShares * dividend
				// there is a dividend, record it
				t := Transaction{
					ID:            uuid.New(),
					Date:          date,
					Ticker:        k,
					Kind:          DividendTransaction,
					PricePerShare: 1.0,
					TotalValue:    totalValue,
					Justification: nil,
				}
				// update cash position in holdings
				p.Holdings["$CASH"] += nShares * dividend
				computeTransactionSourceID(&t)
				addTransactions = append(addTransactions, t)
			}
		}

		// do splits
		iterator = p.splitData[k].ValuesIterator(
			dataframe.ValuesOptions{
				InitialRow:   0,
				Step:         1,
				DontReadLock: false,
			},
		)

		for {
			row, s, _ := iterator(dataframe.SeriesName)
			if row == nil {
				break
			}

			date := s[data.DateIdx].(time.Time)
			if date.After(from) && date.Before(through) {
				// it's in range
				splitFactor := s[k].(float64)
				nShares := p.Holdings[k]
				shares := splitFactor * nShares
				// there is a split, record it
				t := Transaction{
					ID:            uuid.New(),
					Date:          date,
					Ticker:        k,
					Kind:          SplitTransaction,
					PricePerShare: 0.0,
					Shares:        shares,
					TotalValue:    0,
					Justification: nil,
				}
				computeTransactionSourceID(&t)
				addTransactions = append(addTransactions, t)

				// update holdings
				p.Holdings[k] = shares
			}
		}
	}

	sort.SliceStable(addTransactions, func(i, j int) bool { return addTransactions[i].Date.Before(addTransactions[j].Date) })
	p.Transactions = append(p.Transactions, addTransactions...)

	return nil
}

// UpdateTransactions calculates new transactions based on the portfolio strategy
// from the portfolio end date to `through`
func (p *Portfolio) UpdateTransactions(manager *data.Manager, through time.Time) error {
	manager.Begin = p.EndDate
	manager.End = through
	manager.Frequency = data.FrequencyDaily

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

// DATABASE FUNCTIONS

// LOAD

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
	transactionSQL := `SELECT
		id,
		event_date,
		cleared,
		commission,
		composite_figi,
		justification,
		transaction_type,
		memo,
		price_per_share,
		num_shares,
		source,
		source_id,
		tags,
		tax_type,
		ticker,
		total_value
	FROM portfolio_transaction_v1
	WHERE portfolio_id=$1 AND user_id=$2
	ORDER BY sequence_num`
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
		err := rows.Scan(&t.ID, &t.Date, &t.Cleared, &t.Commission, &t.CompositeFIGI,
			&t.Justification, &t.Kind, &t.Memo, &t.PricePerShare, &t.Shares, &t.Source,
			&t.SourceID, &t.Tags, &t.TaxDisposition, &t.Ticker, &t.TotalValue)
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

func LoadFromDB(portfolioID uuid.UUID, userID string, dataProxy *data.Manager) (*Portfolio, error) {
	if userID == "" {
		log.WithFields(log.Fields{
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("userID cannot be an empty string")
		return nil, errors.New("userID cannot be an empty string")
	}
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("failed to create database transaction for user")
		return nil, err
	}

	p := Portfolio{
		ID:           portfolioID,
		UserID:       userID,
		Transactions: []Transaction{},
		dataProxy:    dataProxy,
	}

	p.dividendData = make(map[string]*dataframe.DataFrame)
	p.splitData = make(map[string]*dataframe.DataFrame)

	portfolioSQL := `
	SELECT
		name,
		strategy_shortcode,
		arguments,
		start_date,
		CASE WHEN end_date IS NULL THEN start_date
			ELSE end_DATE
		END AS end_date,
		holdings,
		notifications
	FROM
		portfolio_v1
	WHERE
		id=$1 AND user_id=$2`
	err = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID).Scan(&p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &p.Holdings, &p.Notifications)
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

	p.dataProxy.Begin = p.StartDate
	p.dataProxy.End = time.Now()

	if err := p.loadTransactionsFromDB(); err != nil {
		// Error is logged by loadTransactionsFromDB
		return nil, err
	}

	return &p, nil
}

// Save portfolio to database along with all transaction data
func (p *Portfolio) Save(userID string) error {
	p.UserID = userID

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":  err,
			"UserID": userID,
		}).Error("unable to get database transaction for user")
		return err
	}

	err = p.SaveWithTransaction(trx, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID": p.ID,
			"Strategy":    p.StrategyShortcode,
			"Error":       err,
		}).Warn("failed to create portfolio transactions")
		trx.Rollback(context.Background())
		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID": p.ID,
			"Strategy":    p.StrategyShortcode,
			"Error":       err,
		}).Warn("failed to commit portfolio transaction")
		trx.Rollback(context.Background())
		return err
	}

	return nil
}

func (p *Portfolio) SaveWithTransaction(trx pgx.Tx, userID string) error {
	portfolioSQL := `
	INSERT INTO portfolio_v1 (
		"id",
		"name",
		"strategy_shortcode",
		"arguments",
		"start_date",
		"end_date",
		"holdings",
		"notifications"
	) VALUES (
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7,
		$8
	) ON CONFLICT ON CONSTRAINT portfolio_v1_pkey
	DO UPDATE SET
		name=$2,
		strategy_shortcode=$3,
		arguments=$4,
		start_date=$5,
		end_date=$6,
		holdings=$7,
		notifications=$8`
	holdings, err := json.Marshal(p.Holdings)
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID": p.ID,
			"Strategy":    p.StrategyShortcode,
			"Error":       err,
			"Query":       portfolioSQL,
		}).Warn("failed to marshal holdings")
		trx.Rollback(context.Background())
		return err
	}
	_, err = trx.Exec(context.Background(), portfolioSQL, p.ID, p.Name, p.StrategyShortcode, p.StrategyArguments, p.StartDate, p.EndDate, holdings, p.Notifications)
	if err != nil {
		log.WithFields(log.Fields{
			"PortfolioID": p.ID,
			"Strategy":    p.StrategyShortcode,
			"Error":       err,
			"Query":       portfolioSQL,
		}).Warn("failed to save portfolio")
		trx.Rollback(context.Background())
		return err
	}

	return p.saveTransactions(trx)
}

func (p *Portfolio) saveTransactions(trx pgx.Tx) error {
	transactionSQL := `
	INSERT INTO portfolio_transaction_v1 (
		"id",
		"portfolio_id",
		"transaction_type",
		"cleared",
		"commission",
		"composite_figi",
		"event_date",
		"justification",
		"memo",
		"price_per_share",
		"num_shares",
		"source",
		"source_id",
		"tags",
		"tax_type",
		"ticker",
		"total_value",
		"sequence_num"
	) VALUES (
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7,
		$8,
		$9,
		$10,
		$11,
		$12,
		$13,
		$14,
		$15,
		$16,
		$17,
		$18
	) ON CONFLICT ON CONSTRAINT portfolio_transaction_v1_pkey
 	DO UPDATE SET
		transaction_type=$3,
		cleared=$4,
		commission=$5,
		composite_figi=$6,
		event_date=$7,
		justification=$8,
		memo=$9,
		price_per_share=$10,
		num_shares=$11,
		source=$12,
		source_id=$13,
		tags=$14,
		tax_type=$15,
		ticker=$16,
		total_value=$17,
		sequence_num=$18`

	for idx, t := range p.Transactions {
		if t.TaxDisposition == "" {
			t.TaxDisposition = "TAXABLE"
		}
		_, err := trx.Exec(context.Background(), transactionSQL,
			t.ID,             // 1
			p.ID,             // 2
			t.Kind,           // 3
			t.Cleared,        // 4
			t.Commission,     // 5
			t.CompositeFIGI,  // 6
			t.Date,           // 7
			t.Justification,  // 8
			t.Memo,           // 9
			t.PricePerShare,  // 10
			t.Shares,         // 11
			t.Source,         // 12
			t.SourceID,       // 13
			t.Tags,           // 14
			t.TaxDisposition, // 15
			t.Ticker,         // 16
			t.TotalValue,     // 17
			idx,              // 18
		)
		if err != nil {
			log.WithFields(log.Fields{
				"PortfolioID":   p.ID,
				"TransactionID": t.ID,
				"Error":         err,
				"Query":         transactionSQL,
			}).Warn("failed to save portfolio")
			trx.Rollback(context.Background())
			return err
		}
	}

	return nil
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
