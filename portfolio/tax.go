// Copyright 2021-2023
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
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/rs/zerolog/log"
)

// account types
const (
	Roth        = "ROTH"
	TaxDeferred = "DEFERRED"
	Taxable     = "TAXABLE"
)

// capital gains
const (
	LTC                  = "LTC"
	STC                  = "STC"
	QualifiedDividend    = "QUALIFIED"
	NonQualifiedDividend = "NON-QUALIFIED"
)

const (
	TaxLotFIFOMethod = "TaxLotFIFO"
)

// CheckIfDividendIsQualified verifies if the specified dividend should be treated as a qualified dividend or non-qualified
// Qualified dividends are defined in IRS publication 550 (https://www.irs.gov/forms-pubs/about-publication-550)
// The maximum rate on qualified dividends applies only if all of the following requirements are met.
//   - The dividends must have been paid by a U.S. corporation or a qualified foreign corporation. (See table 1-3 in pub 550)
//   - The dividends are not of the type listed later under Dividends that are not qualified dividends. (see pub 550)
//   - Stock has been held for more than 60 days during the 121-day period that begins 60 days before the ex-dividend date.
//     The ex-dividend date is the first date following the declaration of a dividend on which the buyer of a stock is not entitled to
//     receive the next dividend payment. When counting the number of days you held the stock, include the day you disposed of the stock,
//     but not the day you acquired it.
//
// For penny vault's purposes all dividends are assumed to be qualifying *if* the holding requirement is met. If only a portion of the
// dividend meets the holding requirement then penny vault assumes 100% of the dividend is qualified. This assumption may cause
// penny vault to understate the true tax percentage.
//
// NOTE: if a dividend is not qualified at the time of the ex-dividend date Penny Vault does not re-evaluate if it would be come qualified
// at a future date. This may cause Penny Vault to over-state the amount of taxes paid on dividends
func (t *TaxLotInfo) CheckIfDividendIsQualified(trx *Transaction) bool {
	if trx.Kind != DividendTransaction {
		log.Error().Object("Transaction", trx).Msg("non-dividend transactions cannot be used")
		return false
	}

	qualifyingDate := trx.Date.AddDate(0, 0, 60)
	for _, lot := range t.Items {
		if lot.CompositeFIGI == trx.CompositeFIGI && lot.Date.Before(qualifyingDate) {
			return true
		}
	}

	return false
}

// Update the TaxLotInfo structure wuth the specified buy and sell transactions using the specified trax lot identification method
func (t *TaxLotInfo) Update(date time.Time, buys []*Transaction, sells []*Transaction) []*Transaction {
	t.AsOf = date
	// record buys as new tax lots
	for _, buy := range buys {
		// double check that it's a buy transaction
		if buy.Kind != BuyTransaction {
			continue
		}

		taxLot := &TaxLot{
			Date:          buy.Date,
			TransactionID: buy.ID,
			CompositeFIGI: buy.CompositeFIGI,
			Ticker:        buy.Ticker,
			Shares:        buy.Shares,
			PricePerShare: buy.PricePerShare,
		}
		t.Items = append(t.Items, taxLot)
	}

	// link sell transactions with tax lots and calculate GainLoss
	newSells := make([]*Transaction, 0, len(sells))
	for _, sell := range sells {
		// double check that it's a sell transaction
		if sell.Kind != SellTransaction {
			continue
		}

		enrichedTransactions := t.LinkTransactionsWithTaxLots(sell)
		newSells = append(newSells, enrichedTransactions...)
	}

	return newSells
}

// LinkTransactionsWithTaxLots identifies which tax lots should be linked with the sell transaction and calculate a gain/loss for the
// transaction based on the identified tax lots
func (t *TaxLotInfo) LinkTransactionsWithTaxLots(sell *Transaction) []*Transaction {
	switch t.Method {
	case TaxLotFIFOMethod:
		return t.LinkWithFIFOMethod(sell)
	default:
		return t.LinkWithFIFOMethod(sell)
	}
}

// LinkWithFIFOMethod identifies tax lots using the first-in, first-out method where the oldest shares acquired are sold first
func (t *TaxLotInfo) AdjustForSplit(security *data.Security, splitFactor float64) {
	for _, taxLot := range t.Items {
		if taxLot.CompositeFIGI == security.CompositeFigi {
			taxLot.Shares *= splitFactor
		}
	}
}

// LinkWithFIFOMethod identifies tax lots using the first-in, first-out method where the oldest shares acquired are sold first
func (t *TaxLotInfo) LinkWithFIFOMethod(sell *Transaction) []*Transaction {
	if sell.Kind != SellTransaction || sell.Predicted {
		return []*Transaction{sell}
	}

	numSharesToFind := sell.Shares
	remainingTaxLots := make([]*TaxLot, 0, len(t.Items))

	var ltcGainLoss float64
	var stcGainLoss float64
	var stcShares float64
	var ltcShares float64
	ltcIdentifiedTransactions := [][]byte{}
	stcIdentifiedTransactions := [][]byte{}

	for idx, taxLot := range t.Items {
		if taxLot.CompositeFIGI != sell.CompositeFIGI {
			remainingTaxLots = append(remainingTaxLots, taxLot)
			continue
		}

		sharesExercised := taxLot.Shares
		numSharesToFind -= taxLot.Shares
		var sharesLeft float64
		if numSharesToFind < 0 {
			sharesExercised += numSharesToFind
			sharesLeft = math.Abs(numSharesToFind)
			taxLot.Shares = sharesLeft
			if sharesLeft > 1e-5 {
				remainingTaxLots = append(remainingTaxLots, taxLot)
			}
		}

		ltcDate := sell.Date.AddDate(-1, 0, 0).Add(time.Nanosecond * -1) // subtract 1 nanosecond so we don't have to do before and equal
		if taxLot.Date.Before(ltcDate) {
			// its a long-term capital gain
			ltcShares += sharesExercised
			ltcGainLoss += (sharesExercised * sell.PricePerShare) - (sharesExercised * taxLot.PricePerShare)
			ltcIdentifiedTransactions = append(ltcIdentifiedTransactions, taxLot.TransactionID)
		} else {
			// its a short-term capital gain
			stcShares += sharesExercised
			stcGainLoss += (sharesExercised * sell.PricePerShare) - (sharesExercised * taxLot.PricePerShare)
			stcIdentifiedTransactions = append(stcIdentifiedTransactions, taxLot.TransactionID)
		}

		// if all shares have been linked with tax lots then we are done
		if numSharesToFind <= 0 {
			if len(t.Items) > idx+1 {
				remainingTaxLots = append(remainingTaxLots, t.Items[idx+1:]...)
			}
			break
		}
	}

	// double check that all taxLots are applied
	if numSharesToFind >= 1e-5 {
		log.Warn().Object("SellTransaction", sell).Float64("NumSharesLeft", numSharesToFind).Float64("ltcGainLoss", ltcGainLoss).Float64("stcGainLoss", stcGainLoss).Msg("taxLots and transactions are out-of-sync")
	}

	t.Items = remainingTaxLots

	transactions := make([]*Transaction, 0, 2)
	if ltcGainLoss != 0 {
		trxID, err := uuid.New().MarshalBinary()
		if err != nil {
			log.Panic().Stack().Err(err).Msg("could not marshal uuid to binary")
		}

		trx := &Transaction{
			ID:             trxID,
			Cleared:        sell.Cleared,
			Commission:     sell.Commission,
			CompositeFIGI:  sell.CompositeFIGI,
			Date:           sell.Date,
			GainLoss:       ltcGainLoss,
			Justification:  sell.Justification,
			Kind:           sell.Kind,
			Memo:           sell.Memo,
			Predicted:      sell.Predicted,
			PricePerShare:  sell.PricePerShare,
			Related:        ltcIdentifiedTransactions,
			Shares:         ltcShares,
			Source:         sell.Source,
			Tags:           sell.Tags,
			TaxDisposition: LTC,
			Ticker:         sell.Ticker,
			TotalValue:     (ltcShares*sell.PricePerShare + sell.Commission),
		}

		err = computeTransactionSourceID(trx)
		if err != nil {
			log.Warn().Stack().Err(err).Object("Transaction", trx).Msg("couldn't compute SourceID for transaction")
		}

		transactions = append(transactions, trx)
	}

	if stcGainLoss != 0 {
		trxID, err := uuid.New().MarshalBinary()
		if err != nil {
			log.Panic().Stack().Err(err).Msg("could not marshal uuid to binary")
		}

		trx := &Transaction{
			ID:             trxID,
			Cleared:        sell.Cleared,
			Commission:     sell.Commission,
			CompositeFIGI:  sell.CompositeFIGI,
			Date:           sell.Date,
			GainLoss:       stcGainLoss,
			Justification:  sell.Justification,
			Kind:           sell.Kind,
			Memo:           sell.Memo,
			Predicted:      sell.Predicted,
			PricePerShare:  sell.PricePerShare,
			Related:        stcIdentifiedTransactions,
			Shares:         stcShares,
			Source:         sell.Source,
			Tags:           sell.Tags,
			TaxDisposition: STC,
			Ticker:         sell.Ticker,
			TotalValue:     (stcShares*sell.PricePerShare + sell.Commission),
		}

		err = computeTransactionSourceID(trx)
		if err != nil {
			log.Warn().Stack().Err(err).Object("Transaction", trx).Msg("couldn't compute SourceID for transaction")
		}

		transactions = append(transactions, trx)
	}

	return transactions
}

// Security returns the security associated with the tax lot
func (t *TaxLot) Security() *data.Security {
	return data.MustSecurityFromFigi(t.CompositeFIGI)
}

func (t *TaxLotInfo) Copy() *TaxLotInfo {
	taxLotBinary, err := t.MarshalBinary()
	taxLots := &TaxLotInfo{}
	if err != nil {
		log.Error().Err(err).Msg("error marshalling taxLotInfo to binary")
		return taxLots
	}
	_, err = taxLots.Unmarshal(taxLotBinary)
	if err != nil {
		log.Error().Err(err).Msg("error unmarshalling taxLotInfo from binary")
	}
	return taxLots
}

func taxRatesForUser(ctx context.Context, userID string) *taxRates {
	subLog := log.With().Str("Method", "taxRatesForUser").Str("UserID", userID).Logger()
	subLog.Debug().Msg("Fetching users tax rates")

	if userID == "" {
		subLog.Debug().Msg("using default tax rates")
		return &taxRates{
			NonQualifiedDividendsAndInterestIncome: .35,
			QualifiedDividends:                     .15,
			LTCTaxRate:                             .15,
			STCTaxRate:                             .35,
		}
	}

	taxRatesSQL := `SELECT interest_income_and_non_qualified_dividends, qualified_dividend_rate, ltc_tax_rate, stc_tax_rate FROM profile WHERE user_id=$1`
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("unable to get database transaction for user, using default rates")
		return &taxRates{
			NonQualifiedDividendsAndInterestIncome: .35,
			QualifiedDividends:                     .15,
			LTCTaxRate:                             .15,
			STCTaxRate:                             .35,
		}
	}

	var unqualified float64
	var qualified float64
	var ltc float64
	var stc float64

	err = trx.QueryRow(ctx, taxRatesSQL, userID).Scan(&unqualified, &qualified, &ltc, &stc)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("query tax rates failed")
		if err := trx.Rollback(ctx); err != nil {
			log.Error().Stack().Err(err).Msg("could not rollback transaction")
		}
		return &taxRates{
			NonQualifiedDividendsAndInterestIncome: .35,
			QualifiedDividends:                     .15,
			LTCTaxRate:                             .15,
			STCTaxRate:                             .35,
		}
	}

	if err := trx.Commit(ctx); err != nil {
		subLog.Warn().Stack().Err(err).Msg("commit transaction failed")
		return &taxRates{
			NonQualifiedDividendsAndInterestIncome: .35,
			QualifiedDividends:                     .15,
			LTCTaxRate:                             .15,
			STCTaxRate:                             .35,
		}
	}

	return &taxRates{
		NonQualifiedDividendsAndInterestIncome: unqualified,
		QualifiedDividends:                     qualified,
		LTCTaxRate:                             ltc,
		STCTaxRate:                             stc,
	}
}
