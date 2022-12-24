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
	"math"
	"time"

	"github.com/google/uuid"
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
	LTC = "LTC"
	STC = "STC"
)

const (
	TaxLotFIFOMethod = "TaxLotFIFO"
)

// identify which tax lots should be linked with the sell transaction and calculate a gain/loss for the transaction based on the
// identified tax lots
func (pm *Model) identifyTaxLots(sell *Transaction) []*Transaction {
	switch pm.Portfolio.TaxLotMethod {
	case TaxLotFIFOMethod:
		return pm.taxLotFIFO(sell)
	default:
		return pm.taxLotFIFO(sell)
	}
}

// taxLotFIFO identifies tax lots using the first-in, first-out method where the oldest shares acquired are sold first
func (pm *Model) taxLotFIFO(sell *Transaction) []*Transaction {
	if sell.Kind != SellTransaction || sell.Predicted {
		return []*Transaction{sell}
	}

	numSharesToFind := sell.Shares
	remainingTaxLots := make([]*TaxLot, 0, len(pm.Portfolio.TaxLots))

	var ltcGainLoss float64
	var stcGainLoss float64
	var stcShares float64
	var ltcShares float64
	ltcIdentifiedTransactions := [][]byte{}
	stcIdentifiedTransactions := [][]byte{}

	for idx, taxLot := range pm.Portfolio.TaxLots {
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
			if len(pm.Portfolio.TaxLots) > idx+1 {
				remainingTaxLots = append(remainingTaxLots, pm.Portfolio.TaxLots[idx+1:]...)
			}
			break
		}
	}

	// double check that all taxLots are applied
	if numSharesToFind >= 1e-5 {
		log.Panic().Object("SellTransaction", sell).Float64("NumSharesLeft", numSharesToFind).Float64("ltcGainLoss", ltcGainLoss).Float64("stcGainLoss", stcGainLoss).Msg("taxLots and transactions are out-of-sync")
	}

	pm.Portfolio.TaxLots = remainingTaxLots

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
