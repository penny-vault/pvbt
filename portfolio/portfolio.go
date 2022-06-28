// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dfextras"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/strategies"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/rocketlaunchr/dataframe-go"
	"github.com/rs/zerolog/log"
	"github.com/zeebo/blake3"
)

var (
	ErrEmptyUserID       = errors.New("user id empty")
	ErrStrategyNotFound  = errors.New("strategy not found")
	ErrHoldings          = errors.New("holdings are out of sync, cannot rebalance portfolio")
	ErrInvalidSell       = errors.New("refusing to sell 0 shares - cannot rebalance portfolio; target allocation broken")
	ErrTimeInverted      = errors.New("start date occurs after through date")
	ErrPortfolioNotFound = errors.New("could not find portfolio ID in database")
	ErrGenerateHash      = errors.New("could not create a new hash")
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
	value          float64
	holdings       map[string]float64
	justifications map[string][]*Justification
}

type Period struct {
	Begin time.Time
	End   time.Time
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
		log.Warn().Err(err).Time("TransactionDate", startDate).Str("TransactionTicker", "$CASH").Str("TransactionType", DepositTransaction).Msg("couldn't compute SourceID for initial deposit")
	}
	p.Transactions = append(p.Transactions, &t)
	model.holdings = map[string]float64{
		"$CASH": initial,
	}

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
func (pm *PortfolioModel) RebalanceTo(ctx context.Context, date time.Time, target map[string]float64, justification []*Justification) error {
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
			log.Error().Time("Date", date).Time("LastTransactionDate", lastDate).Msg("cannot rebalance portfolio when date is before last transaction date")
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
		log.Error().Float64("TotalPercentAllocated", total).Time("Date", date).Msg("TotalPercentAllocated must equal 1.0")
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
			price, err := pm.dataProxy.Get(ctx, date, data.MetricClose, k)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "security price data not available")
				log.Warn().Str("Symbol", k).Time("Date", date).Float64("Price", price).Msg("security price data not available.")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}

			log.Debug().Time("Date", date).Str("Ticker", k).Float64("Price", price).Float64("Value", v*price).Msg("Retrieve price data")

			if !math.IsNaN(price) {
				securityValue += v * price
			}
			priceMap[k] = price
		}
	}

	// get any prices that we haven't already loaded
	for k := range target {
		if _, ok := priceMap[k]; !ok {
			price, err := pm.dataProxy.Get(ctx, date, data.MetricClose, k)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "security price data not available")
				log.Warn().Err(err).Str("Symbol", k).Time("Date", date).Msg("security price data not available")
				return fmt.Errorf("security %s price data not available for date %s", k, date.String())
			}
			priceMap[k] = price
		}
	}

	investable := cash + securityValue
	pm.value = investable

	// process all targets
	sells := make([]*Transaction, 0, 10)
	buys := make([]*Transaction, 0, 10)

	// sell any holdings that we no longer want
	for k, v := range pm.holdings {
		if k == "$CASH" {
			continue
		}

		if v <= 1.0e-5 {
			log.Warn().Str("Ticker", k).Str("Kind", "SellTransaction").Float64("Shares", v).Msg("holdings are out of sync")
			return ErrHoldings
		}

		if _, ok := target[k]; !ok {
			price := priceMap[k]
			if math.IsNaN(priceMap[k]) {
				log.Warn().Float64("Cash", cash).Float64("Shares", v).Str("Ticker", k).Msg("price is not known - writing off asset")
				price = 0.0
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
				log.Warn().Err(err).Time("TransactionDate", date).Str("TransactionTicker", k).Str("TransactionType", SellTransaction).Msg("couldn't compute SourceID for transaction")
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
					log.Error().Str("Ticker", k).Msg("No known price for asset; Cannot sell partial")
					price = 0.0
					newHoldings[k] = 0.0
				}

				toSellDollars := currentDollars - targetDollars
				toSellShares := toSellDollars / price
				if toSellDollars <= 1.0e-5 {
					log.Error().
						Str("Ticker", k).
						Str("Kind", "SellTransaction").
						Float64("Shares", toSellShares).
						Time("Date", date).
						Float64("Price", price).
						Float64("CurrentDollars", currentDollars).
						Float64("TargetDollars", targetDollars).
						Msg("refusing to sell 0 shares")
					return ErrInvalidSell
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
					log.Error().
						Err(err).
						Time("TransactionDate", date).
						Str("TransactionTicker", k).
						Str("TransactionType", SellTransaction).
						Msg("couldn't compute SourceID for transaction")
				}

				sells = append(sells, &t)
			} else if targetDollars > currentDollars {
				// Need to buy to target amount
				toBuyDollars := targetDollars - currentDollars
				toBuyShares := toBuyDollars / price

				subLog := log.With().Str("Ticker", k).Str("Kind", "BuyTransaction").Float64("Shares", v).Float64("TotalValue", toBuyDollars).Time("Date", date).Logger()
				if toBuyShares <= 1.0e-5 {
					subLog.Warn().Msg("refusing to buy 0 shares")
				} else if math.IsNaN(price) {
					subLog.Warn().Msg("refusing to buy shares of asset with unknown price")
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
						subLog.Warn().Msg("toBuyDollars is NaN")
					} else {
						cash -= toBuyDollars
					}

					subLog.Debug().Msg("Buy additional shares")

					err := computeTransactionSourceID(&t)
					if err != nil {
						subLog.Warn().Err(err).Msg("couldn't compute SourceID for transaction")
					}

					buys = append(buys, &t)
				}
			}
		} else {
			// this is a new position
			value := investable * v
			price := priceMap[k]
			shares := value / price

			subLog := log.With().Str("Ticker", k).Str("Kind", "BuyTransaction").Float64("Shares", v).Float64("TotalValue", value).Time("Date", date).Logger()
			if shares <= 1.0e-5 {
				subLog.Warn().Msg("refusing to buy 0 shares")
			} else if math.IsNaN(price) {
				subLog.Warn().Msg("refusing to buy shares of asset with unknown price")
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
				subLog.Debug().Msg("buy new holding")

				err := computeTransactionSourceID(&t)
				if err != nil {
					subLog.Warn().Msg("couldn't compute SourceID for transaction")
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

// BuildAssetPlan scans the dataframe and builds a map on asset of the time frame the asset is held in the portfolio
func BuildAssetPlan(ctx context.Context, target *dataframe.DataFrame) map[string]*Period {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "BuildAssetPlan")
	defer span.End()

	plan := map[string]*Period{}

	tickerSeriesIdx, err := target.NameToColumn(common.TickerName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		log.Warn().Err(err).Msg("invest target portfolio failed")
		return plan
	}

	// check series type
	isSingleAsset := false
	series := target.Series[tickerSeriesIdx]
	if series.Type() == "string" {
		isSingleAsset = true
	}

	// Get price data
	iterator := target.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	for {
		row, val, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}

		date := val[common.DateIdx].(time.Time)

		if isSingleAsset {
			ticker := val[common.TickerName].(string)
			period, ok := plan[ticker]
			if !ok {
				period = &Period{
					Begin: date,
				}
				plan[ticker] = period
			}
			if period.End.Before(date) {
				period.End = date
			}
		} else {
			// it's multi-asset which means a map of tickers
			assetMap := val[common.TickerName].(map[string]float64)
			for ticker, _ := range assetMap {
				period, ok := plan[ticker]
				if !ok {
					period = &Period{
						Begin: date,
					}
					plan[ticker] = period
				}
				if period.End.Before(date) {
					period.End = date
				}
			}
		}
	}

	return plan
}

// TargetPortfolio invests the portfolio in the ratios specified by the dataframe `target`.
//   `target` must have a column named `common.DateIdx` (DATE) and either a string column
//   or MixedAsset column of map[string]float64 where the keys are the tickers and values are
//   the percentages of portfolio to hold
func (pm *PortfolioModel) TargetPortfolio(ctx context.Context, target *dataframe.DataFrame) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "TargetPortfolio")
	defer span.End()

	p := pm.Portfolio

	log.Info().Msg("building target portfolio")
	log.Info().Int("CacheSizeMB", pm.dataProxy.HashSize()/(1024.0*1024.0)).Msg("EOD price cache size")
	if target.NRows() == 0 {
		log.Warn().Msg("target rows = 0; nothing to do!")
		return nil
	}

	timeIdx, err := target.NameToColumn(common.DateIdx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		log.Warn().Msg("could not find date index in data frame")
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
		log.Warn().Err(err).Str("FieldName", common.TickerName).Msg("target portfolio does not have required field")
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

		err = pm.RebalanceTo(ctx, date, rebalance, justification)
		if err != nil {
			return err
		}
	}
	return nil
}

// FillCorporateActions finds any corporate actions and creates transactions for them. The
// search occurs from the date of the last transaction to `through`
func (pm *PortfolioModel) FillCorporateActions(ctx context.Context, through time.Time) error {
	_, span := otel.Tracer(opentelemetry.Name).Start(ctx, "FillCorporateActions")
	defer span.End()

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

	log.Debug().Time("Through", through).Msg("evaluating corporate actions")

	// remove time and move to midnight the next day... this
	// ensures that the search is inclusive of through
	through = time.Date(through.Year(), through.Month(), through.Day(), 0, 0, 0, 0, tz)
	through = through.AddDate(0, 0, 1)
	from := time.Date(dt.Year(), dt.Month(), dt.Day(), 16, 0, 0, 0, tz)
	if from.After(through) {
		log.Warn().Time("Through", through).Time("Start", from).Msg("start date occurs after through date")
		return ErrTimeInverted
	}

	// Load split & dividend history
	cnt := 0
	for k := range pm.holdings {
		if k != "$CASH" {
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
		if k == "$CASH" {
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
	}

	sort.SliceStable(addTransactions, func(i, j int) bool { return addTransactions[i].Date.Before(addTransactions[j].Date) })
	p.Transactions = append(p.Transactions, addTransactions...)

	return nil
}

// BuildPredictedHoldings creates a PortfolioHoldingItem from a date, target map, and justification map
func BuildPredictedHoldings(tradeDate time.Time, target map[string]float64, justificationMap map[string]float64) *PortfolioHoldingItem {
	holdings := make([]*ReportableHolding, 0, len(target))
	for k, v := range target {
		h := ReportableHolding{
			Ticker:           k,
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
func (pm *PortfolioModel) UpdateTransactions(ctx context.Context, through time.Time) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "portfolio.UpdateTransactions")
	defer span.End()

	p := pm.Portfolio
	pm.dataProxy.Begin = p.EndDate.AddDate(0, -6, 1)
	startDate := p.EndDate.AddDate(0, 0, 1)

	if through.Before(pm.dataProxy.Begin) {
		span.SetStatus(codes.Error, "cannot update portfolio due to dates being out of order")
		log.Error().
			Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).
			Time("Begin", pm.dataProxy.Begin).
			Time("End", through).
			Msg("cannot update portfolio dates are out of order")
		return fmt.Errorf("cannot update portfolio, through must be greater than %s", pm.dataProxy.Begin)
	}

	pm.dataProxy.End = through
	pm.dataProxy.Frequency = data.FrequencyDaily

	arguments := make(map[string]json.RawMessage)
	json.Unmarshal([]byte(p.StrategyArguments), &arguments)

	strategy, ok := strategies.StrategyMap[p.StrategyShortcode]
	if !ok {
		span.SetStatus(codes.Error, "strategy not found")
		log.Error().
			Str("Portfolio", hex.EncodeToString(p.ID)).
			Str("Strategy", p.StrategyShortcode).
			Msg("portfolio strategy not found")
		return ErrStrategyNotFound
	}

	stratObject, err := strategy.Factory(arguments)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to initialize portfolio strategy")
		log.Error().
			Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("Strategy", p.StrategyShortcode).
			Msg("failed to initialize portfolio strategy")
		return err
	}

	targetPortfolio, predictedAssets, err := stratObject.Compute(ctx, pm.dataProxy)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to execute portfolio strategy")
		log.Error().
			Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("Strategy", p.StrategyShortcode).
			Msg("failed to run portfolio strategy")
		return err
	}

	// thin the targetPortfolio to only include info on or after the startDate
	_, err = dfextras.TimeTrim(ctx, targetPortfolio, startDate, through, true)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not trim target portfolio to date range")
		log.Error().Err(err).
			Time("StartDate", startDate).
			Time("EndDate", through).
			Msg("could not trim target portfolio to date range")
	}

	err = pm.TargetPortfolio(ctx, targetPortfolio)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to apply target porfolio")
		log.Error().Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("Strategy", p.StrategyShortcode).
			Msg("failed to apply target portfolio")
		return err
	}

	pm.Portfolio.PredictedAssets = BuildPredictedHoldings(predictedAssets.TradeDate, predictedAssets.Target, predictedAssets.Justification)
	p.EndDate = through

	return nil
}

// DATABASE FUNCTIONS

// LOAD

func (pm *PortfolioModel) LoadTransactionsFromDB() error {
	p := pm.Portfolio
	trx, err := database.TrxForUser(p.UserID)
	if err != nil {
		log.Error().Err(err).
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
		source_id,
		tags,
		tax_type,
		ticker,
		total_value
	FROM portfolio_transactions
	WHERE portfolio_id=$1 AND user_id=$2
	ORDER BY sequence_num`
	rows, err := trx.Query(context.Background(), transactionSQL, p.ID, p.UserID)
	if err != nil {
		log.Error().Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("UserID", p.UserID).
			Str("Query", transactionSQL).
			Msg("could not load portfolio transactions from database")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
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

		var sourceID pgtype.Bytea

		err := rows.Scan(&t.ID, &t.Date, &t.Cleared, &t.Commission, &compositeFIGI,
			&t.Justification, &t.Kind, &memo, &pricePerShare, &shares, &t.Source,
			sourceID, &t.Tags, &taxDisposition, &t.Ticker, &t.TotalValue)
		if err != nil {
			log.Warn().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("UserID", p.UserID).
				Str("Query", transactionSQL).
				Msg("failed scanning row into transaction fields")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
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
			t.SourceID = string(sourceID.Bytes)
		}

		transactions = append(transactions, &t)
	}
	p.Transactions = transactions
	return nil
}

func LoadFromDB(portfolioIDs []string, userID string, dataProxy *data.Manager) ([]*PortfolioModel, error) {
	subLog := log.With().Str("UserID", userID).Strs("PortfolioIDs", portfolioIDs).Logger()
	if userID == "" {
		subLog.Error().Msg("userID cannot be an empty string")
		return nil, ErrEmptyUserID
	}
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Msg("failed to create database transaction for user")
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
		subLog.Warn().Err(err).Msg("could not load portfolio from database")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

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
		}

		err = rows.Scan(&p.ID, &p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &pm.holdings, &p.Notifications, &p.Benchmark)
		tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
		p.StartDate = p.StartDate.In(tz)
		p.EndDate = p.EndDate.In(tz)
		if err != nil {
			subLog.Warn().Err(err).Msg("could not load portfolio from database")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
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

	trx.Commit(context.Background())
	return resultSet, nil
}

// Save portfolio to database along with all transaction data
func (pm *PortfolioModel) Save(userID string) error {
	p := pm.Portfolio
	p.UserID = userID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Logger()

	// Save to database
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	err = pm.SaveWithTransaction(trx, userID, false)
	if err != nil {
		subLog.Error().Err(err).Msg("failed to create portfolio transactions")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		subLog.Error().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

func (pm *PortfolioModel) SaveWithTransaction(trx pgx.Tx, userID string, permanent bool) error {
	temporary := !permanent
	p := pm.Portfolio
	subLog := log.With().Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Logger()
	portfolioSQL := `
	INSERT INTO portfolios (
		"id",
		"name",
		"strategy_shortcode",
		"arguments",
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
		$11
	) ON CONFLICT ON CONSTRAINT portfolios_pkey
	DO UPDATE SET
		name=$2,
		strategy_shortcode=$3,
		arguments=$4,
		start_date=$5,
		end_date=$6,
		holdings=$7,
		notifications=$8,
		predicted_bytes=$11`
	holdings, err := json.Marshal(pm.holdings)
	if err != nil {
		subLog.Error().Err(err).Msg("failed to marshal holdings")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}
	predictedBytes, err := p.PredictedAssets.MarshalBinary()
	if err != nil {
		subLog.Error().Err(err).Msg("Could not marshal predicted bytes")
	}
	_, err = trx.Exec(context.Background(), portfolioSQL, p.ID, p.Name, p.StrategyShortcode,
		p.StrategyArguments, p.StartDate, p.EndDate, holdings, p.Notifications, temporary, userID, predictedBytes)
	if err != nil {
		subLog.Error().Err(err).Str("Query", portfolioSQL).Msg("failed to save portfolio")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return pm.saveTransactions(trx, userID)
}

func (pm *PortfolioModel) saveTransactions(trx pgx.Tx, userID string) error {
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
		$13,
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
		source_id=$13,
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
			log.Warn().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("TransactionID", hex.EncodeToString(t.ID)).
				Str("Query", transactionSQL).
				Msg("failed to save portfolio")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}

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
		return ErrGenerateHash
	}

	t.SourceID = hex.EncodeToString(buf)
	return nil
}
