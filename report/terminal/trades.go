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

package terminal

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/report"
)

const maxTradesShown = 10

// renderTrades writes trade summary stats and the trade log.
func renderTrades(builder *strings.Builder, trades report.Trades) {
	builder.WriteString(sectionTitleStyle.Render("Trade Summary"))
	builder.WriteString("\n")

	// Summary stats in 2-column layout.
	const summaryLabel = 20
	const summaryVal = 14

	type statPair struct {
		leftLabel  string
		leftValue  string
		rightLabel string
		rightValue string
	}

	pairs := []statPair{
		{
			leftLabel:  "Win Rate",
			leftValue:  fmtPct(trades.WinRate),
			rightLabel: "Avg Holding",
			rightValue: fmtDays(trades.AvgHolding),
		},
		{
			leftLabel:  "Avg Win",
			leftValue:  fmtCurrency(trades.AvgWin),
			rightLabel: "Avg Loss",
			rightValue: fmtCurrency(trades.AvgLoss),
		},
		{
			leftLabel:  "Profit Factor",
			leftValue:  fmtRatio(trades.ProfitFactor),
			rightLabel: "Gain/Loss",
			rightValue: fmtRatio(trades.GainLossRatio),
		},
		{
			leftLabel:  "Turnover",
			leftValue:  fmtPct(trades.Turnover),
			rightLabel: "Positive Periods",
			rightValue: fmtPct(trades.PositivePeriods),
		},
	}

	for _, pair := range pairs {
		left := padRight(labelStyle.Render(pair.leftLabel), summaryLabel) + padLeft(pair.leftValue, summaryVal)
		right := padRight(labelStyle.Render(pair.rightLabel), summaryLabel) + padLeft(pair.rightValue, summaryVal)
		builder.WriteString("  " + left + "  " + right + "\n")
	}

	// Trade log.
	if len(trades.Trades) == 0 {
		return
	}

	builder.WriteString("\n")
	builder.WriteString(sectionTitleStyle.Render("Recent Trades"))
	builder.WriteString("\n")

	// Column widths for trade table.
	const dateCol = 12
	const actionCol = 8
	const tickerCol = 8
	const sharesCol = 10
	const priceCol = 12
	const amountCol = 14

	// Header.
	header := padRight(tableHeaderStyle.Render("Date"), dateCol) +
		padRight(tableHeaderStyle.Render("Action"), actionCol) +
		padRight(tableHeaderStyle.Render("Ticker"), tickerCol) +
		padLeft(tableHeaderStyle.Render("Shares"), sharesCol) +
		padLeft(tableHeaderStyle.Render("Price"), priceCol) +
		padLeft(tableHeaderStyle.Render("Amount"), amountCol)

	builder.WriteString("  " + header + "\n")

	// Show the last N trades.
	startIdx := 0
	truncated := false

	if len(trades.Trades) > maxTradesShown {
		startIdx = len(trades.Trades) - maxTradesShown
		truncated = true
	}

	if truncated {
		earlier := len(trades.Trades) - maxTradesShown
		builder.WriteString("  " + dimStyle.Render(fmt.Sprintf("... and %d earlier transactions", earlier)) + "\n")
	}

	for idx := startIdx; idx < len(trades.Trades); idx++ {
		trade := trades.Trades[idx]

		actionStyled := valueStyle.Render(trade.Action)
		if trade.Action == "BUY" {
			actionStyled = positiveStyle.Render(trade.Action)
		} else if trade.Action == "SELL" {
			actionStyled = negativeStyle.Render(trade.Action)
		}

		line := padRight(dimStyle.Render(trade.Date.Format("2006-01-02")), dateCol) +
			padRight(actionStyled, actionCol) +
			padRight(valueStyle.Render(trade.Ticker), tickerCol) +
			padLeft(valueStyle.Render(fmt.Sprintf("%.2f", trade.Shares)), sharesCol) +
			padLeft(fmtCurrency(trade.Price), priceCol) +
			padLeft(fmtCurrency(trade.Amount), amountCol)

		builder.WriteString("  " + line + "\n")
	}
}
