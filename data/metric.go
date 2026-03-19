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
	// MetricOpen is the opening price for the trading day.
	MetricOpen Metric = "Open"
	// MetricHigh is the highest price reached during the trading day.
	MetricHigh Metric = "High"
	// MetricLow is the lowest price reached during the trading day.
	MetricLow Metric = "Low"
	// MetricClose is the closing price for the trading day.
	MetricClose Metric = "Close"
	// AdjClose is the closing price adjusted for splits and dividends.
	AdjClose Metric = "AdjClose"
	// Volume is the number of shares traded during the trading day.
	Volume Metric = "Volume"
	// Dividend is the per-share cash dividend paid on the ex-date.
	Dividend Metric = "Dividend"
	// SplitFactor is the ratio applied to shares on the split effective date (e.g. 2.0 for a 2-for-1 split).
	SplitFactor Metric = "SplitFactor"
)

// Live/streaming price metrics.
const (
	// Price is the last-traded price from a live/streaming feed.
	Price Metric = "Price"
	// Bid is the highest current bid price from a live/streaming feed.
	Bid Metric = "Bid"
	// Ask is the lowest current ask price from a live/streaming feed.
	Ask Metric = "Ask"
)

// Valuation metrics (metrics table).
const (
	// MarketCap is the total market capitalization (share price times shares outstanding).
	MarketCap Metric = "MarketCap"
	// EnterpriseValue is market cap plus debt minus cash, representing the total value of the business.
	EnterpriseValue Metric = "EnterpriseValue"
	// PE is the trailing price-to-earnings ratio (price divided by trailing-twelve-month EPS).
	PE Metric = "PE"
	// ForwardPE is the forward price-to-earnings ratio (price divided by consensus forward EPS estimate).
	ForwardPE Metric = "ForwardPE"
	// PEG is the price/earnings-to-growth ratio (PE divided by expected earnings growth rate).
	PEG Metric = "PEG"
	// PB is the price-to-book ratio (price divided by book value per share).
	PB Metric = "PB"
	// PS is the price-to-sales ratio (price divided by revenue per share).
	PS Metric = "PS"
	// PriceToCashFlow is the price-to-cash-flow ratio (price divided by operating cash flow per share).
	PriceToCashFlow Metric = "PriceToCashFlow"
	// EVtoEBIT is the enterprise-value-to-EBIT ratio.
	EVtoEBIT Metric = "EVtoEBIT"
	// EVtoEBITDA is the enterprise-value-to-EBITDA ratio.
	EVtoEBITDA Metric = "EVtoEBITDA"
	// Beta measures the asset's sensitivity to market movements (slope of returns vs. market returns).
	Beta Metric = "Beta"
)

// Income statement metrics (fundamentals table).
const (
	// Revenue is total top-line sales for the period.
	Revenue Metric = "Revenue"
	// CostOfRevenue is the direct costs attributable to producing goods or services sold.
	CostOfRevenue Metric = "CostOfRevenue"
	// GrossProfit is revenue minus cost of revenue.
	GrossProfit Metric = "GrossProfit"
	// OperatingExpenses is total operating costs excluding cost of revenue (includes SGA, R&D, etc.).
	OperatingExpenses Metric = "OperatingExpenses"
	// OperatingIncome is gross profit minus operating expenses.
	OperatingIncome Metric = "OperatingIncome"
	// EBIT is earnings before interest and taxes.
	EBIT Metric = "EBIT"
	// EBITDA is earnings before interest, taxes, depreciation, and amortization.
	EBITDA Metric = "EBITDA"
	// EBT is earnings before taxes (net income before income tax provision).
	EBT Metric = "EBT"
	// ConsolidatedIncome is net income including all consolidated subsidiaries.
	ConsolidatedIncome Metric = "ConsolidatedIncome"
	// NetIncome is the bottom-line profit after all expenses, taxes, and adjustments.
	NetIncome Metric = "NetIncome"
	// NetIncomeCommonStock is net income available to common stockholders (after preferred dividends).
	NetIncomeCommonStock Metric = "NetIncomeCommonStock"
	// EarningsPerShare is basic earnings per share (net income divided by basic shares outstanding).
	EarningsPerShare Metric = "EarningsPerShare"
	// EPSDiluted is diluted earnings per share (net income divided by diluted shares outstanding).
	EPSDiluted Metric = "EPSDiluted"
	// InterestExpense is the cost of borrowed funds for the period.
	InterestExpense Metric = "InterestExpense"
	// IncomeTaxExpense is the total income tax provision for the period.
	IncomeTaxExpense Metric = "IncomeTaxExpense"
	// RandDExpenses is research and development expenditures for the period.
	RandDExpenses Metric = "RandDExpenses"
	// SGAExpense is selling, general, and administrative expenses for the period.
	SGAExpense Metric = "SGAExpense"
	// ShareBasedCompensation is the non-cash expense recognized for equity-based employee compensation.
	ShareBasedCompensation Metric = "ShareBasedCompensation"
	// DividendsPerShare is the total dividends declared per basic common share.
	DividendsPerShare Metric = "DividendsPerShare"

	// NetLossIncomeDiscontinuedOperations is the gain or loss from discontinued business segments.
	NetLossIncomeDiscontinuedOperations Metric = "NetLossIncomeDiscontinuedOperations"
	// NetIncomeToNonControllingInterests is the portion of net income allocated to minority/non-controlling interests.
	NetIncomeToNonControllingInterests Metric = "NetIncomeToNonControllingInterests"
	// PreferredDividendsImpact is the income statement impact of preferred stock dividends.
	PreferredDividendsImpact Metric = "PreferredDividendsImpact"
)

// Balance sheet metrics (fundamentals table).
const (
	// TotalAssets is the sum of all current and non-current assets.
	TotalAssets Metric = "TotalAssets"
	// CurrentAssets is the total assets expected to be converted to cash within one year.
	CurrentAssets Metric = "CurrentAssets"
	// AssetsNonCurrent is the total long-term assets not expected to be converted to cash within one year.
	AssetsNonCurrent Metric = "AssetsNonCurrent"
	// AverageAssets is the average of beginning and ending total assets for the period.
	AverageAssets Metric = "AverageAssets"

	// CashAndEquivalents is cash on hand plus short-term highly liquid investments.
	CashAndEquivalents Metric = "CashAndEquivalents"
	// Inventory is the value of raw materials, work-in-progress, and finished goods held for sale.
	Inventory Metric = "Inventory"
	// Receivables is amounts owed by customers for goods or services delivered.
	Receivables Metric = "Receivables"
	// Investments is the total value of investment securities (current and non-current combined).
	Investments Metric = "Investments"
	// InvestmentsCurrent is the value of investment securities maturing within one year.
	InvestmentsCurrent Metric = "InvestmentsCurrent"
	// InvestmentsNonCur is the value of long-term investment securities maturing beyond one year.
	InvestmentsNonCur Metric = "InvestmentsNonCurrent"
	// Intangibles is the value of non-physical assets such as goodwill, patents, and trademarks.
	Intangibles Metric = "Intangibles"
	// PPENet is property, plant, and equipment net of accumulated depreciation.
	PPENet Metric = "PPENet"
	// TaxAssets is deferred and other tax assets on the balance sheet.
	TaxAssets Metric = "TaxAssets"

	// TotalLiabilities is the sum of all current and non-current liabilities.
	TotalLiabilities Metric = "TotalLiabilities"
	// CurrentLiabilities is obligations due within one year.
	CurrentLiabilities Metric = "CurrentLiabilities"
	// LiabilitiesNonCurrent is obligations due beyond one year.
	LiabilitiesNonCurrent Metric = "LiabilitiesNonCurrent"
	// TotalDebt is the sum of all short-term and long-term borrowings.
	TotalDebt Metric = "TotalDebt"
	// DebtCurrent is borrowings due within one year.
	DebtCurrent Metric = "DebtCurrent"
	// DebtNonCurrent is borrowings due beyond one year.
	DebtNonCurrent Metric = "DebtNonCurrent"
	// Payables is amounts owed to suppliers for goods or services received.
	Payables Metric = "Payables"
	// DeferredRevenue is payments received for goods or services not yet delivered.
	DeferredRevenue Metric = "DeferredRevenue"
	// Deposits is customer or counterparty deposits held (primarily for banks and financial institutions).
	Deposits Metric = "Deposits"
	// TaxLiabilities is deferred and other tax liabilities on the balance sheet.
	TaxLiabilities Metric = "TaxLiabilities"

	// Equity is total shareholders' equity (assets minus liabilities).
	Equity Metric = "Equity"
	// EquityAvg is the average of beginning and ending shareholders' equity for the period.
	EquityAvg Metric = "EquityAvg"
	// AccumulatedOtherComprehensiveIncome is unrealized gains/losses not yet recognized in net income (e.g. foreign currency, securities).
	AccumulatedOtherComprehensiveIncome Metric = "AccumulatedOtherComprehensiveIncome"
	// AccumulatedRetainedEarningsDeficit is cumulative net income retained in the business since inception.
	AccumulatedRetainedEarningsDeficit Metric = "AccumulatedRetainedEarningsDeficit"
)

// Cash flow metrics (fundamentals table).
const (
	// FreeCashFlow is operating cash flow minus capital expenditures.
	FreeCashFlow Metric = "FreeCashFlow"
	// NetCashFlow is the net change in cash and equivalents for the period.
	NetCashFlow Metric = "NetCashFlow"
	// NetCashFlowFromOperations is cash generated or consumed by core business operations.
	NetCashFlowFromOperations Metric = "NetCashFlowFromOperations"
	// NetCashFlowFromInvesting is cash used for or generated by investing activities (capex, acquisitions, etc.).
	NetCashFlowFromInvesting Metric = "NetCashFlowFromInvesting"
	// NetCashFlowFromFinancing is cash from or used for financing activities (debt issuance, buybacks, dividends).
	NetCashFlowFromFinancing Metric = "NetCashFlowFromFinancing"
	// NetCashFlowBusiness is cash flow from business acquisitions and disposals.
	NetCashFlowBusiness Metric = "NetCashFlowBusiness"
	// NetCashFlowCommon is cash flow from issuance or repurchase of common stock.
	NetCashFlowCommon Metric = "NetCashFlowCommon"
	// NetCashFlowDebt is cash flow from issuance or repayment of debt.
	NetCashFlowDebt Metric = "NetCashFlowDebt"
	// NetCashFlowDividend is cash paid out as dividends to shareholders.
	NetCashFlowDividend Metric = "NetCashFlowDividend"
	// NetCashFlowInvest is cash flow from purchases and sales of investment securities.
	NetCashFlowInvest Metric = "NetCashFlowInvest"
	// NetCashFlowFx is the effect of foreign exchange rate changes on cash.
	NetCashFlowFx Metric = "NetCashFlowFx"
	// CapitalExpenditure is cash spent on acquiring or maintaining fixed assets.
	CapitalExpenditure Metric = "CapitalExpenditure"
	// DepreciationAmortization is the non-cash reduction in value of tangible and intangible assets.
	DepreciationAmortization Metric = "DepreciationAmortization"
)

// Per-share and ratio metrics (fundamentals table).
const (
	// BookValue is book value per share (total equity divided by shares outstanding).
	BookValue Metric = "BookValue"
	// FreeCashFlowPerShare is free cash flow divided by shares outstanding.
	FreeCashFlowPerShare Metric = "FreeCashFlowPerShare"
	// SalesPerShare is total revenue divided by shares outstanding.
	SalesPerShare Metric = "SalesPerShare"
	// TangibleAssetsBookValuePerShare is tangible book value (equity minus intangibles) divided by shares outstanding.
	TangibleAssetsBookValuePerShare Metric = "TangibleAssetsBookValuePerShare"
	// ShareFactor is the cumulative split-adjustment factor for the period.
	ShareFactor Metric = "ShareFactor"
	// SharesBasic is the basic (non-diluted) number of shares outstanding.
	SharesBasic Metric = "SharesBasic"
	// WeightedAverageShares is the weighted average basic shares outstanding over the period.
	WeightedAverageShares Metric = "WeightedAverageShares"
	// WeightedAverageSharesDiluted is the weighted average diluted shares outstanding over the period.
	WeightedAverageSharesDiluted Metric = "WeightedAverageSharesDiluted"
	// FundamentalPrice is the closing share price as reported in the fundamental data source.
	FundamentalPrice Metric = "FundamentalPrice"
	// PE1 is an alternative price-to-earnings calculation from the fundamental data source.
	PE1 Metric = "PE1"
	// PS1 is an alternative price-to-sales calculation from the fundamental data source.
	PS1 Metric = "PS1"
	// FxUSD is the foreign exchange rate to US dollars used in financial statement conversion.
	FxUSD Metric = "FxUSD"
)

// Margin and return ratios (fundamentals table).
const (
	// GrossMargin is gross profit divided by revenue, expressed as a ratio.
	GrossMargin Metric = "GrossMargin"
	// EBITDAMargin is EBITDA divided by revenue, expressed as a ratio.
	EBITDAMargin Metric = "EBITDAMargin"
	// ProfitMargin is net income divided by revenue, expressed as a ratio.
	ProfitMargin Metric = "ProfitMargin"
	// ROA is return on assets (net income divided by average total assets).
	ROA Metric = "ROA"
	// ROE is return on equity (net income divided by average shareholders' equity).
	ROE Metric = "ROE"
	// ROIC is return on invested capital (NOPAT divided by average invested capital).
	ROIC Metric = "ROIC"
	// ReturnOnSales is operating income divided by revenue, expressed as a ratio.
	ReturnOnSales Metric = "ReturnOnSales"
	// AssetTurnover is revenue divided by average total assets.
	AssetTurnover Metric = "AssetTurnover"
	// CurrentRatio is current assets divided by current liabilities.
	CurrentRatio Metric = "CurrentRatio"
	// DebtToEquity is total debt divided by total shareholders' equity.
	DebtToEquity Metric = "DebtToEquity"
	// DividendYield is annual dividends per share divided by share price, expressed as a ratio.
	DividendYield Metric = "DividendYield"
	// PayoutRatio is dividends paid divided by net income, expressed as a ratio.
	PayoutRatio Metric = "PayoutRatio"
)

// Invested capital metrics (fundamentals table).
const (
	// InvestedCapital is total debt plus equity minus cash and equivalents.
	InvestedCapital Metric = "InvestedCapital"
	// InvestedCapitalAvg is the average of beginning and ending invested capital for the period.
	InvestedCapitalAvg Metric = "InvestedCapitalAvg"
	// TangibleAssetValue is total assets minus intangible assets and goodwill.
	TangibleAssetValue Metric = "TangibleAssetValue"
	// WorkingCapital is current assets minus current liabilities.
	WorkingCapital Metric = "WorkingCapital"
	// MarketCapFundamental is market capitalization as reported in the fundamental data source.
	MarketCapFundamental Metric = "MarketCapFundamental"
)

// Economic indicator metrics.
const (
	// Unemployment is the civilian unemployment rate.
	Unemployment Metric = "Unemployment"
)

// Computed aggregate metrics.
const (
	// Count is the metric used by CountWhere to store per-timestep counts.
	Count Metric = "Count"
)

// Portfolio performance tracking metrics.
const (
	// PortfolioEquity is the total equity value of the portfolio at each time step.
	PortfolioEquity Metric = "PortfolioEquity"
	// PortfolioBenchmark is the benchmark index value used for portfolio comparison.
	PortfolioBenchmark Metric = "PortfolioBenchmark"
	// PortfolioRiskFree is the risk-free rate used for risk-adjusted return calculations.
	PortfolioRiskFree Metric = "PortfolioRiskFree"
)
