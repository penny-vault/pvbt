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

// PortfolioModel stores a portfolio and associated price data that is used for computation
type PortfolioModel struct {
	Portfolio *Portfolio

	// private
	dataProxy      *data.Manager
	holdings       map[string]float64
	justifications map[string][]*Justification
	dividendData   map[string]*dataframe.DataFrame
	splitData      map[string]*dataframe.DataFrame
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, startDate time.Time, initial float64, manager *data.Manager) *PortfolioModel {
	id, _ := uuid.New().MarshalBinary()
	p := Portfolio{
		ID:           id,
		Name:         name,
		Benchmark:    "VFINX",
		Transactions: []*Transaction{},
		StartDate:    startDate,
	}

	model := PortfolioModel{
		Portfolio:      &p,
		dataProxy:      manager,
		justifications: make(map[string][]*Justification),
	}

	model.dataProxy.Begin = startDate
	model.dataProxy.End = time.Now()

	// Create initial deposit
	trxID, _ := uuid.New().MarshalBinary()
	t := Transaction{
		ID:            trxID,
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
	p.Transactions = append(p.Transactions, &t)
	model.holdings = map[string]float64{
		"$CASH": initial,
	}

	model.dividendData = make(map[string]*dataframe.DataFrame)
	model.splitData = make(map[string]*dataframe.DataFrame)

	return &model
}

func buildHoldingsArray(date time.Time, holdings map[string]float64) []*Holding {
	holdingArray := make([]*Holding, 0, len(holdings))
	for k, v := range holdings {
		h := &Holding{
			Date:   date,
			Ticker: k,
			Shares: v,
		}
		holdingArray = append(holdingArray, h)
	}
	return holdingArray
}

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (pm *PortfolioModel) RebalanceTo(date time.Time, target map[string]float64, justification []*Justification) error {
	p := pm.Portfolio
	err := pm.FillCorporateActions(date)
	if err != nil {
		return err
	}

	nTrx := len(p.Transactions)
	if nTrx > 0 {
		lastDate := p.Transactions[nTrx-1].Date
		//fmt.Printf("%s %s %s\n", p.Transactions[nTrx-1].Date, p.Transactions[nTrx-1].Kind, p.Transactions[nTrx-1].Ticker)
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

	// cash position of the portfolio
	var cash float64
	if currCash, ok := pm.holdings["$CASH"]; ok {
		cash += currCash
	}

	// get the current value of non-cash holdings
	var securityValue float64
	priceMap := map[string]float64{
		"$CASH": 1.0,
	}
	for k, v := range pm.holdings {
		if k != "$CASH" {
			price, err := pm.dataProxy.Get(date, data.MetricClose, k)
			if err != nil {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
					"Price":  price,
				}).Warn("security price data not available.")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}

			log.Debugf("[Retrieve price data] Date = %s, Ticker = %s, Price = %.5f, Value = %.5f", date, k, price, v*price)

			if !math.IsNaN(price) {
				securityValue += v * price
			}
			priceMap[k] = price
		}
	}

	// get any prices that we haven't already loaded
	for k := range target {
		if _, ok := priceMap[k]; !ok {
			price, err := pm.dataProxy.Get(date, data.MetricClose, k)
			if err != nil {
				log.WithFields(log.Fields{
					"Symbol": k,
					"Date":   date,
				}).Warn("security price data not available")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}
			priceMap[k] = price
		}
	}

	investable := cash + securityValue

	// process all targets
	sells := make([]*Transaction, 0, 10)
	buys := make([]*Transaction, 0, 10)

	// sell any holdings that we no longer want
	for k, v := range pm.holdings {
		if k == "$CASH" {
			continue
		}

		if v <= 1.0e-5 {
			log.WithFields(log.Fields{
				"Ticker":   k,
				"Kind":     "SellTransaction",
				"Shares":   v,
				"Holdings": pm.holdings,
			}).Warn("holdings are out of sync")
			return errors.New("holdings are out of sync, cannot rebalance portfolio")
		}

		if _, ok := target[k]; !ok {
			price := priceMap[k]
			if math.IsNaN(priceMap[k]) {
				log.Warnf("%s price is not known - writing off asset", k)
				price = 0.0
				fmt.Printf("\tcash = %.2f  v = %.2f  price = %.2f\n", cash, v, price)
			}
			trxId, _ := uuid.New().MarshalBinary()
			t := Transaction{
				ID:            trxId,
				Date:          date,
				Ticker:        k,
				Kind:          SellTransaction,
				PricePerShare: price,
				Shares:        v,
				TotalValue:    v * price,
				Justification: justification,
				Source:        SourceName,
			}

			cash += v * price

			err := computeTransactionSourceID(&t)
			if err != nil {
				log.WithFields(log.Fields{
					"Error":             err,
					"TransactionDate":   date,
					"TransactionTicker": k,
					"TransactionType":   SellTransaction,
				}).Warn("couldn't compute SourceID for transaction")
			}

			sells = append(sells, &t)
		}
	}

	newHoldings := make(map[string]float64)
	for k, v := range target {
		// is this security currently held and should we sell it?
		if holding, ok := pm.holdings[k]; ok {
			targetDollars := investable * v
			price := priceMap[k]
			currentDollars := holding * priceMap[k]
			if (targetDollars / priceMap[k]) > 1.0e-5 {
				if !math.IsNaN(price) {
					newHoldings[k] = targetDollars / price
				}
			}
			if targetDollars < currentDollars {
				// Need to sell to target amount
				if math.IsNaN(price) {
					log.Fatalf("No known price for asset %s; Cannot sell partial", k)
					price = 0.0
					newHoldings[k] = 0.0
				}

				toSellDollars := currentDollars - targetDollars
				toSellShares := toSellDollars / price
				if toSellDollars <= 1.0e-5 {
					log.WithFields(log.Fields{
						"Ticker": k,
						"Kind":   "SellTransaction",
						"Shares": toSellShares,
						"Date":   date,
					}).Warn("holdings are out of sync - refusing to sell 0 shares")
					return errors.New("holdings are out of sync, cannot rebalance portfolio")
				}

				trxId, _ := uuid.New().MarshalBinary()
				t := Transaction{
					ID:            trxId,
					Date:          date,
					Ticker:        k,
					Kind:          SellTransaction,
					PricePerShare: price,
					Shares:        toSellShares,
					TotalValue:    toSellDollars,
					Justification: justification,
				}

				if !math.IsNaN(toSellDollars) {
					cash += toSellDollars
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

				sells = append(sells, &t)
			} else if targetDollars > currentDollars {
				// Need to buy to target amount
				toBuyDollars := targetDollars - currentDollars
				toBuyShares := toBuyDollars / price

				if toBuyShares <= 1.0e-5 {
					log.WithFields(log.Fields{
						"Ticker":     k,
						"Kind":       "BuyTransaction",
						"Shares":     v,
						"TotalValue": toBuyDollars,
						"Date":       date,
					}).Warn("Refusing to buy 0 shares")
				} else if math.IsNaN(price) {
					log.WithFields(log.Fields{
						"Ticker":     k,
						"Kind":       "BuyTransaction",
						"Shares":     v,
						"TotalValue": toBuyDollars,
						"Date":       date,
					}).Warn("Refusing to buy shares of asset with unknown price")
				} else {
					trxId, _ := uuid.New().MarshalBinary()
					t := Transaction{
						ID:            trxId,
						Date:          date,
						Ticker:        k,
						Kind:          BuyTransaction,
						PricePerShare: price,
						Shares:        toBuyShares,
						TotalValue:    toBuyDollars,
						Justification: justification,
					}

					if math.IsNaN(toBuyDollars) {
						log.Warnf("toBuyDollars is NaN %s\n", k)
					} else {
						cash -= toBuyDollars
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

					buys = append(buys, &t)
				}
			}
		} else {
			// this is a new position
			value := investable * v
			price := priceMap[k]
			shares := value / price

			if shares <= 1.0e-5 {
				log.WithFields(log.Fields{
					"Ticker":     k,
					"Kind":       "BuyTransaction",
					"Shares":     v,
					"TotalValue": value,
					"Date":       date,
				}).Warn("Refusing to buy 0 shares")
			} else if math.IsNaN(price) {
				log.WithFields(log.Fields{
					"Ticker":     k,
					"Kind":       "BuyTransaction",
					"Shares":     v,
					"TotalValue": value,
					"Date":       date,
				}).Warn("Refusing to buy shares of asset with unknown price")
			} else {
				newHoldings[k] = shares
				trxId, _ := uuid.New().MarshalBinary()
				t := Transaction{
					ID:            trxId,
					Date:          date,
					Ticker:        k,
					Kind:          BuyTransaction,
					PricePerShare: price,
					Shares:        shares,
					TotalValue:    value,
					Justification: justification,
				}

				cash -= value
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

				buys = append(buys, &t)
			}
		}
	}

	p.Transactions = append(p.Transactions, sells...)
	p.Transactions = append(p.Transactions, buys...)
	if cash > 1.0e-05 {
		newHoldings["$CASH"] = cash
	}
	pm.holdings = newHoldings

	p.CurrentHoldings = buildHoldingsArray(date, newHoldings)

	return nil
}

// TargetPortfolio invests the portfolio in the ratios specified by the dataframe `target`.
//   `target` must have a column named `common.DateIdx` (DATE) and either a string column
//   or MixedAsset column of map[string]float64 where the keys are the tickers and values are
//   the percentages of portfolio to hold
func (pm *PortfolioModel) TargetPortfolio(target *dataframe.DataFrame) error {
	p := pm.Portfolio

	fmt.Println("Building target portfolio")
	if target.NRows() == 0 {
		return nil
	}

	timeIdx, err := target.NameToColumn(common.DateIdx)
	if err != nil {
		return err
	}

	timeSeries := target.Series[timeIdx]

	// Set time range of portfolio
	p.EndDate = timeSeries.Value(timeSeries.NRows() - 1).(time.Time)
	now := time.Now()
	if now.Before(p.EndDate) {
		p.EndDate = now
	}

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

	// Create transactions
	// t3 := time.Now()
	targetIter := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	for {
		row, val, _ := targetIter(dataframe.SeriesName)
		if row == nil {
			break
		}

		// Get next transaction symbol
		var date time.Time
		var symbol interface{}
		justification := make([]*Justification, 0, len(val))

		for k, v := range val {
			idx := k.(string)
			switch idx {
			case common.DateIdx:
				date = v.(time.Time)
			case common.TickerName:
				symbol = v
			default:
				if value, ok := v.(float64); ok {
					j := &Justification{
						Key:   idx,
						Value: value,
					}
					justification = append(justification, j)
				}
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

		pm.justifications[date.String()] = justification

		var rebalance map[string]float64
		if isSingleAsset {
			strSymbol := symbol.(string)
			rebalance = map[string]float64{}
			rebalance[strSymbol] = 1.0
		} else {
			rebalance = symbol.(map[string]float64)
		}

		err = pm.RebalanceTo(date, rebalance, justification)
		if err != nil {
			return err
		}
	}
	// t4 := time.Now()

	/*
		log.WithFields(log.Fields{
			"QuoteDownload":      t2.Sub(t1).Round(time.Millisecond),
			"CreateTransactions": t4.Sub(t3).Round(time.Millisecond),
			"NumRebalances":      target.NRows(),
		}).Info("TargetPortfolio runtimes")
	*/

	return nil
}

// FillCorporateActions finds any corporate actions and creates transactions for them. The
// search occurs from the date of the last transaction to `through`
func (pm *PortfolioModel) FillCorporateActions(through time.Time) error {
	p := pm.Portfolio
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
		log.WithFields(log.Fields{
			"Through": through,
			"Start":   from,
		}).Warn("start date occurs after through date")
		return errors.New("start date occurs after through date")
	}

	// Load split & dividend history
	symbols := []string{}
	cnt := 0
	for k := range pm.holdings {
		if k != "$CASH" {
			cnt++
			// NOTE: splitData and dividendData are always updated in sync so they
			// always match
			if _, ok := pm.dividendData[k]; !ok {
				symbols = append(symbols, k)
			}
		}
	}

	if cnt == 0 {
		return nil // nothing to do
	}

	frequency := pm.dataProxy.Frequency
	pm.dataProxy.Frequency = data.FrequencyDaily
	if len(symbols) > 0 {
		divMap, errs := pm.dataProxy.GetDataFrame(data.MetricDividendCash, symbols...)
		if errs != nil {
			log.Error(errs)
			return errors.New("failed to download dividend data for tickers")
		}
		dateSeriesIdx := divMap.MustNameToColumn(common.DateIdx)
		dateSeries := divMap.Series[dateSeriesIdx]
		for _, series := range divMap.Series {
			if series.Name() == common.DateIdx {
				continue
			}
			// filter to non 0 values
			df := dataframe.NewDataFrame(dateSeries.Copy(), series)
			df, err = dfextras.DropNA(context.Background(), df)
			if err != nil {
				return err
			}
			k := series.Name()
			filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
				if vals[k].(float64) == 0.0 {
					return dataframe.DROP, nil
				}
				return dataframe.KEEP, nil
			})

			dataframe.Filter(context.Background(), df, filterFn, dataframe.FilterOptions{
				InPlace: true,
			})
			pm.dividendData[k] = df
		}

		splitMap, errs := pm.dataProxy.GetDataFrame(data.MetricSplitFactor, symbols...)
		dateSeriesIdx = splitMap.MustNameToColumn(common.DateIdx)
		dateSeries = splitMap.Series[dateSeriesIdx]
		if errs != nil {
			log.Error(errs)
			return errors.New("failed to download split data for tickers")
		}
		for _, series := range splitMap.Series {
			if series.Name() == common.DateIdx {
				continue
			}
			df := dataframe.NewDataFrame(dateSeries.Copy(), series)
			df, err = dfextras.DropNA(context.Background(), df)
			if err != nil {
				return err
			}
			k := series.Name()
			// filter to non 1 values
			filterFn := dataframe.FilterDataFrameFn(func(vals map[interface{}]interface{}, row, nRows int) (dataframe.FilterAction, error) {
				if vals[k].(float64) == 1.0 {
					return dataframe.DROP, nil
				}
				return dataframe.KEEP, nil
			})

			dataframe.Filter(context.Background(), df, filterFn, dataframe.FilterOptions{
				InPlace: true,
			})
			pm.splitData[k] = df
		}
	}
	pm.dataProxy.Frequency = frequency

	addTransactions := make([]*Transaction, 0, 10)

	for k := range pm.holdings {
		if k == "$CASH" {
			continue
		}

		// do dividends
		iterator := pm.dividendData[k].ValuesIterator(
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

			date := divs[common.DateIdx].(time.Time)
			if date.After(from) && date.Before(through) {
				// it's in range
				dividend := divs[k].(float64)
				nShares := pm.holdings[k]
				totalValue := nShares * dividend
				// there is a dividend, record it
				trxId, _ := uuid.New().MarshalBinary()
				t := Transaction{
					ID:            trxId,
					Date:          date,
					Ticker:        k,
					Kind:          DividendTransaction,
					PricePerShare: 1.0,
					TotalValue:    totalValue,
					Justification: nil,
				}
				// update cash position in holdings
				pm.holdings["$CASH"] += nShares * dividend
				computeTransactionSourceID(&t)
				addTransactions = append(addTransactions, &t)
			}
		}

		// do splits
		iterator = pm.splitData[k].ValuesIterator(
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

			date := s[common.DateIdx].(time.Time)
			if date.After(from) && date.Before(through) {
				// it's in range
				splitFactor := s[k].(float64)
				nShares := pm.holdings[k]
				shares := splitFactor * nShares
				// there is a split, record it
				trxId, _ := uuid.New().MarshalBinary()
				t := Transaction{
					ID:            trxId,
					Date:          date,
					Ticker:        k,
					Kind:          SplitTransaction,
					PricePerShare: 0.0,
					Shares:        shares,
					TotalValue:    0,
					Justification: nil,
				}
				computeTransactionSourceID(&t)
				addTransactions = append(addTransactions, &t)

				// update holdings
				pm.holdings[k] = shares
			}
		}
	}

	sort.SliceStable(addTransactions, func(i, j int) bool { return addTransactions[i].Date.Before(addTransactions[j].Date) })
	p.Transactions = append(p.Transactions, addTransactions...)

	return nil
}

// UpdateTransactions calculates new transactions based on the portfolio strategy
// from the portfolio end date to `through`
func (pm *PortfolioModel) UpdateTransactions(through time.Time) error {
	p := pm.Portfolio
	pm.dataProxy.Begin = p.EndDate
	pm.dataProxy.End = through
	pm.dataProxy.Frequency = data.FrequencyDaily

	arguments := make(map[string]json.RawMessage)
	json.Unmarshal([]byte(p.StrategyArguments), &arguments)

	if strategy, ok := strategies.StrategyMap[p.StrategyShortcode]; ok {
		stratObject, err := strategy.Factory(arguments)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":     err,
				"Portfolio": p.ID,
				"Strategy":  p.StrategyShortcode,
			}).Error("failed to initialize portfolio strategy")
			return err
		}

		targetPortfolio, err := stratObject.Compute(pm.dataProxy)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":     err,
				"Portfolio": p.ID,
				"Strategy":  p.StrategyShortcode,
			}).Error("failed to run portfolio strategy")
			return err
		}

		err = pm.TargetPortfolio(targetPortfolio)
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

func (pm *PortfolioModel) LoadTransactionsFromDB() error {
	p := pm.Portfolio
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

	transactions := make([]*Transaction, 0, 1000)
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
		transactions = append(transactions, &t)
	}
	p.Transactions = transactions
	return nil
}

func LoadFromDB(portfolioIDs []string, userID string, dataProxy *data.Manager) ([]*PortfolioModel, error) {
	if userID == "" {
		log.WithFields(log.Fields{
			"PortfolioID": portfolioIDs,
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

	var rows pgx.Rows
	if len(portfolioIDs) > 0 {
		portfolioSQL := `
		SELECT
			id,
			name,
			strategy_shortcode,
			arguments,
			start_date,
			CASE WHEN end_date IS NULL THEN start_date
				ELSE end_DATE
			END AS end_date,
			holdings,
			notifications,
			benchmark
		FROM
			portfolio_v1
		WHERE
			id = ANY ($1) AND user_id=$2`
		rows, err = trx.Query(context.Background(), portfolioSQL, portfolioIDs, userID)
	} else {
		portfolioSQL := `
		SELECT
			id,
			name,
			strategy_shortcode,
			arguments,
			start_date,
			CASE WHEN end_date IS NULL THEN start_date
				ELSE end_DATE
			END AS end_date,
			holdings,
			notifications,
			benchmark
		FROM
			portfolio_v1
		WHERE
			user_id=$1`
		rows, err = trx.Query(context.Background(), portfolioSQL, userID)
	}

	if err != nil {
		log.WithFields(log.Fields{
			"UserID":      userID,
			"PortfolioID": portfolioIDs,
			"Error":       err,
		}).Warn("could not load portfolio from database")
		trx.Rollback(context.Background())
		return nil, err
	}

	sz := len(portfolioIDs)
	if sz == 0 {
		sz = 10
	}
	resultSet := make([]*PortfolioModel, 0, sz)

	for rows.Next() {
		p := &Portfolio{
			UserID:       userID,
			Transactions: []*Transaction{},
		}

		pm := &PortfolioModel{
			Portfolio:      p,
			dataProxy:      dataProxy,
			justifications: make(map[string][]*Justification),
			dividendData:   make(map[string]*dataframe.DataFrame),
			splitData:      make(map[string]*dataframe.DataFrame),
		}

		err = rows.Scan(&p.ID, &p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &pm.holdings, &p.Notifications, &p.Benchmark)
		if err != nil {
			log.WithFields(log.Fields{
				"UserID":      userID,
				"PortfolioID": portfolioIDs,
				"Error":       err,
			}).Warn("could not load portfolio from database")
			trx.Rollback(context.Background())
			return nil, err
		}

		p.CurrentHoldings = buildHoldingsArray(time.Now(), pm.holdings)
		pm.dataProxy.Begin = p.StartDate
		pm.dataProxy.End = time.Now()

		resultSet = append(resultSet, pm)
	}

	if len(resultSet) == 0 && len(portfolioIDs) != 0 {
		return nil, errors.New("requested portfolioID is not in the database")
	}

	trx.Commit(context.Background())
	return resultSet, nil
}

// Save portfolio to database along with all transaction data
func (pm *PortfolioModel) Save(userID string) error {
	p := pm.Portfolio
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

	err = pm.SaveWithTransaction(trx, userID, false)
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

func (pm *PortfolioModel) SaveWithTransaction(trx pgx.Tx, userID string, permanent bool) error {
	temporary := !permanent
	p := pm.Portfolio
	portfolioSQL := `
	INSERT INTO portfolio_v1 (
		"id",
		"name",
		"strategy_shortcode",
		"arguments",
		"start_date",
		"end_date",
		"holdings",
		"notifications",
		"temporary",
		"user_id"
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
		$10
	) ON CONFLICT ON CONSTRAINT portfolio_v1_pkey
	DO UPDATE SET
		name=$2,
		strategy_shortcode=$3,
		arguments=$4,
		start_date=$5,
		end_date=$6,
		holdings=$7,
		notifications=$8`
	holdings, err := json.Marshal(pm.holdings)
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
	_, err = trx.Exec(context.Background(), portfolioSQL, p.ID, p.Name, p.StrategyShortcode,
		p.StrategyArguments, p.StartDate, p.EndDate, holdings, p.Notifications, temporary, userID)
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

	return pm.saveTransactions(trx, userID)
}

func (pm *PortfolioModel) saveTransactions(trx pgx.Tx, userID string) error {
	p := pm.Portfolio
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
		"sequence_num",
		"user_id"
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
		$18,
		$19
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

	now := time.Now()
	for idx, t := range p.Transactions {
		if t.Date.After(now) {
			log.WithFields(log.Fields{
				"TransactionID":   t.ID,
				"TransactionDate": t.Date,
				"Now":             now,
			}).Info("Not saving transaction because it is in the future")
		}
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
			userID,           // 19
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
