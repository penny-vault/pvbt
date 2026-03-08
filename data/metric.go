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

package data

// Metric identifies an externally-sourced measurement about an asset or the economy.
type Metric string

// End-of-day price metrics (eod table).
const (
	MetricOpen  Metric = "Open"
	MetricHigh  Metric = "High"
	MetricLow   Metric = "Low"
	MetricClose Metric = "Close"
	AdjClose    Metric = "AdjClose"
	Volume      Metric = "Volume"
	Dividend    Metric = "Dividend"
	SplitFactor Metric = "SplitFactor"
)

// Live/streaming price metrics.
const (
	Price Metric = "Price"
	Bid   Metric = "Bid"
	Ask   Metric = "Ask"
)

// Valuation metrics (metric table).
const (
	MarketCap       Metric = "MarketCap"
	EnterpriseValue Metric = "EnterpriseValue"
	PE              Metric = "PE"
	PB              Metric = "PB"
	PS              Metric = "PS"
	EVtoEBIT        Metric = "EVtoEBIT"
	EVtoEBITDA      Metric = "EVtoEBITDA"
	SP500           Metric = "SP500"
)

// Income statement metrics (fundamental table).
const (
	Revenue                Metric = "Revenue"
	CostOfRevenue          Metric = "CostOfRevenue"
	GrossProfit            Metric = "GrossProfit"
	OperatingExpenses      Metric = "OperatingExpenses"
	OperatingIncome        Metric = "OperatingIncome"
	EBIT                   Metric = "EBIT"
	EBITDA                 Metric = "EBITDA"
	EBT                    Metric = "EBT"
	ConsolidatedIncome     Metric = "ConsolidatedIncome"
	NetIncome              Metric = "NetIncome"
	NetIncomeCommonStock   Metric = "NetIncomeCommonStock"
	EarningsPerShare       Metric = "EarningsPerShare"
	EPSDiluted             Metric = "EPSDiluted"
	InterestExpense        Metric = "InterestExpense"
	IncomeTaxExpense       Metric = "IncomeTaxExpense"
	RandDExpenses          Metric = "RandDExpenses"
	SGAExpense             Metric = "SGAExpense"
	ShareBasedCompensation Metric = "ShareBasedCompensation"
	DividendsPerShare      Metric = "DividendsPerShare"

	NetLossIncomeDiscontinuedOperations Metric = "NetLossIncomeDiscontinuedOperations"
	NetIncomeToNonControllingInterests  Metric = "NetIncomeToNonControllingInterests"
	PreferredDividendsImpact            Metric = "PreferredDividendsImpact"
)

// Balance sheet metrics (fundamental table).
const (
	TotalAssets      Metric = "TotalAssets"
	CurrentAssets    Metric = "CurrentAssets"
	AssetsNonCurrent Metric = "AssetsNonCurrent"
	AverageAssets    Metric = "AverageAssets"

	CashAndEquivalents Metric = "CashAndEquivalents"
	Inventory          Metric = "Inventory"
	Receivables        Metric = "Receivables"
	Investments        Metric = "Investments"
	InvestmentsCurrent Metric = "InvestmentsCurrent"
	InvestmentsNonCur  Metric = "InvestmentsNonCurrent"
	Intangibles        Metric = "Intangibles"
	PPENet             Metric = "PPENet"
	TaxAssets          Metric = "TaxAssets"

	TotalLiabilities      Metric = "TotalLiabilities"
	CurrentLiabilities    Metric = "CurrentLiabilities"
	LiabilitiesNonCurrent Metric = "LiabilitiesNonCurrent"
	TotalDebt             Metric = "TotalDebt"
	DebtCurrent           Metric = "DebtCurrent"
	DebtNonCurrent        Metric = "DebtNonCurrent"
	Payables              Metric = "Payables"
	DeferredRevenue       Metric = "DeferredRevenue"
	Deposits              Metric = "Deposits"
	TaxLiabilities        Metric = "TaxLiabilities"

	Equity                                Metric = "Equity"
	EquityAvg                             Metric = "EquityAvg"
	AccumulatedOtherComprehensiveIncome   Metric = "AccumulatedOtherComprehensiveIncome"
	AccumulatedRetainedEarningsDeficit    Metric = "AccumulatedRetainedEarningsDeficit"
)

// Cash flow metrics (fundamental table).
const (
	FreeCashFlow             Metric = "FreeCashFlow"
	NetCashFlow              Metric = "NetCashFlow"
	NetCashFlowFromOperations Metric = "NetCashFlowFromOperations"
	NetCashFlowFromInvesting  Metric = "NetCashFlowFromInvesting"
	NetCashFlowFromFinancing  Metric = "NetCashFlowFromFinancing"
	NetCashFlowBusiness      Metric = "NetCashFlowBusiness"
	NetCashFlowCommon        Metric = "NetCashFlowCommon"
	NetCashFlowDebt          Metric = "NetCashFlowDebt"
	NetCashFlowDividend      Metric = "NetCashFlowDividend"
	NetCashFlowInvest        Metric = "NetCashFlowInvest"
	NetCashFlowFx            Metric = "NetCashFlowFx"
	CapitalExpenditure       Metric = "CapitalExpenditure"
	DepreciationAmortization Metric = "DepreciationAmortization"
)

// Per-share and ratio metrics (fundamental table).
const (
	BookValue                       Metric = "BookValue"
	FreeCashFlowPerShare            Metric = "FreeCashFlowPerShare"
	SalesPerShare                   Metric = "SalesPerShare"
	TangibleAssetsBookValuePerShare Metric = "TangibleAssetsBookValuePerShare"
	ShareFactor                     Metric = "ShareFactor"
	SharesBasic                     Metric = "SharesBasic"
	WeightedAverageShares           Metric = "WeightedAverageShares"
	WeightedAverageSharesDiluted    Metric = "WeightedAverageSharesDiluted"
	FundamentalPrice                Metric = "FundamentalPrice"
	PE1                             Metric = "PE1"
	PS1                             Metric = "PS1"
	FxUSD                           Metric = "FxUSD"
)

// Margin and return ratios (fundamental table).
const (
	GrossMargin    Metric = "GrossMargin"
	EBITDAMargin   Metric = "EBITDAMargin"
	ProfitMargin   Metric = "ProfitMargin"
	ROA            Metric = "ROA"
	ROE            Metric = "ROE"
	ROIC           Metric = "ROIC"
	ReturnOnSales  Metric = "ReturnOnSales"
	AssetTurnover  Metric = "AssetTurnover"
	CurrentRatio   Metric = "CurrentRatio"
	DebtToEquity   Metric = "DebtToEquity"
	DividendYield  Metric = "DividendYield"
	PayoutRatio    Metric = "PayoutRatio"
)

// Invested capital metrics (fundamental table).
const (
	InvestedCapital    Metric = "InvestedCapital"
	InvestedCapitalAvg Metric = "InvestedCapitalAvg"
	TangibleAssetValue Metric = "TangibleAssetValue"
	WorkingCapital     Metric = "WorkingCapital"
	MarketCapFundamental Metric = "MarketCapFundamental"
)

// Economic indicator metrics.
const (
	Unemployment Metric = "Unemployment"
)
