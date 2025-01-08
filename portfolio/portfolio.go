// Copyright 2021-2025
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
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/olekukonko/tablewriter"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
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
	InterestTransaction = "INTEREST"
	DividendTransaction = "DIVIDEND"
	SplitTransaction    = "SPLIT"
	DepositTransaction  = "DEPOSIT"
	WithdrawTransaction = "WITHDRAW"
)

const (
	SplitFactor = "SplitFactor"
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
func NewPortfolio(name string, startDate time.Time, initial float64) *Model {
	id, _ := uuid.New().MarshalBinary()
	p := Portfolio{
		ID:           id,
		Name:         name,
		AccountType:  Taxable,
		Benchmark:    "BBG000BHTMY2",
		Transactions: []*Transaction{},
		TaxLots: &TaxLotInfo{
			AsOf:   time.Time{},
			Method: TaxLotFIFOMethod,
			Items:  make([]*TaxLot, 0, 5),
		},
		StartDate: startDate,
	}

	model := Model{
		Portfolio:      &p,
		justifications: make(map[string][]*Justification),
	}

	// Create initial deposit
	trxID, _ := uuid.New().MarshalBinary()
	t := Transaction{
		ID:            trxID,
		Date:          startDate,
		Ticker:        data.CashSecurity.Ticker,
		CompositeFIGI: data.CashSecurity.CompositeFigi,
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

func (pm *Model) processDividends(dividends dataframe.Map) []*Transaction {
	addTransactions := make([]*Transaction, 0, len(dividends))

	// for each holding check if there are splits
	for k, v := range dividends {
		securityMetric := data.NewSecurityMetricFromString(k)
		if securityMetric.SecurityObject.Ticker == data.CashAsset {
			continue
		}

		for idx, date := range v.Dates {
			// it's in range
			dividend := v.Vals[0][idx]
			nShares := pm.holdings[securityMetric.SecurityObject]
			totalValue := nShares * dividend
			// there is a dividend, record it
			pm.AddActivity(date, fmt.Sprintf("%s paid a $%.2f/share dividend", k, dividend), []string{"dividend"})
			justification := []*Justification{{Key: "Dividend", Value: dividend}}
			trx, err := createTransaction(date, &securityMetric.SecurityObject, DividendTransaction, 1.0,
				totalValue, pm.Portfolio.AccountType, justification)
			if err != nil {
				log.Error().Stack().Err(err).Msg("failed to create transaction")
			}
			trx.Memo = fmt.Sprintf("$%.2f/share dividend", dividend)

			// update cash position in holdings
			pm.holdings[data.CashSecurity] += totalValue
			if pm.Portfolio.TaxLots.CheckIfDividendIsQualified(trx) {
				trx.TaxDisposition = QualifiedDividend
			} else {
				trx.TaxDisposition = NonQualifiedDividend
			}
			addTransactions = append(addTransactions, trx)
		}
	}

	return addTransactions
}

func (pm *Model) processSplits(splits dataframe.Map) []*Transaction {
	addTransactions := make([]*Transaction, 0, len(splits))

	// for each holding check if there are splits
	for k, v := range splits {
		securityMetric := data.NewSecurityMetricFromString(k)
		if securityMetric.SecurityObject.Ticker == data.CashAsset {
			continue
		}

		for idx, date := range v.Dates {
			// it's in range
			splitFactor := v.Vals[0][idx]
			nShares := pm.holdings[securityMetric.SecurityObject]
			shares := splitFactor * nShares
			// there is a split, record it
			pm.AddActivity(date, fmt.Sprintf("shares of %s split by a factor of %.2f", k, splitFactor), []string{"split"})
			justification := []*Justification{{Key: SplitFactor, Value: splitFactor}}
			trx, err := createTransaction(date, &securityMetric.SecurityObject, SplitTransaction, 0.0, shares,
				pm.Portfolio.AccountType, justification)
			trx.Memo = fmt.Sprintf("split by a factor of %.2f", splitFactor)
			pm.Portfolio.TaxLots.AdjustForSplit(&securityMetric.SecurityObject, splitFactor)
			if err != nil {
				log.Error().Stack().Err(err).Msg("failed to create transaction")
			}
			addTransactions = append(addTransactions, trx)
			// update holdings
			pm.holdings[securityMetric.SecurityObject] = shares
		}
	}

	return addTransactions
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
	// check what date range to consider
	lastTrxDate := p.Transactions[n-1].Date
	from := time.Date(lastTrxDate.Year(), lastTrxDate.Month(), lastTrxDate.Day()+1, 0, 0, 0, 0, common.GetTimezone())
	if from.After(through) {
		// nothing to do
		return nil
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

	holdings := make([]*data.Security, 0, len(pm.holdings))
	for holding := range pm.holdings {
		holdings = append(holdings, &data.Security{
			Ticker:        holding.Ticker,
			CompositeFigi: holding.CompositeFigi,
		})
	}

	dividends, err := data.NewDataRequest(holdings...).Metrics(data.MetricDividendCash).Between(from, through)
	if err != nil {
		log.Error().Err(err).Msg("could not load dividends")
		return err
	}

	splits, err := data.NewDataRequest(holdings...).Metrics(data.MetricSplitFactor).Between(from, through)
	if err != nil {
		log.Error().Err(err).Msg("could not load splits")
		return err
	}

	divTrx := pm.processDividends(dividends)
	splitTrx := pm.processSplits(splits)
	addTransactions := make([]*Transaction, 0, len(divTrx)+len(splitTrx))
	addTransactions = append(addTransactions, divTrx...)
	addTransactions = append(addTransactions, splitTrx...)

	sort.SliceStable(addTransactions, func(i, j int) bool {
		a := addTransactions[i]
		b := addTransactions[j]
		if a.Date.Equal(b.Date) {
			if a.Kind == b.Kind {
				// same type sort by ID
				return hex.EncodeToString(a.ID) < hex.EncodeToString(b.ID)
			}

			// different types dividend first then splits
			return a.Kind == DividendTransaction
		}
		return a.Date.Before(b.Date)
	})
	p.Transactions = append(p.Transactions, addTransactions...)

	return nil
}

// RebalanceTo rebalance the portfolio to the target percentages
// Assumptions: can only rebalance current holdings
func (pm *Model) RebalanceTo(ctx context.Context, allocation *data.SecurityAllocation, justifications []*Justification) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "RebalanceTo")
	defer span.End()

	span.SetAttributes(
		attribute.KeyValue{
			Key:   "date",
			Value: attribute.StringValue(allocation.Date.Format("2006-01-02")),
		},
	)

	p := pm.Portfolio
	err := pm.FillCorporateActions(ctx, allocation.Date)
	if err != nil {
		return err
	}

	nTrx := len(p.Transactions)
	if nTrx > 0 {
		lastDate := p.Transactions[nTrx-1].Date
		if lastDate.After(allocation.Date) {
			log.Error().Stack().Time("Date", allocation.Date).Time("LastTransactionDate", lastDate).
				Msg("cannot rebalance portfolio when date is before last transaction date")
			return ErrTransactionsOutOfOrder
		}
	}

	// check that target sums to 1.0
	var total float64
	for _, v := range allocation.Members {
		total += v
	}

	// Allow for floating point error
	diff := math.Abs(1.0 - total)
	if diff > 1.0e-11 {
		log.Error().Stack().Float64("TotalPercentAllocated", total).Time("Date", allocation.Date).
			Msg("TotalPercentAllocated must equal 1.0")
		return ErrRebalancePercentWrong
	}

	// cash position of the portfolio
	var cash float64
	if currCash, ok := pm.holdings[data.CashSecurity]; ok {
		cash += currCash
	}

	// get the current value of non-cash holdings
	securityValue, err := pm.getPortfolioSecuritiesValue(allocation.Date)
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

		if _, ok := allocation.Members[security]; !ok {
			price := pm.getPriceSafe(allocation.Date, &security)
			t, err := createTransaction(allocation.Date, &security, SellTransaction, price, shares, pm.Portfolio.AccountType, justifications)
			if err != nil {
				return err
			}
			cash += t.TotalValue
			sells = append(sells, t)
		}
	}

	// purchase holdings based on target
	newHoldings := make(map[data.Security]float64)
	for security, targetPercent := range allocation.Members {
		targetDollars := investable * targetPercent
		t, numShares, err := pm.modifyPosition(allocation.Date, &security, targetDollars, justifications)
		if err != nil {
			// don't fail if position could not be modified, just continue -- writing off asset as $0
			log.Warn().Err(err).Time("Date", allocation.Date).Str("Ticker", security.Ticker).Msg("writing off asset")
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

	sells = pm.Portfolio.TaxLots.Update(allocation.Date, buys, sells)
	p.Transactions = append(p.Transactions, sells...)
	p.Transactions = append(p.Transactions, buys...)
	if cash > 1.0e-05 {
		newHoldings[data.CashSecurity] = cash
	}
	pm.holdings = newHoldings

	p.CurrentHoldings = buildHoldingsArray(allocation.Date, newHoldings)

	return nil
}

// Table formats the portfolio into a human-readable string representation
func (pm *Model) Table() string {
	s := &strings.Builder{}

	s.WriteString(
		fmt.Sprintf("Name: %s (%s)\n",
			pm.Portfolio.Name,
			hex.EncodeToString(pm.Portfolio.ID),
		),
	)

	years := int(pm.Portfolio.EndDate.Sub(pm.Portfolio.StartDate)/(time.Hour*24)) / 365
	s.WriteString(
		fmt.Sprintf("Time Period: %s to %s (%dy)\n\n",
			pm.Portfolio.StartDate.Format("2006-01-02"),
			pm.Portfolio.EndDate.Format("2006-01-02"),
			years,
		),
	)

	if len(pm.Portfolio.Transactions) == 0 {
		return "<NO DATA>" // nothing to do as there is no data available in the dataframe
	}

	// construct table header
	tableCols := append([]string{"Date"}, "Action", "Security", "Shares", "Price per Share", "Total")

	// initialize table
	table := tablewriter.NewWriter(s)
	table.SetHeader(tableCols)
	footer := make([]string, len(tableCols))
	footer[0] = "Num Rows"
	footer[1] = fmt.Sprintf("%d", len(pm.Portfolio.Transactions))
	table.SetFooter(footer)
	table.SetBorder(false) // Set Border to false

	for _, trx := range pm.Portfolio.Transactions {
		row := []string{
			trx.Date.Format("2006-01-02"),
			trx.Kind,
			trx.Ticker,
			fmt.Sprintf("%.2f", trx.Shares),
			fmt.Sprintf("$%.2f", trx.PricePerShare),
			fmt.Sprintf("$%.2f", trx.TotalValue),
		}
		table.Append(row)
	}

	table.Render()
	return s.String()
}

// TargetPortfolio invests the portfolio in the ratios specified by the PieHistory `target`.
func (pm *Model) TargetPortfolio(ctx context.Context, target data.PortfolioPlan) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "TargetPortfolio")
	defer span.End()

	p := pm.Portfolio

	log.Info().Msg("building target portfolio")
	if len(target) == 0 {
		log.Debug().Stack().Msg("target rows = 0; nothing to do!")
		return nil
	}

	// Set time range of portfolio
	p.EndDate = target.EndDate()
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
		p.StartDate = target.StartDate()
		p.Transactions[0].Date = p.StartDate
	}

	// Create transactions
	for _, allocation := range target {
		// HACK - if the date is Midnight adjust to market close (i.e. 4pm EST)
		// This should really be set correctly for the day. The problem is if the
		// transaction is on a day where the market closes early (either because of
		// a holiday or because a "circuit-breaker" threshold was reached) we do
		// not have a reliable datasource that tells us the time the market closed.
		//
		// Generally speaking getting the time slightly off here is immaterial so this
		// is an OK hack
		if allocation.Date.Hour() == 0 && allocation.Date.Minute() == 0 && allocation.Date.Second() == 0 {
			allocation.Date = allocation.Date.Add(time.Hour * 16)
		}

		justifications := make([]*Justification, 0, len(allocation.Justifications))
		for k, v := range allocation.Justifications {
			j := &Justification{
				Key:   k,
				Value: v,
			}
			justifications = append(justifications, j)
		}

		pm.justifications[allocation.Date.String()] = justifications

		err := pm.RebalanceTo(ctx, allocation, justifications)
		if err != nil {
			return err
		}
	}
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
	trxCalcStartDate := p.EndDate.AddDate(0, -6, 1)
	addTrxOnDate := p.EndDate.AddDate(0, 0, 1)
	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Logger()

	if through.Before(trxCalcStartDate) {
		span.SetStatus(codes.Error, "cannot update portfolio due to dates being out of order")
		subLog.Error().Stack().
			Time("Begin", trxCalcStartDate).
			Time("End", through).
			Msg("cannot update portfolio dates are out of order")
		return ErrInvalidDateRange
	}

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

	subLog.Info().Time("TrxCalcStartDate", trxCalcStartDate).Time("AddTrxOnDate", addTrxOnDate).Msg("computing portfolio strategy over date period")
	targetPortfolio, predictedAssets, err := stratObject.Compute(ctx, trxCalcStartDate, through)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to execute portfolio strategy")
		subLog.Error().Err(err).Stack().Msg("failed to execute portfolio strategy")
		return err
	}

	// thin the targetPortfolio to only include info on or after the startDate
	targetPortfolio = targetPortfolio.Trim(addTrxOnDate, through)

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

	pm.Portfolio.PredictedAssets = BuildPredictedHoldings(predictedAssets.Date, predictedAssets.Members, predictedAssets.Justifications)
	p.EndDate = through

	return nil
}

// DATABASE FUNCTIONS

// LOAD

func (pm *Model) LoadTransactionsFromDB(ctx context.Context) error {
	p := pm.Portfolio
	trx, err := database.TrxForUser(ctx, p.UserID)
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
		gain_loss::double precision,
		source,
		encode(source_id, 'hex'),
		tags,
		tax_type,
		ticker,
		total_value
	FROM portfolio_transactions
	WHERE portfolio_id=$1 AND user_id=$2
	ORDER BY sequence_num`
	rows, err := trx.Query(ctx, transactionSQL, p.ID, p.UserID)
	if err != nil {
		log.Error().Stack().Err(err).
			Str("PortfolioID", hex.EncodeToString(p.ID)).
			Str("UserID", p.UserID).
			Str("Query", transactionSQL).
			Msg("could not load portfolio transactions from database")
		if err := trx.Rollback(ctx); err != nil {
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
		var gainLoss pgtype.Float8

		var sourceID pgtype.Text

		err := rows.Scan(&t.ID, &t.Date, &t.Cleared, &t.Commission, &compositeFIGI,
			&t.Justification, &t.Kind, &memo, &pricePerShare, &shares, &gainLoss, &t.Source,
			&sourceID, &t.Tags, &taxDisposition, &t.Ticker, &t.TotalValue)
		if err != nil {
			log.Warn().Stack().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("UserID", p.UserID).
				Str("Query", transactionSQL).
				Msg("failed scanning row into transaction fields")
			if err := trx.Rollback(ctx); err != nil {
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
		if gainLoss.Status == pgtype.Present {
			t.GainLoss = gainLoss.Float
		}
		if sourceID.Status == pgtype.Present {
			t.SourceID = sourceID.String
		}

		// put event date in the correct time zone
		t.Date = time.Date(t.Date.Year(), t.Date.Month(), t.Date.Day(), 16, 0, 0, 0, common.GetTimezone())

		transactions = append(transactions, &t)
	}
	p.Transactions = transactions

	if err := trx.Commit(ctx); err != nil {
		log.Error().Stack().Err(err).Msg("could not commit transaction to database")
	}

	return nil
}

func LoadFromDB(ctx context.Context, portfolioIDs []string, userID string) ([]*Model, error) {
	subLog := log.With().Str("UserID", userID).Strs("PortfolioIDs", portfolioIDs).Logger()
	if userID == "" {
		subLog.Error().Stack().Msg("userID cannot be an empty string")
		return nil, ErrEmptyUserID
	}
	trx, err := database.TrxForUser(ctx, userID)
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
		rows, err = trx.Query(ctx, portfolioSQL, portfolioIDs, userID)
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
		rows, err = trx.Query(ctx, portfolioSQL, userID)
	}

	if err != nil {
		subLog.Warn().Stack().Err(err).Msg("could not load portfolio from database")
		if err := trx.Rollback(ctx); err != nil {
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
			justifications: make(map[string][]*Justification),
		}

		tz := common.GetTimezone()
		tmpHoldings := make(map[string]float64)
		err = rows.Scan(&p.ID, &p.Name, &p.StrategyShortcode, &p.StrategyArguments, &p.StartDate, &p.EndDate, &tmpHoldings, &p.Notifications, &p.Benchmark)

		pm.holdings = make(map[data.Security]float64, len(tmpHoldings))
		for k, v := range tmpHoldings {
			if k == data.CashAsset {
				pm.holdings[data.CashSecurity] = v
			} else {
				security, err := data.SecurityFromFigi(k)
				if err != nil {
					subLog.Warn().Str("CompositeFigi", k).Msg("portfolio holds inactive security")
					continue
				}
				pm.holdings[*security] = v
			}
		}

		p.StartDate = p.StartDate.In(tz)
		p.EndDate = p.EndDate.In(tz)
		if err != nil {
			subLog.Warn().Stack().Err(err).Msg("could not load portfolio from database")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return nil, err
		}

		p.CurrentHoldings = buildHoldingsArray(time.Now(), pm.holdings)
		resultSet = append(resultSet, pm)
	}

	if len(resultSet) == 0 && len(portfolioIDs) != 0 {
		return nil, ErrPortfolioNotFound
	}

	if err := trx.Commit(ctx); err != nil {
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

func (pm *Model) SaveActivities(ctx context.Context) error {
	p := pm.Portfolio
	userID := p.UserID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Logger()

	if pm.activities == nil {
		pm.activities = make([]*Activity, 0, 5)
	}

	// Save to database
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	for _, activity := range pm.activities {

		sql := `INSERT INTO activity ("user_id", "portfolio_id", "event_date", "activity", "tags") VALUES ($1, $2, $3, $4, $5)`
		if _, err := trx.Exec(ctx, sql, userID, p.ID, activity.Date, activity.Msg, activity.Tags); err != nil {
			subLog.Error().Err(err).Msg("could not create activity")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			return err
		}
	}

	err = trx.Commit(ctx)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return err
	}

	return nil
}

func (pm *Model) SetStatus(ctx context.Context, msg string) error {
	p := pm.Portfolio
	userID := p.UserID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Str("Status", msg).Logger()

	// Save to database
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	sql := `UPDATE portfolios SET status=$1 WHERE id=$2`
	if _, err := trx.Exec(ctx, sql, msg, p.ID); err != nil {
		subLog.Error().Err(err).Msg("could not update portfolio status")
	}

	err = trx.Commit(ctx)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

// Save portfolio to database along with all transaction data
func (pm *Model) Save(ctx context.Context, userID string) error {
	p := pm.Portfolio
	p.UserID = userID

	subLog := log.With().Str("PortfolioID", hex.EncodeToString(p.ID)).Str("Strategy", p.StrategyShortcode).Str("UserID", userID).Logger()

	if err := pm.SaveActivities(ctx); err != nil {
		subLog.Error().Err(err).Msg("could not save activities")
	}

	// Save to database
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	err = pm.SaveWithTransaction(ctx, trx, userID, false)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to create portfolio transactions")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	err = trx.Commit(ctx)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to commit portfolio transaction")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

func (pm *Model) SaveWithTransaction(ctx context.Context, trx pgx.Tx, userID string, permanent bool) error {
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
		"predicted_bytes",
		"tax_lot_bytes"
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
		$13
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
		predicted_bytes=$12,
		tax_lot_bytes=$13`

	marshableHoldings := make(map[string]float64, len(pm.holdings))
	for k, v := range pm.holdings {
		marshableHoldings[k.CompositeFigi] = v
	}

	holdings, err := json.MarshalContext(ctx, marshableHoldings)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to marshal holdings")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	predictedBytes, err := p.PredictedAssets.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("Could not marshal predicted bytes")
	}

	taxLotBytes, err := p.TaxLots.MarshalBinary()
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("Could not marshal tax lot bytes")
	}

	_, err = trx.Exec(ctx, portfolioSQL, p.ID, p.Name, p.StrategyShortcode,
		p.StrategyArguments, p.Benchmark, p.StartDate, p.EndDate, holdings, p.Notifications, temporary, userID,
		predictedBytes, taxLotBytes)
	if err != nil {
		subLog.Error().Stack().Err(err).Str("Query", portfolioSQL).Msg("failed to save portfolio")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return pm.saveTransactions(ctx, trx, userID)
}

func (pm *Model) saveTransactions(ctx context.Context, trx pgx.Tx, userID string) error {
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
		"gain_loss",
		"source",
		"source_id",
		"related",
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
		decode($14, 'hex'),
		$15,
		$16,
		$17,
		$18,
		$19,
		$20,
		$21
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
		gain_loss=$12,
		source=$13,
		source_id=decode($14, 'hex'),
		related=$15,
		tags=$16,
		tax_type=$17,
		ticker=$18,
		total_value=$19,
		sequence_num=$20`

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
			jsonJustification, err = json.MarshalContext(ctx, t.Justification)
			if err != nil {
				log.Error().Err(err).Msg("could not marshal to JSON the justification array")
			}
		}
		related := make([]string, len(t.Related))
		for idx, relatedID := range t.Related {
			related[idx] = hex.EncodeToString(relatedID)
		}
		_, err := trx.Exec(ctx, transactionSQL,
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
			t.GainLoss,        // 12
			t.Source,          // 13
			t.SourceID,        // 14
			related,           // 15
			t.Tags,            // 16
			t.TaxDisposition,  // 17
			t.Ticker,          // 18
			t.TotalValue,      // 19
			idx,               // 20
			userID,            // 21
		)
		if err != nil {
			log.Warn().Stack().Err(err).
				Str("PortfolioID", hex.EncodeToString(p.ID)).
				Str("Query", transactionSQL).
				Bytes("Justification", jsonJustification).
				Int("Idx", idx).
				Str("UserID", userID).
				Object("Transaction", t).
				Msg("failed to save transaction")
			if err := trx.Rollback(ctx); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}

			return err
		}
	}

	return nil
}

// Private API

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

// computeTransactionSourceID calculates a 16-byte blake3 hash using the date, source, composite figi, ticker, kind, price per share, shares, and total value
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

func createTransaction(date time.Time, security *data.Security, kind string, price float64, shares float64, taxDisposition string, justification []*Justification) (*Transaction, error) {
	trxID, err := uuid.New().MarshalBinary()
	if err != nil {
		log.Warn().Stack().Err(err).Msg("could not marshal uuid to binary")
		return nil, err
	}

	log.Debug().Str("Ticker", security.Ticker).Str("Kind", kind).Float64("Shares", shares).Float64("TotalValue", shares*price).Float64("Price", price).Time("TrxDate", date).Msg("creating transaction")

	t := Transaction{
		ID:             trxID,
		Date:           date,
		Ticker:         security.Ticker,
		CompositeFIGI:  security.CompositeFigi,
		Kind:           kind,
		PricePerShare:  price,
		Shares:         shares,
		TotalValue:     shares * price,
		TaxDisposition: taxDisposition,
		Justification:  justification,
		Source:         SourceName,
	}

	err = computeTransactionSourceID(&t)
	if err != nil {
		log.Warn().Stack().Err(err).Time("TransactionDate", date).Str("TransactionTicker", security.Ticker).Str("TransactionType", kind).Msg("couldn't compute SourceID for transaction")
	}

	return &t, nil
}

func (pm *Model) getPortfolioSecuritiesValue(date time.Time) (float64, error) {
	var securityValue float64

	for k, v := range pm.holdings {
		if k != data.CashSecurity {
			price, err := data.NewDataRequest(&k).Metrics(data.MetricClose).OnSingle(date)
			if err != nil {
				log.Warn().Stack().Str("Symbol", k.Ticker).Time("Date", date).Float64("Price", price).
					Msg("security price data not available.")
				return 0, ErrSecurityPriceNotAvailable
			}

			log.Debug().Time("Date", date).Str("Ticker", k.Ticker).Float64("Price", price).Float64("Value", v*price).
				Msg("Retrieve price data")

			if !math.IsNaN(price) {
				securityValue += v * price
			}
		}
	}

	return securityValue, nil
}

func (pm *Model) getPriceSafe(date time.Time, security *data.Security) float64 {
	price, err := data.NewDataRequest(security).Metrics(data.MetricClose).OnSingle(date)
	if err != nil {
		log.Warn().Stack().Err(err).Str("Ticker", security.Ticker).Msg("DataRequest.OnSingle returned an error")
		price = 0.0
	}
	if math.IsNaN(price) {
		log.Warn().Stack().Str("Ticker", security.Ticker).Time("ForDate", date).Msg("price is NaN")
		price = 0.0
	}
	return price
}

// modifyPosition creates a new transaction such that the final holdings of the portfolio for the specified security matches the
// targetDollars requested. Returns the new transaction, the number of shares transacted, and any errors that may have occurred.
func (pm *Model) modifyPosition(date time.Time, security *data.Security, targetDollars float64, justification []*Justification) (*Transaction, float64, error) {
	// is this security currently held? If so, do we need to buy more or sell some
	price := pm.getPriceSafe(date, security)
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

			t, err = createTransaction(date, security, SellTransaction, price, toSellShares, pm.Portfolio.AccountType, justification)
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

			t, err = createTransaction(date, security, BuyTransaction, price, toBuyShares, pm.Portfolio.AccountType, justification)
			if err != nil {
				return nil, 0, err
			}
		}
	} else {
		// this is a new position
		shares := targetDollars / price
		subLog := log.With().Str("Ticker", security.Ticker).Str("Kind", "BuyTransaction").Float64("Shares", shares).Float64("TotalValue", targetDollars).Time("Date", date).Logger()
		if shares <= 1.0e-5 {
			subLog.Warn().Stack().Msg("refusing to buy 0 shares")
			return nil, 0, ErrRebalancePercentWrong
		}

		t, err = createTransaction(date, security, BuyTransaction, price, shares, pm.Portfolio.AccountType, justification)
		if err != nil {
			return nil, 0, err
		}
	}

	return t, targetShares, nil
}
