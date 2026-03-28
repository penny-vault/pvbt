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
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

const maxTradesShown = 10

func buildTrades(acct portfolio.Portfolio, warnings *[]string) trades {
	transactions := acct.Transactions()

	var entries []tradeEntry

	totalTransactions := 0
	roundTrips := 0

	for _, txn := range transactions {
		switch txn.Type {
		case asset.BuyTransaction, asset.SellTransaction:
			totalTransactions++

			if txn.Type == asset.SellTransaction {
				roundTrips++
			}

			entries = append(entries, tradeEntry{
				date:   txn.Date,
				action: txn.Type.String(),
				ticker: txn.Asset.Ticker,
				shares: txn.Qty,
				price:  txn.Price,
				amount: txn.Amount,
			})
		}
	}

	result := trades{
		totalTransactions: totalTransactions,
		roundTrips:        roundTrips,
		tradeList:         entries,
	}

	tradeMetrics, err := acct.TradeMetrics()
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("trade metrics: %v", err))
	} else {
		result.winRate = tradeMetrics.WinRate
		result.avgHolding = tradeMetrics.AverageHoldingPeriod
		result.avgWin = tradeMetrics.AverageWin
		result.avgLoss = tradeMetrics.AverageLoss
		result.profitFactor = tradeMetrics.ProfitFactor
		result.gainLossRatio = tradeMetrics.GainLossRatio
		result.turnover = tradeMetrics.Turnover
		result.positivePeriods = tradeMetrics.NPositivePeriods
	}

	return result
}

// renderTrades writes trade summary stats and the trade log.
func renderTrades(builder *strings.Builder, tr trades) {
	builder.WriteString(sectionTitleStyle.Render("Trade Summary"))
	builder.WriteString("\n")

	// Summary stats in 2-column layout.
	const (
		summaryLabel = 20
		summaryVal   = 14
	)

	type statPair struct {
		leftLabel  string
		leftValue  string
		rightLabel string
		rightValue string
	}

	pairs := []statPair{
		{
			leftLabel:  "Win Rate",
			leftValue:  fmtPct(tr.winRate),
			rightLabel: "Avg Holding",
			rightValue: fmtDays(tr.avgHolding),
		},
		{
			leftLabel:  "Avg Win",
			leftValue:  fmtCurrency(tr.avgWin),
			rightLabel: "Avg Loss",
			rightValue: fmtCurrency(tr.avgLoss),
		},
		{
			leftLabel:  "Profit Factor",
			leftValue:  fmtRatio(tr.profitFactor),
			rightLabel: "Gain/Loss",
			rightValue: fmtRatio(tr.gainLossRatio),
		},
		{
			leftLabel:  "Turnover",
			leftValue:  fmtPct(tr.turnover),
			rightLabel: "Positive Periods",
			rightValue: fmtPct(tr.positivePeriods),
		},
	}

	for _, pair := range pairs {
		left := padRight(labelStyle.Render(pair.leftLabel), summaryLabel) + padLeft(pair.leftValue, summaryVal)
		right := padRight(labelStyle.Render(pair.rightLabel), summaryLabel) + padLeft(pair.rightValue, summaryVal)
		builder.WriteString("  " + left + "  " + right + "\n")
	}

	// Trade log.
	if len(tr.tradeList) == 0 {
		return
	}

	builder.WriteString("\n")
	builder.WriteString(sectionTitleStyle.Render("Recent Trades"))
	builder.WriteString("\n")

	// Column widths for trade table.
	const (
		dateCol   = 12
		actionCol = 8
		tickerCol = 8
		sharesCol = 10
		priceCol  = 12
		amountCol = 14
	)

	// Header.
	hdr := padRight(tableHeaderStyle.Render("Date"), dateCol) +
		padRight(tableHeaderStyle.Render("Action"), actionCol) +
		padRight(tableHeaderStyle.Render("Ticker"), tickerCol) +
		padLeft(tableHeaderStyle.Render("Shares"), sharesCol) +
		padLeft(tableHeaderStyle.Render("Price"), priceCol) +
		padLeft(tableHeaderStyle.Render("Amount"), amountCol)

	builder.WriteString("  " + hdr + "\n")

	// Show the last N trades.
	startIdx := 0
	truncated := false

	if len(tr.tradeList) > maxTradesShown {
		startIdx = len(tr.tradeList) - maxTradesShown
		truncated = true
	}

	if truncated {
		earlier := len(tr.tradeList) - maxTradesShown
		builder.WriteString("  " + dimStyle.Render(fmt.Sprintf("... and %d earlier transactions", earlier)) + "\n")
	}

	for idx := startIdx; idx < len(tr.tradeList); idx++ {
		trade := tr.tradeList[idx]

		actionStyled := valueStyle.Render(trade.action)
		switch trade.action {
		case "BUY":
			actionStyled = positiveStyle.Render(trade.action)
		case "SELL":
			actionStyled = negativeStyle.Render(trade.action)
		}

		line := padRight(dimStyle.Render(trade.date.Format("2006-01-02")), dateCol) +
			padRight(actionStyled, actionCol) +
			padRight(valueStyle.Render(trade.ticker), tickerCol) +
			padLeft(valueStyle.Render(fmt.Sprintf("%.2f", trade.shares)), sharesCol) +
			padLeft(fmtCurrency(trade.price), priceCol) +
			padLeft(fmtCurrency(trade.amount), amountCol)

		builder.WriteString("  " + line + "\n")
	}
}
