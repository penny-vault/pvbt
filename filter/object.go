package filter

import (
	"main/portfolio"
	"math"
	"time"
)

type FilterObject struct {
	Performance portfolio.Performance
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
	case "twrr_1wk":
		return float64(m.TWRROneWeek)
	case "twrr_1mo":
		return float64(m.TWRROneMonth)
	case "twrr_3mo":
		return float64(m.TWRRThreeMonth)
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
	case "mwrr_1wk":
		return float64(m.MWRROneWeek)
	case "mwrr_1mo":
		return float64(m.MWRROneMonth)
	case "mwrr_3mo":
		return float64(m.MWRRThreeMonth)
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
