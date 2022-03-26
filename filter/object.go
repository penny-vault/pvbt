// Copyright 2021 JD Fergason
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

package filter

import (
	"main/portfolio"
	"math"
	"time"
)

type FilterObject struct {
	Portfolio   *portfolio.Portfolio
	Performance *portfolio.Performance
}

func getValue(m *portfolio.PerformanceMeasurement, field string) float64 {
	switch field {
	case "alpha_1yr":
		return float64(m.AlphaOneYear)
	case "alpha_3yr":
		return float64(m.AlphaThreeYear)
	case "alpha_5yr":
		return float64(m.AlphaFiveYear)
	case "alpha_10yr":
		return float64(m.AlphaTenYear)
	case "beta_1yr":
		return float64(m.BetaOneYear)
	case "beta_3yr":
		return float64(m.BetaThreeYear)
	case "beta_5yr":
		return float64(m.BetaFiveYear)
	case "beta_10yr":
		return float64(m.BetaTenYear)
	case "twrr_1d":
		return float64(m.TWRROneDay)
	case "twrr_wtd":
		return float64(m.TWRRWeekToDate)
	case "twrr_1wk":
		return float64(m.TWRROneWeek)
	case "twrr_mtd":
		return float64(m.TWRRMonthToDate)
	case "twrr_1mo":
		return float64(m.TWRROneMonth)
	case "twrr_3mo":
		return float64(m.TWRRThreeMonth)
	case "twrr_ytd":
		return float64(m.TWRRYearToDate)
	case "twrr_1yr":
		return float64(m.TWRROneYear)
	case "twrr_3yr":
		return float64(m.TWRRThreeYear)
	case "twrr_5yr":
		return float64(m.TWRRFiveYear)
	case "twrr_10yr":
		return float64(m.TWRRTenYear)
	case "mwrr_1d":
		return float64(m.MWRROneDay)
	case "mwrr_wtd":
		return float64(m.MWRRWeekToDate)
	case "mwrr_1wk":
		return float64(m.MWRROneWeek)
	case "mwrr_mtd":
		return float64(m.MWRRMonthToDate)
	case "mwrr_1mo":
		return float64(m.MWRROneMonth)
	case "mwrr_3mo":
		return float64(m.MWRRThreeMonth)
	case "mwrr_ytd":
		return float64(m.MWRRYearToDate)
	case "mwrr_1yr":
		return float64(m.MWRROneYear)
	case "mwrr_3yr":
		return float64(m.MWRRThreeYear)
	case "mwrr_5yr":
		return float64(m.MWRRFiveYear)
	case "mwrr_10yr":
		return float64(m.MWRRTenYear)
	case "active_return_1yr":
		return float64(m.ActiveReturnOneYear)
	case "active_return_3yr":
		return float64(m.ActiveReturnThreeYear)
	case "active_return_5yr":
		return float64(m.ActiveReturnFiveYear)
	case "active_return_10yr":
		return float64(m.ActiveReturnTenYear)
	case "calmar_ratio":
		return float64(m.CalmarRatio)
	case "downside_deviation":
		return float64(m.DownsideDeviation)
	case "information_ratio":
		return float64(m.InformationRatio)
	case "k_ratio":
		return float64(m.KRatio)
	case "keller_ratio":
		return float64(m.KellerRatio)
	case "sharpe_ratio":
		return float64(m.SharpeRatio)
	case "sortino_ratio":
		return float64(m.SortinoRatio)
	case "std_dev":
		return float64(m.StdDev)
	case "treynor_ratio":
		return float64(m.TreynorRatio)
	case "ulcer_index":
		return float64(m.UlcerIndex)
	case "strategy_value":
		return m.Value
	case "risk_free_value":
		return m.RiskFreeValue
	case "total_deposited_to_date":
		return m.TotalDeposited
	case "total_withdrawn_to_date":
		return m.TotalWithdrawn
	case "benchmark_value":
		return m.BenchmarkValue
	case "strategy_growth_of_10k":
		return m.StrategyGrowthOf10K
	case "benchmark_growth_of_10k":
		return m.BenchmarkGrowthOf10K
	case "risk_free_growth_of_10k":
		return m.RiskFreeGrowthOf10K
	default:
		return math.NaN()
	}
}

func (f *FilterObject) GetMeasurements(field1 string, field2 string, since time.Time) ([]byte, error) {
	fields := []string{field1, field2}

	// filter measurements by where
	filtered := make([]*portfolio.PerformanceMeasurement, 0, len(f.Performance.Measurements))
	for _, meas := range f.Performance.Measurements {
		if meas.Time.After(since) {
			filtered = append(filtered, meas)
		}
	}

	meas := portfolio.PerformanceMeasurementItemList{
		FieldNames: fields,
		Items:      make([]*portfolio.PerformanceMeasurementItem, len(filtered)),
	}

	for idx, xx := range filtered {
		meas.Items[idx] = &portfolio.PerformanceMeasurementItem{
			Time:   xx.Time,
			Value1: getValue(xx, field1),
			Value2: getValue(xx, field2),
		}
	}
	return meas.MarshalBinary()
}

func (f *FilterObject) GetHoldings(frequency string, since time.Time) ([]byte, error) {
	var periodReturn string
	switch frequency {
	case "annually":
		periodReturn = "twrr_ytd"
	case "monthly":
		periodReturn = "twrr_mtd"
	case "weekly":
		periodReturn = "twrr_wtd"
	case "daily":
		periodReturn = "twrr_1d"
	default:
		periodReturn = "twrr_mtd"
	}

	holdingsAtPeriodStart := make([]*portfolio.ReportableHolding, 0)
	justificationAtPeriodStart := make([]*portfolio.Justification, 0)

	holdings := portfolio.PortfolioHoldingItemList{
		Items: make([]*portfolio.PortfolioHoldingItem, 0, len(f.Performance.Measurements)),
	}

	// filter measurements by where
	var last *portfolio.PerformanceMeasurement
	added := false
	for _, meas := range f.Performance.Measurements {
		added = false
		switch frequency {
		case "annually":
			if last == nil {
				if meas.Time.After(since) {
					holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
						Time:          meas.Time,
						Holdings:      meas.Holdings,
						Justification: meas.Justification,
						PercentReturn: getValue(meas, periodReturn),
						Value:         meas.Value,
					})
					added = true
				}
			} else if last.Time.Year() != meas.Time.Year() && meas.Time.After(since) {
				holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
					Time:          last.Time,
					Holdings:      holdingsAtPeriodStart,
					Justification: justificationAtPeriodStart,
					PercentReturn: getValue(last, periodReturn),
					Value:         last.Value,
				})
				holdingsAtPeriodStart = meas.Holdings
				justificationAtPeriodStart = last.Justification
				added = true
			}
		case "monthly":
			if last != nil && meas.Time.Month() != last.Time.Month() && meas.Time.After(since) {
				holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
					Time:          last.Time,
					Holdings:      holdingsAtPeriodStart,
					Justification: justificationAtPeriodStart,
					PercentReturn: getValue(last, periodReturn),
					Value:         last.Value,
				})

				holdingsAtPeriodStart = meas.Holdings
				justificationAtPeriodStart = last.Justification

				added = true
			}
		case "weekly":
			if last != nil && meas.Time.Weekday() < last.Time.Weekday() && meas.Time.After(since) {
				holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
					Time:          last.Time,
					Holdings:      holdingsAtPeriodStart,
					Justification: justificationAtPeriodStart,
					PercentReturn: getValue(last, periodReturn),
					Value:         last.Value,
				})

				holdingsAtPeriodStart = meas.Holdings
				justificationAtPeriodStart = last.Justification

				added = true
			}
		case "daily":
			if meas.Time.After(since) {
				holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
					Time:          meas.Time,
					Holdings:      meas.Holdings,
					Justification: meas.Justification,
					PercentReturn: getValue(meas, periodReturn),
					Value:         meas.Value,
				})
				added = true
			}
		default: // monthly
			if last != nil && meas.Time.Month() != last.Time.Month() && meas.Time.After(since) {
				holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
					Time:          last.Time,
					Holdings:      holdingsAtPeriodStart,
					Justification: justificationAtPeriodStart,
					PercentReturn: getValue(last, periodReturn),
					Value:         last.Value,
				})

				holdingsAtPeriodStart = meas.Holdings
				justificationAtPeriodStart = last.Justification

				added = true
			}
		}
		last = meas
	}

	// add the last measurement if it wasn't already added
	if !added {
		holdings.Items = append(holdings.Items, &portfolio.PortfolioHoldingItem{
			Time:          last.Time,
			Holdings:      holdingsAtPeriodStart,
			Justification: justificationAtPeriodStart,
			PercentReturn: getValue(last, periodReturn),
			Value:         last.Value,
		})
	}

	// add predicted holding item
	predicted := f.Portfolio.PredictedAssets
	switch frequency {
	case "annually":
		nyc, _ := time.LoadLocation("America/New_York")
		predicted.Time = time.Date(predicted.Time.Year()+1, predicted.Time.Month(), 1, 16, 0, 0, 0, nyc)
	case "monthly":
		nyc, _ := time.LoadLocation("America/New_York")
		predicted.Time = time.Date(predicted.Time.Year(), predicted.Time.Month()+1, 1, 16, 0, 0, 0, nyc)
	}

	holdings.Items = append(holdings.Items, predicted)

	return holdings.MarshalBinary()
}

func (f *FilterObject) GetTransactions(since time.Time) ([]byte, error) {
	// filter transactions
	filtered := make([]*portfolio.Transaction, 0, len(f.Portfolio.Transactions))
	for _, xx := range f.Portfolio.Transactions {
		if xx.Date.After(since) || xx.Date.Equal(since) {
			filtered = append(filtered, xx)
		}
	}

	trx := portfolio.PortfolioTransactionList{
		Items: filtered,
	}

	return trx.MarshalBinary()
}
