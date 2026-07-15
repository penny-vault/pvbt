// Copyright 2021-2026
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

package summary

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// predictedHolding describes one position in the predicted portfolio.
type predictedHolding struct {
	ticker      string
	shares      float64
	marketValue float64
	weight      float64
}

// prediction holds the predicted activity for the next scheduled trade date.
type prediction struct {
	date     time.Time
	trades   []tradeEntry
	holdings []predictedHolding
}

// buildPrediction extracts the account's stored prediction for display.
// Returns nil if no prediction was recorded.
func buildPrediction(acct portfolio.Portfolio) *prediction {
	pred := acct.Prediction()
	if pred == nil {
		return nil
	}

	result := &prediction{date: pred.Date}

	for _, txn := range pred.Transactions {
		switch txn.Type {
		case asset.BuyTransaction, asset.SellTransaction:
			result.trades = append(result.trades, tradeEntry{
				date:   txn.Date,
				action: txn.Type.String(),
				ticker: txn.Asset.Ticker,
				shares: txn.Qty,
				price:  txn.Price,
				amount: txn.Amount,
			})
		}
	}

	totalValue := 0.0

	for _, holding := range pred.Holdings {
		if !math.IsNaN(holding.MarketValue) {
			totalValue += holding.MarketValue
		}
	}

	for _, holding := range pred.Holdings {
		weight := math.NaN()
		if totalValue > 0 {
			weight = holding.MarketValue / totalValue
		}

		result.holdings = append(result.holdings, predictedHolding{
			ticker:      holding.Asset.Ticker,
			shares:      holding.Quantity,
			marketValue: holding.MarketValue,
			weight:      weight,
		})
	}

	return result
}

// renderPrediction writes the predicted trades and resulting holdings for
// the next scheduled trade date. A nil prediction renders nothing.
func renderPrediction(builder *strings.Builder, pred *prediction) {
	if pred == nil {
		return
	}

	builder.WriteString(sectionTitleStyle.Render(
		fmt.Sprintf("Predicted Trades for %s", pred.date.Format("2006-01-02"))))
	builder.WriteString("\n")

	const (
		actionCol = 8
		tickerCol = 8
		sharesCol = 10
		priceCol  = 12
		amountCol = 14
		weightCol = 10
	)

	if len(pred.trades) == 0 {
		builder.WriteString("  " + dimStyle.Render("No trades predicted") + "\n")
	} else {
		hdr := padRight(tableHeaderStyle.Render("Action"), actionCol) +
			padRight(tableHeaderStyle.Render("Ticker"), tickerCol) +
			padLeft(tableHeaderStyle.Render("Shares"), sharesCol) +
			padLeft(tableHeaderStyle.Render("Price"), priceCol) +
			padLeft(tableHeaderStyle.Render("Amount"), amountCol)

		builder.WriteString("  " + hdr + "\n")

		for _, trade := range pred.trades {
			actionStyled := valueStyle.Render(trade.action)
			switch trade.action {
			case "Buy":
				actionStyled = positiveStyle.Render(trade.action)
			case "Sell":
				actionStyled = negativeStyle.Render(trade.action)
			}

			line := padRight(actionStyled, actionCol) +
				padRight(valueStyle.Render(trade.ticker), tickerCol) +
				padLeft(valueStyle.Render(fmt.Sprintf("%.2f", trade.shares)), sharesCol) +
				padLeft(fmtCurrency(trade.price), priceCol) +
				padLeft(fmtCurrency(trade.amount), amountCol)

			builder.WriteString("  " + line + "\n")
		}
	}

	if len(pred.holdings) == 0 {
		return
	}

	builder.WriteString("\n")
	builder.WriteString(sectionTitleStyle.Render("Predicted Holdings"))
	builder.WriteString("\n")

	hdr := padRight(tableHeaderStyle.Render("Ticker"), tickerCol) +
		padLeft(tableHeaderStyle.Render("Shares"), sharesCol) +
		padLeft(tableHeaderStyle.Render("Value"), amountCol) +
		padLeft(tableHeaderStyle.Render("Weight"), weightCol)

	builder.WriteString("  " + hdr + "\n")

	for _, holding := range pred.holdings {
		line := padRight(valueStyle.Render(holding.ticker), tickerCol) +
			padLeft(valueStyle.Render(fmt.Sprintf("%.2f", holding.shares)), sharesCol) +
			padLeft(fmtCurrency(holding.marketValue), amountCol) +
			padLeft(fmtPct(holding.weight), weightCol)

		builder.WriteString("  " + line + "\n")
	}
}
