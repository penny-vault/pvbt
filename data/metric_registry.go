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

import "sort"

var metricRegistry = map[string]Metric{
	// EOD
	"MetricOpen": MetricOpen, "MetricHigh": MetricHigh, "MetricLow": MetricLow, "MetricClose": MetricClose,
	"AdjClose": AdjClose, "Volume": Volume, "Dividend": Dividend, "SplitFactor": SplitFactor,
	// Live
	"Price": Price, "Bid": Bid, "Ask": Ask,
	// Valuation
	"MarketCap": MarketCap, "EnterpriseValue": EnterpriseValue, "PE": PE, "PB": PB, "PS": PS,
	"EVtoEBIT": EVtoEBIT, "EVtoEBITDA": EVtoEBITDA,
	"ForwardPE": ForwardPE, "PEG": PEG, "PriceToCashFlow": PriceToCashFlow, "Beta": Beta,
	// Income statement
	"Revenue": Revenue, "CostOfRevenue": CostOfRevenue, "GrossProfit": GrossProfit,
	"OperatingExpenses": OperatingExpenses, "OperatingIncome": OperatingIncome,
	"EBIT": EBIT, "EBITDA": EBITDA, "EBT": EBT, "ConsolidatedIncome": ConsolidatedIncome,
	"NetIncome": NetIncome, "NetIncomeCommonStock": NetIncomeCommonStock,
	"EarningsPerShare": EarningsPerShare, "EPSDiluted": EPSDiluted,
	"InterestExpense": InterestExpense, "IncomeTaxExpense": IncomeTaxExpense,
	"RandDExpenses": RandDExpenses, "SGAExpense": SGAExpense,
	"ShareBasedCompensation": ShareBasedCompensation, "DividendsPerShare": DividendsPerShare,
	"NetLossIncomeDiscontinuedOperations": NetLossIncomeDiscontinuedOperations,
	"NetIncomeToNonControllingInterests":  NetIncomeToNonControllingInterests,
	"PreferredDividendsImpact":            PreferredDividendsImpact,
	// Balance sheet
	"TotalAssets": TotalAssets, "CurrentAssets": CurrentAssets, "AssetsNonCurrent": AssetsNonCurrent,
	"AverageAssets": AverageAssets, "CashAndEquivalents": CashAndEquivalents,
	"Inventory": Inventory, "Receivables": Receivables, "Investments": Investments,
	"InvestmentsCurrent": InvestmentsCurrent, "InvestmentsNonCurrent": InvestmentsNonCur,
	"Intangibles": Intangibles, "PPENet": PPENet, "TaxAssets": TaxAssets,
	"TotalLiabilities": TotalLiabilities, "CurrentLiabilities": CurrentLiabilities,
	"LiabilitiesNonCurrent": LiabilitiesNonCurrent, "TotalDebt": TotalDebt,
	"DebtCurrent": DebtCurrent, "DebtNonCurrent": DebtNonCurrent,
	"Payables": Payables, "DeferredRevenue": DeferredRevenue, "Deposits": Deposits,
	"TaxLiabilities": TaxLiabilities, "Equity": Equity, "EquityAvg": EquityAvg,
	"AccumulatedOtherComprehensiveIncome": AccumulatedOtherComprehensiveIncome,
	"AccumulatedRetainedEarningsDeficit":  AccumulatedRetainedEarningsDeficit,
	// Cash flow
	"FreeCashFlow": FreeCashFlow, "NetCashFlow": NetCashFlow,
	"NetCashFlowFromOperations": NetCashFlowFromOperations,
	"NetCashFlowFromInvesting":  NetCashFlowFromInvesting,
	"NetCashFlowFromFinancing":  NetCashFlowFromFinancing,
	"NetCashFlowBusiness":       NetCashFlowBusiness, "NetCashFlowCommon": NetCashFlowCommon,
	"NetCashFlowDebt": NetCashFlowDebt, "NetCashFlowDividend": NetCashFlowDividend,
	"NetCashFlowInvest": NetCashFlowInvest, "NetCashFlowFx": NetCashFlowFx,
	"CapitalExpenditure": CapitalExpenditure, "DepreciationAmortization": DepreciationAmortization,
	// Per-share and ratios
	"BookValue": BookValue, "FreeCashFlowPerShare": FreeCashFlowPerShare,
	"SalesPerShare": SalesPerShare, "TangibleAssetsBookValuePerShare": TangibleAssetsBookValuePerShare,
	"ShareFactor": ShareFactor, "SharesBasic": SharesBasic,
	"WeightedAverageShares": WeightedAverageShares, "WeightedAverageSharesDiluted": WeightedAverageSharesDiluted,
	"FundamentalPrice": FundamentalPrice, "PE1": PE1, "PS1": PS1, "FxUSD": FxUSD,
	// Margin and return ratios
	"GrossMargin": GrossMargin, "EBITDAMargin": EBITDAMargin, "ProfitMargin": ProfitMargin,
	"ROA": ROA, "ROE": ROE, "ROIC": ROIC, "ReturnOnSales": ReturnOnSales,
	"AssetTurnover": AssetTurnover, "CurrentRatio": CurrentRatio, "DebtToEquity": DebtToEquity,
	"DividendYield": DividendYield, "PayoutRatio": PayoutRatio,
	// Invested capital
	"InvestedCapital": InvestedCapital, "InvestedCapitalAvg": InvestedCapitalAvg,
	"TangibleAssetValue": TangibleAssetValue, "WorkingCapital": WorkingCapital,
	"MarketCapFundamental": MarketCapFundamental,
	// Economic
	"Unemployment": Unemployment,
}

// MetricByName looks up a Metric constant by its registry name.
func MetricByName(name string) (Metric, bool) {
	m, ok := metricRegistry[name]
	return m, ok
}

// AllMetricNames returns the sorted list of all registered metric names.
func AllMetricNames() []string {
	names := make([]string, 0, len(metricRegistry))
	for name := range metricRegistry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
