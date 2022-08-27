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

package portfolio

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jdfergason/dataframe-go"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dfextras"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"
	"github.com/zeebo/blake3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrEmptyUserID               = errors.New("user id empty")
	ErrStrategyNotFound          = errors.New("strategy not found")
	ErrHoldings                  = errors.New("holdings are out of sync, cannot rebalance portfolio")
	ErrInvalidSell               = errors.New("refusing to sell 0 shares - cannot rebalance portfolio; target allocation broken")
	ErrTimeInverted              = errors.New("start date occurs after through date")
	ErrPortfolioNotFound         = errors.New("could not find portfolio ID in database")
	ErrGenerateHash              = errors.New("could not create a new hash")
	ErrTransactionsOutOfOrder    = errors.New("transactions would be out-of-order if executed")
	ErrRebalancePercentWrong     = errors.New("rebalance total must equal 1.0")
	ErrSecurityPriceNotAvailable = errors.New("security price not available for date")
	ErrNoTickerColumn            = errors.New("no ticker column present")
	ErrInvalidDateRange          = errors.New("invalid date range")
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

type Activity struct {
	Date time.Time
	Msg  string
	Tags []string
}

// Model stores a portfolio and associated price data that is used for computation
type Model struct {
	Portfolio *Portfolio

	// private
	dataProxy      *data.Manager
	value          float64
	holdings       map[data.Security]float64
	activities     []*Activity
	justifications map[string][]*Justification
}

type Period struct {
	Begin time.Time
	End   time.Time
}

// NewPortfolio create a portfolio
func NewPortfolio(name string, startDate time.Time, initial float64, manager *data.Manager) *Model {
	id, _ := uuid.New().MarshalBinary()
	p := Portfolio{
		ID:           id,
		Name:         name,
		Benchmark:    "VFINX",
		Transactions: []*Transaction{},
		StartDate:    startDate,
	}

	model := Model{
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
		Ticker:        data.CashAsset,
		Kind:          DepositTransaction,
		PricePerShare: 1.0,
		Shares:        initial,
		TotalValue:    initial,
		Justification: nil,
	}

	err := computeTransactionSourceID(&t)
	if err != nil {
		log.Warn().Stack().Err(err).Time("TransactionDate", startDate).Str("TransactionTicker", data.CashAsset).Str("TransactionType", DepositTransaction).Msg("couldn't compute SourceID for initial deposit")
	}
	p.Transactions = append(p.Transactions, &t)
	model.holdings = map[data.Security]float64{
		data.CashSecurity: initial,
	}

	return &model
}

func buildHoldingsArray(date time.Time, holdings map[data.Security]float64) []*Holding {
	holdingArray := make([]*Holding, 0, len(holdings))
	for k, v := range holdings {
		h := &Holding{
			Date:          date,
			CompositeFIGI: k.CompositeFigi,
			Ticker:        k.Ticker,
			Shares:        v,
		}
		holdingArray = append(holdingArray, h)
	}
	return holdingArray
}

func (pm *Model) getPortfolioSecuritiesValue(ctx context.Context, date time.Time) (float64, error) {
	var securityValue float64

	for k, v := range pm.holdings {
		if k != data.CashSecurity {
			price, err := pm.dataProxy.Get(ctx, date, data.MetricClose, &k)
			if err != nil {
				log.Warn().Stack().Str("Symbol", k.Ticker).Time("Date", date).Float64("Price", price).Msg("security price data not available.")
				return 0, ErrSecurityPriceNotAvailable
			}

			log.Debug().Time("Date", date).Str("Ticker", k.Ticker).Float64("Price", price).Float64("Value", v*price).Msg("Retrieve price data")

			if !math.IsNaN(price) {
				securityValue += v * price
			}
		}
	}

	return securityValue, nil
}

func createTransaction(date time.Time, security *data.Security, kind string, price float64, shares float64, justification []*Justification) (*Transaction, error) {
	trxID, err := uuid.New().MarshalBinary()
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not marshal uuid to binary")
		return nil, err
	}
	t := Transaction{
		ID:            trxID,
		Date:          date,
		Ticker:        security.Ticker,
		CompositeFIGI: security.CompositeFigi,
		Kind:          kind,
		PricePerShare: price,
		Shares:        shares,
		TotalValue:    shares * price,
		Justification: justification,
		Source:        SourceName,
	}

	err = computeTransactionSourceID(&t)
	if err != nil {
		log.Warn().Stack().Err(err).Time("TransactionDate", date).Str("TransactionTicker", security.Ticker).Str("TransactionType", kind).Msg("couldn't compute SourceID for transaction")
	}

	return &t, nil
}

func (pm *Model) getPriceSafe(ctx context.Context, date time.Time, security *data.Security) float64 {
	price, err := pm.dataProxy.Get(ctx, date, data.MetricClose, security)
	if err != nil {
		log.Warn().Stack().Err(err).Str("Ticker", security.Ticker).Msg("dataProxy.Get returned an error")
		price = 0.0
	}
	if math.IsNaN(price) {
		log.Warn().Stack().Str("Ticker", security.Ticker).Msg("price is NaN")
		price = 0.0
	}
	return price
}

func (pm *Model) modifyPosition(ctx context.Context, date time.Time, security *data.Security, targetDollars float64, justification []*Justification) (*Transaction, float64, error) {
	// is this security currently held? If so, do we need to buy more or sell some
	price := pm.getPriceSafe(ctx, date, security)
	subLog := log.With().Float64("Price", price).Time("Date", date).Str("Ticker", security.Ticker).Float64("TargetDollars", targetDollars).Logger()
	if price < 1.0e-5 {
		subLog.Error().Stack().Msg("cannot purchase an asset when price is 0; skip asset")
		return nil, 0, ErrSecurityPriceNotAvailable
	}
	targetShares := targetDollars / price
	var t *Transaction
	var err error

	if currentNumShares, ok := pm.holdings[*security]; ok {
		currentDollars := currentNumShares * price
		diff := targetDollars - currentDollars

		if math.Abs(diff) < 1.0e-5 {
			// already hold what we need
			return nil, targetShares, nil
		}

		if diff < 0 {
			// Need to sell to target amount
			toSellDollars := currentDollars - targetDollars
			toSellShares := toSellDollars / price
			if toSellDollars <= 1.0e-5 || toSellShares <= 1.0e-5 {
				log.Error().Stack().
					Str("Ticker", security.Ticker).
					Str("Kind", "SellTransaction").
					Float64("Shares", toSellShares).
					Time("Date", date).
					Float64("Price", price).
					Float64("CurrentDollars", currentDollars).
					Float64("TargetDollars", targetDollars).
					Msg("refusing to sell 0 shares")
				return nil, currentNumShares, nil
			}

			t, err = createTransaction(date, security, SellTransaction, price, toSellShares, justification)
			if err != nil {
				return nil, 0, err
			}
		} else {
			// Need to buy to target amount
			toBuyDollars := targetDollars - currentDollars
			toBuyShares := toBuyDollars / price

			subLog := log.With().Str("Ticker", security.Ticker).Str("Kind", "BuyTransaction").Float64("Shares", toBuyShares).Float64("TotalValue", toBuyDollars).Time("Date", date).Logger()
			if toBuyShares <= 1.0e-5 {
				subLog.Warn().Stack().Msg("refusing to buy 0 shares")
				return nil, currentNumShares, nil
			}

			if math.IsNaN(toBuyDollars) {
				subLog.Warn().Stack().Msg("toBuyDollars is NaN")
				return nil, 0, ErrSecurityPriceNotAvailable
			}

			t, err = createTransaction(date, security, BuyTransaction, price, toBuyShares, justification)
			if err != nil {
				return nil, 0, err
			}

			subLog.Debug().Msg("Buy additional shares")
		}
	} else {
		// this is a new position
		shares := targetDollars / price
		subLog := log.With().Str("Ticker", security.Ticker).Str("Kind", "BuyTransaction").Float64("Shares", shares).Float64("TotalValue", targetDollars).Time("Date", date).Logger()
		if shares <= 1.0e-5 {
			subLog.Warn().Stack().Msg("refusing to buy 0 shares")
			return nil, 0, ErrRebalancePercentWrong
		}

		t, err = createTransaction(date, security, BuyTransaction, price, shares, justification)
		if err != nil {
			return nil, 0, err
		}
		subLog.Debug().Msg("buy new holding")
	}

	return t, targetShares, nil
}

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (pm *Model) RebalanceTo(ctx context.Context, date time.Time, target map[data.Security]float64, justification []*Justification) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "RebalanceTo")
	defer span.End()

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "date",
			Value: attribute.StringValue(date.Format("2006-01-02")),
		},
	)

	p := pm.Portfolio
	err := pm.FillCorporateActions(ctx, date)
	if err != nil {
		return err
	}

	nTrx := len(p.Transactions)
	if nTrx > 0 {
		lastDate := p.Transactions[nTrx-1].Date
		if lastDate.After(date) {
			log.Error().Stack().Time("Date", date).Time("LastTransactionDate", lastDate).Msg("cannot rebalance portfolio when date is before last transaction date")
			return ErrTransactionsOutOfOrder
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
		log.Error().Stack().Float64("TotalPercentAllocated", total).Time("Date", date).Msg("TotalPercentAllocated must equal 1.0")
		return ErrRebalancePercentWrong
	}

	// cash position of the portfolio
	var cash float64
	if currCash, ok := pm.holdings[data.CashSecurity]; ok {
		cash += currCash
	}

	// get the current value of non-cash holdings
	securityValue, err := pm.getPortfolioSecuritiesValue(ctx, date)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "security price data not available")
		return err
	}

	// compute the investable value in the portfolio
	investable := cash + securityValue
	pm.value = investable

	// process all targets
	sells := make([]*Transaction, 0, 10)
	buys := make([]*Transaction, 0, 10)

	// sell any holdings that we no longer want
	for security, shares := range pm.holdings {
		if security.Ticker == data.CashAsset {
			continue
		}

		if shares <= 1.0e-5 {
			log.Warn().Stack().Str("Ticker", security.Ticker).Float64("Shares", shares).Msg("holdings are out of sync")
			return ErrHoldings
		}

		if _, ok := target[security]; !ok {
			price := pm.getPriceSafe(ctx, date, &security)
			t, err := createTransaction(date, &security, SellTransaction, price, shares, justification)
			if err != nil {
				return err
			}
			cash += t.TotalValue
			sells = append(sells, t)
		}
	}

	// purchase holdings based on target
	newHoldings := make(map[data.Security]float64)
	for security, targetPercent := range target {
		targetDollars := investable * targetPercent
		t, numShares, err := pm.modifyPosition(ctx, date, &security, targetDollars, justification)
		if err != nil {
			// don't fail if position could not be modified, just continue -- writing off asset as $0
			log.Warn().Err(err).Time("Date", date).Str("Ticker", security.Ticker).Msg("writing off asset")
			continue
		}

		if t != nil {
			switch t.Kind {
			case SellTransaction:
				sells = append(sells, t)
				cash += t.TotalValue
			case BuyTransaction:
				buys = append(buys, t)
				cash -= t.TotalValue
			}
		}

		newHoldings[security] = numShares
	}

	p.Transactions = append(p.Transactions, sells...)
	p.Transactions = append(p.Transactions, buys...)
	if cash > 1.0e-05 {
		newHoldings[data.CashSecurity] = cash
	}
	pm.holdings = newHoldings

	p.CurrentHoldings = buildHoldingsArray(date, newHoldings)

	return nil
}

// TargetPortfolio invests the portfolio in the ratios specified by the dataframe `target`.
//
//	`target` must have a column named `common.DateIdx` (DATE) and either a string column
//	or MixedAsset column of map[data.Security]float64 where the keys are the tickers and values are
//	the percentages of portfolio to hold
func (pm *Model) TargetPortfolio(ctx context.Context, target *dataframe.DataFrame) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "TargetPortfolio")
	defer span.End()

	p := pm.Portfolio

	log.Info().Msg("building target portfolio")
	log.Info().Int("CacheSizeMB", pm.dataProxy.HashSize()/(1024.0*1024.0)).Msg("EOD price cache size")
	if target.NRows() == 0 {
		log.Warn().Stack().Msg("target rows = 0; nothing to do!")
		return nil
	}

	timeIdx, err := target.NameToColumn(common.DateIdx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		log.Warn().Stack().Msg("could not find date index in data frame")
		return err
	}

	timeSeries := target.Series[timeIdx]

	// Set time range of portfolio
	p.EndDate = timeSeries.Value(timeSeries.NRows() - 1).(time.Time)
	now := time.Now()
	if now.Before(p.EndDate) {
		p.EndDate = now
	}

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "StartDate",
			Value: attribute.StringValue(p.StartDate.Format("2006-01-02")),
		},
		attribute.KeyValue{
			Key:   "EndDate",
			Value: attribute.StringValue(p.EndDate.Format("2006-01-02")),
		},
	)

	// Adjust first transaction to the target portfolio's first date if
	// there are no other transactions in the portfolio
	if len(p.Transactions) == 1 {
		p.StartDate = timeSeries.Value(0).(time.Time)
		p.Transactions[0].Date = p.StartDate
	}

	tickerSeriesIdx, err := target.NameToColumn(common.TickerName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		log.Warn().Stack().Err(err).Str("FieldName", common.TickerName).Msg("target portfolio does not have required field")
		return ErrNoTickerColumn
	}

	// check series type
	isSingleAsset := false
	series := target.Series[tickerSeriesIdx]
	if series.Type() == "string" {
		isSingleAsset = true
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

		var rebalance map[data.Security]float64
		if isSingleAsset {
			strSymbol := symbol.(string)
			security, err := data.SecurityFromFigi(strSymbol)
			if err != nil {
				log.Error().Err(err).Str("CompositeFigi", strSymbol).Msg("security not found")
				return err
			}
			rebalance = map[data.Security]float64{}
			rebalance[*security] = 1.0
		} else {
			rebalance = symbol.(map[data.Security]float64)
		}

		err = pm.RebalanceTo(ctx, date, rebalance, justification)
		if err != nil {
			return err
		}
	}
	return nil
}

// FillCorporateActions finds any corporate actions and creates transactions for them. The
// search occurs from the date of the last transaction to `through`
func (pm *Model) FillCorporateActions(ctx context.Context, through time.Time) error {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "FillCorporateActions")
	defer span.End()

	p := pm.Portfolio
	// nothing to do if there are no transactions
	n := len(p.Transactions)
	if n == 0 {
		return nil
	}

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Logger()

	dt := p.Transactions[n-1].Date
	tz := common.GetTimezone()

	// remove time and move to midnight the next day... this
	// ensures that the search is inclusive of through
	through = time.Date(through.Year(), through.Month(), through.Day(), 0, 0, 0, 0, tz)
	through = through.AddDate(0, 0, 1)
	from := time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
	if from.After(through) {
		subLog.Warn().Stack().Time("Through", through).Time("Start", from).Msg("start date occurs after through date")
		return ErrTimeInverted
	}

	subLog.Debug().Time("From", from).Time("Through", through).Msg("evaluating corporate actions")

	// Load split & dividend history
	cnt := 0
	for k := range pm.holdings {
		if k.Ticker != data.CashAsset {
			cnt++
		}
	}

	if cnt == 0 {
		return nil // nothing to do
	}

	addTransactions := make([]*Transaction, 0, 10)

	myDividends := pm.dataProxy.GetDividends()
	mySplits := pm.dataProxy.GetSplits()

	// for each holding check if there are splits
	for k := range pm.holdings {
		if k.Ticker == data.CashAsset {
			continue
		}

		// do dividends
		if divs, ok := myDividends[k]; ok {
			for _, d := range divs {
				date := d.Date
				if date.After(from) && date.Before(through) {
					// it's in range
					dividend := d.Value
					nShares := pm.holdings[k]
					totalValue := nShares * dividend
					// there is a dividend, record it
					pm.AddActivity(date, fmt.Sprintf("%s paid a $%.2f/share dividend", k, dividend), []string{"dividend"})
					trxID, _ := uuid.New().MarshalBinary()
					t := Transaction{
						ID:            trxID,
						Date:          date,
						Ticker:        k.Ticker,
						CompositeFIGI: k.CompositeFigi,
						Kind:          DividendTransaction,
						PricePerShare: 1.0,
						TotalValue:    totalValue,
						Justification: nil,
					}
					// update cash position in holdings
					pm.holdings[data.CashSecurity] += nShares * dividend
					if err := computeTransactionSourceID(&t); err != nil {
						log.Error().Stack().Err(err).Msg("failed to compute transaction source ID")
					}
					addTransactions = append(addTransactions, &t)
				}
			}
		}

		// do splits

		if splits, ok := mySplits[k]; ok {
			for _, s := range splits {
				date := s.Date
				if date.After(from) && date.Before(through) {
					// it's in range
					splitFactor := s.Value
					nShares := pm.holdings[k]
					shares := splitFactor * nShares
					// there is a split, record it
					pm.AddActivity(date, fmt.Sprintf("shares of %s split by a factor of %.2f", k, splitFactor), []string{"split"})
					trxID, _ := uuid.New().MarshalBinary()
					t := Transaction{
						ID:            trxID,
						Date:          date,
						Ticker:        k.Ticker,
						CompositeFIGI: k.CompositeFigi,
						Kind:          SplitTransaction,
						PricePerShare: 0.0,
						Shares:        shares,
						TotalValue:    0,
						Justification: nil,
					}
					if err := computeTransactionSourceID(&t); err != nil {
						log.Error().Stack().Err(err).Msg("could not compute transaction source ID")
					}
					addTransactions = append(addTransactions, &t)

					// update holdings
					pm.holdings[k] = shares
				}
			}
		}
	}

	sort.SliceStable(addTransactions, func(i, j int) bool { return addTransactions[i].Date.Before(addTransactions[j].Date) })
	p.Transactions = append(p.Transactions, addTransactions...)

	return nil
}

// BuildPredictedHoldings creates a PortfolioHoldingItem from a date, target map, and justification map
func BuildPredictedHoldings(tradeDate time.Time, target map[data.Security]float64, justificationMap map[string]float64) *PortfolioHoldingItem {
	holdings := make([]*ReportableHolding, 0, len(target))
	for k, v := range target {
		h := ReportableHolding{
			Ticker:           k.Ticker,
			CompositeFIGI:    k.CompositeFigi,
			Shares:           v * 100.0,
			PercentPortfolio: float32(v),
		}
		holdings = append(holdings, &h)
	}
	justification := make([]*Justification, 0, len(justificationMap))
	for k, v := range justificationMap {
		j := Justification{
			Key:   k,
			Value: v,
		}
		justification = append(justification, &j)
	}
	return &PortfolioHoldingItem{
		Time:          tradeDate,
		Holdings:      holdings,
		Justification: justification,
		Predicted:     true,
	}
}

// UpdateTransactions calculates new transactions based on the portfolio strategy
// from the portfolio end date to `through`
func (pm *Model) UpdateTransactions(ctx context.Context, through time.Time) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "portfolio.UpdateTransactions")
	defer span.End()

	p := pm.Portfolio
	pm.dataProxy.Begin = p.EndDate.AddDate(0, -6, 1)
	startDate := p.EndDate.AddDate(0, 0, 1)
	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Logger()

	if through.Before(pm.dataProxy.Begin) {
		span.SetStatus(codes.Error, "cannot update portfolio due to dates being out of order")
		subLog.Error().Stack().
			Time("Begin", pm.dataProxy.Begin).
			Time("End", through).
			Msg("cannot update portfolio dates are out of order")
		return ErrInvalidDateRange
	}

	pm.dataProxy.End = through
	pm.dataProxy.Frequency = data.FrequencyDaily

	arguments := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(p.StrategyArguments), &arguments); err != nil {
		subLog.Error().Stack().Err(err).Msg("could not unmarshal strategy arguments")
		return err
	}

	strategy, ok := strategies.StrategyMap[p.StrategyShortcode]
	if !ok {
		span.SetStatus(codes.Error, "strategy not found")
		subLog.Error().Stack().Msg("portfolio strategy not found")
		return ErrStrategyNotFound
	}

	stratObject, err := strategy.Factory(arguments)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to initialize portfolio strategy")
		subLog.Error().Err(err).Stack().Msg("failed to initialize portfolio strategy")
		return err
	}

	subLog.Info().Msg("computing portfolio strategy over date period")
	targetPortfolio, predictedAssets, err := stratObject.Compute(ctx, pm.dataProxy)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to execute portfolio strategy")
		subLog.Error().Err(err).Stack().Msg("failed to execute portfolio strategy")
		return err
	}

	// thin the targetPortfolio to only include info on or after the startDate
	_, err = dfextras.TimeTrim(ctx, targetPortfolio, startDate, through, true)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not trim target portfolio to date range")
		subLog.Error().Err(err).Stack().
			Time("StartDate", startDate).
			Time("EndDate", through).
			Msg("could not trim target portfolio to date range")
	}

	err = pm.TargetPortfolio(ctx, targetPortfolio)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to apply target porfolio")
		subLog.Error().Err(err).Msg("failed to apply target portfolio")
		return err
	}

	// make sure any corporate actions are applied
	if err := pm.FillCorporateActions(ctx, through); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not update corporate actions")
		subLog.Error().Err(err).Msg("could not update corporate actions")
		return err
	}

	pm.Portfolio.PredictedAssets = BuildPredictedHoldings(predictedAssets.TradeDate, predictedAssets.Target, predictedAssets.Justification)
	p.EndDate = through

	return nil
}

// DATABASE FUNCTIONS

// LOAD

func (pm *Model) LoadTransactionsFromDB() error {
	p := pm.Portfolio
	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.Error().Stack().Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("UserID", p.UserID).
			Msg("unable to get database transaction object")
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
		price_per_share::double precision,
		num_shares::double precision,
		source,
		encode(source_id, 'hex'),
		tags,
		tax_type,
		ticker,
		total_value
	FROM portfolio_transactions
	WHERE portfolio_id=$1 AND user_id=$2
	ORDER BY sequence_num`
	rows, err := trx.Query(context.Background(), transactionSQL, p.ID, p.UserID)
	if err != nil {
		log.Error().Stack().Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("UserID", p.UserID).
			Str("Query", transactionSQL).
			Msg("could not load portfolio transactions from database")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	transactions := make([]*Transaction, 0, 1000)
	for rows.Next() {
		t := Transaction{}

		var compositeFIGI pgtype.Text
		var memo pgtype.Text
		var taxDisposition pgtype.Text

		var pricePerShare pgtype.Float8
		var shares pgtype.Float8

		var sourceID pgtype.Text

		err := rows.Scan(&t.ID, &t.Date, &t.Cleared, &t.Commission, &compositeFIGI,
			&t.Justification, &t.Kind, &memo, &pricePerShare, &shares, &t.Source,
			&sourceID, &t.Tags, &taxDisposition, &t.Ticker, &t.TotalValue)
		if err != nil {
			log.Warn().Stack().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("UserID", p.UserID).
				Str("Query", transactionSQL).
				Msg("failed scanning row into transaction fields")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return err
		}

		if compositeFIGI.Status == pgtype.Present {
			t.CompositeFIGI = compositeFIGI.String
		}
		if memo.Status == pgtype.Present {
			t.Memo = memo.String
		}
		if taxDisposition.Status == pgtype.Present {
			t.TaxDisposition = taxDisposition.String
		}
		if pricePerShare.Status == pgtype.Present {
			t.PricePerShare = pricePerShare.Float
		}
		if shares.Status == pgtype.Present {
			t.Shares = shares.Float
		}
		if sourceID.Status == pgtype.Present {
			t.SourceID = sourceID.String
		}

		transactions = append(transactions, &t)
	}
	p.Transactions = transactions

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction to database")
	}

	return nil
}

func LoadFromDB(portfolioIDs []string, userID string, dataProxy *data.Manager) ([]*Model, error) {
	subLog := log.With().Str("UserID", userID).Strs("PortfolioIDs", portfolioIDs).Logger()
	if userID == "" {
		subLog.Error().Stack().Msg("userID cannot be an empty string")
		return nil, ErrEmptyUserID
	}
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Stack().Msg("failed to create database transaction for user")
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
			portfolios
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
			portfolios
		WHERE
			user_id=$1`
		rows, err = trx.Query(context.Background(), portfolioSQL, userID)
	}

	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not load portfolio from database")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return nil, err
	}

	sz := len(portfolioIDs)
	if sz == 0 {
		sz = 10
	}
	resultSet := make([]*Model, 0, sz)

	for rows.Next() {
		p := &Portfolio{
			UserID:       userID,
			Transactions: []*Transaction{},
		}

		pm := &Model{
			Portfolio:      p,
			dataProxy:      dataProxy,
			justifications: make(map[string][]*Justification),
		}

		tz := common.GetTimezone()
		tmpHoldings := make(map[string]float64)
		err = rows.Scan(&p.ID, &p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &tmpHoldings, &p.Notifications, &p.Benchmark)

		for k, v := range tmpHoldings {
			security, err := data.SecurityFromFigi(k)
			if err != nil {
				subLog.Warn().Str("CompositeFigi", k).Msg("portfolio holds inactive security")
				continue
			}
			pm.holdings[*security] = v
		}

		p.StartDate = p.StartDate.In(tz)
		p.EndDate = p.EndDate.In(tz)
		if err != nil {
			subLog.Warn().Stack().Err(err).Msg("could not load portfolio from database")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		p.CurrentHoldings = buildHoldingsArray(time.Now(), pm.holdings)
		pm.dataProxy.Begin = p.StartDate
		pm.dataProxy.End = time.Now()

		resultSet = append(resultSet, pm)
	}

	if len(resultSet) == 0 && len(portfolioIDs) != 0 {
		return nil, ErrPortfolioNotFound
	}

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction to database")
	}
	return resultSet, nil
}

func (pm *Model) AddActivity(date time.Time, msg string, tags []string) {
	if pm.activities == nil {
		pm.activities = make([]*Activity, 0, 5)
	}

	pm.activities = append(pm.activities, &Activity{
		Date: date,
		Msg:  msg,
		Tags: tags,
	})
}

func (pm *Model) SaveActivities() error {
	p := pm.Portfolio
	userID := p.UserID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Logger()

	if pm.activities == nil {
		pm.activities = make([]*Activity, 0, 5)
	}

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	for _, activity := range pm.activities {

		sql := `INSERT INTO activity ("user_id", "portfolio_id", "event_date", "activity", "tags") VALUES ($1, $2, $3, $4, $5)`
		if _, err := trx.Exec(context.Background(), sql, userID, p.ID, activity.Date, activity.Msg, activity.Tags); err != nil {
			subLog.Error().Err(err).Msg("could not create activity")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return err
		}
	}

	err = trx.Commit(context.Background())
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return err
	}

	return nil
}

func (pm *Model) SetStatus(msg string) error {
	p := pm.Portfolio
	userID := p.UserID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Str("Status", msg).Logger()

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	sql := `UPDATE portfolios SET status=$1 WHERE id=$2`
	if _, err := trx.Exec(context.Background(), sql, msg, p.ID); err != nil {
		subLog.Error().Err(err).Msg("could not update portfolio status")
	}

	err = trx.Commit(context.Background())
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

// Save portfolio to database along with all transaction data
func (pm *Model) Save(userID string) error {
	p := pm.Portfolio
	p.UserID = userID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Logger()

	if err := pm.SaveActivities(); err != nil {
		subLog.Error().Err(err).Msg("could not save activities")
	}

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	err = pm.SaveWithTransaction(trx, userID, false)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to create portfolio transactions")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

func (pm *Model) SaveWithTransaction(trx pgx.Tx, userID string, permanent bool) error {
	temporary := !permanent
	p := pm.Portfolio
	subLog := log.With().Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Logger()
	portfolioSQL := `
	INSERT INTO portfolios (
		"id",
		"name",
		"strategy_shortcode",
		"arguments",
		"benchmark",
		"start_date",
		"end_date",
		"holdings",
		"notifications",
		"temporary",
		"user_id",
		"predicted_bytes"
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
		$12
	) ON CONFLICT ON CONSTRAINT portfolios_pkey
	DO UPDATE SET
		name=$2,
		strategy_shortcode=$3,
		arguments=$4,
		benchmark=$5,
		start_date=$6,
		end_date=$7,
		holdings=$8,
		notifications=$9,
		predicted_bytes=$12`
	marshableHoldings := make(map[string]float64, len(pm.holdings))
	for k, v := range pm.holdings {
		marshableHoldings[k.CompositeFigi] = v
	}
	holdings, err := json.Marshal(marshableHoldings)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to marshal holdings")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}
	predictedBytes, err := p.PredictedAssets.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("Could not marshal predicted bytes")
	}
	_, err = trx.Exec(context.Background(), portfolioSQL, p.ID, p.Name, p.StrategyShortcode,
		p.StrategyArguments, p.Benchmark, p.StartDate, p.EndDate, holdings, p.Notifications, temporary, userID, predictedBytes)
	if err != nil {
		subLog.Error().Stack().Err(err).Str("Query", portfolioSQL).Msg("failed to save portfolio")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return pm.saveTransactions(trx, userID)
}

func (pm *Model) saveTransactions(trx pgx.Tx, userID string) error {
	p := pm.Portfolio

	log.Info().
		Str("PortfolioID", hex.EncodeToString(p.ID)).
		Str("PortfolioName", p.Name).
		Str("Strategy", p.StrategyShortcode).
		Str("StrategyArguments", p.StrategyArguments).
		Int("NumTransactions", len(p.Transactions)).
		Msg("Saving portfolio transactions")

	transactionSQL := `
	INSERT INTO portfolio_transactions (
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
		decode($13, 'hex'),
		$14,
		$15,
		$16,
		$17,
		$18,
		$19
	) ON CONFLICT ON CONSTRAINT portfolio_transactions_pkey
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
		source_id=decode($13, 'hex'),
		tags=$14,
		tax_type=$15,
		ticker=$16,
		total_value=$17,
		sequence_num=$18`

	now := time.Now()
	for idx, t := range p.Transactions {
		if t.Date.After(now) {
			log.Info().
				Str("TransactionID", hex.EncodeToString(t.ID)).
				Time("TransactionDate", t.Date).
				Time("Now", now).
				Msg("not saving transaction because it is in the future")
		}
		if t.TaxDisposition == "" {
			t.TaxDisposition = "TAXABLE"
		}
		var jsonJustification []byte
		if len(t.Justification) > 0 {
			var err error
			jsonJustification, err = json.Marshal(t.Justification)
			if err != nil {
				log.Error().Err(err).Msg("could not marshal to JSON the justification array")
			}
		}
		_, err := trx.Exec(context.Background(), transactionSQL,
			t.ID,              // 1
			p.ID,              // 2
			t.Kind,            // 3
			t.Cleared,         // 4
			t.Commission,      // 5
			t.CompositeFIGI,   // 6
			t.Date,            // 7
			jsonJustification, // 8
			t.Memo,            // 9
			t.PricePerShare,   // 10
			t.Shares,          // 11
			t.Source,          // 12
			t.SourceID,        // 13
			t.Tags,            // 14
			t.TaxDisposition,  // 15
			t.Ticker,          // 16
			t.TotalValue,      // 17
			idx,               // 18
			userID,            // 19
		)
		if err != nil {
			log.Warn().Stack().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("TransactionID", hex.EncodeToString(t.ID)).
				Str("Query", transactionSQL).
				Str("Kind", t.Kind).
				Bool("Cleared", t.Cleared).
				Float64("Commission", t.Commission).
				Str("CompositeFigi", t.CompositeFIGI).
				Time("Date", t.Date).
				Bytes("Justification", jsonJustification).
				Str("Memo", t.Memo).
				Float64("PricePerShare", t.PricePerShare).
				Float64("Shares", t.Shares).
				Str("Source", t.Source).
				Str("SourceID", t.SourceID).
				Strs("Tags", t.Tags).
				Str("TaxDisposition", t.TaxDisposition).
				Str("Ticker", t.Ticker).
				Float64("TotalValue", t.TotalValue).
				Int("Idx", idx).
				Str("UserID", userID).
				Msg("failed to save transaction")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return err
		}
	}

	return nil
}

// Private API

// computeTransactionSourceID calculates a 16-byte blake3 hash using the date, source,
//
//	composite figi, ticker, kind, price per share, shares, and total value
func computeTransactionSourceID(t *Transaction) error {
	h := blake3.New()

	// Date as UTC unix timestamp (second precision)
	// NOTE: casting to uint64 doesn't change the sign bit here
	d, err := t.Date.UTC().MarshalText()
	if err != nil {
		return err
	}

	if _, err := h.Write(d); err != nil {
		log.Error().Stack().Err(err).Msg("could not write date to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(t.Source)); err != nil {
		log.Error().Stack().Err(err).Msg("could not write source to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(t.CompositeFIGI)); err != nil {
		log.Error().Stack().Err(err).Msg("could not write composite figi to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(t.Ticker)); err != nil {
		log.Error().Stack().Err(err).Msg("could not write ticker to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(t.Kind)); err != nil {
		log.Error().Stack().Err(err).Msg("could not write kind to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(fmt.Sprintf("%.5f", t.PricePerShare))); err != nil {
		log.Error().Stack().Err(err).Msg("could not write price per share to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(fmt.Sprintf("%.5f", t.Shares))); err != nil {
		log.Error().Stack().Err(err).Msg("could not write shares to blake3 hasher")
		return err
	}

	if _, err := h.Write([]byte(fmt.Sprintf("%.5f", t.TotalValue))); err != nil {
		log.Error().Stack().Err(err).Msg("could not write total value to blake3 hasher")
		return err
	}

	digest := h.Digest()
	buf := make([]byte, 16)
	n, err := digest.Read(buf)
	if err != nil {
		return err
	}
	if n != 16 {
		return ErrGenerateHash
	}

	t.SourceID = hex.EncodeToString(buf)
	return nil
}
