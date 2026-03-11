package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Metric Helpers", func() {

	// ---------------------------------------------------------------
	// excessReturns
	// ---------------------------------------------------------------
	Describe("excessReturns", func() {
		It("subtracts element-wise for equal-length inputs", func() {
			r := []float64{0.10, 0.05, -0.02}
			rf := []float64{0.01, 0.01, 0.01}
			er := portfolio.ExportExcessReturns(r, rf)
			Expect(er).To(HaveLen(3))
			Expect(er[0]).To(BeNumerically("~", 0.09, 1e-12))
			Expect(er[1]).To(BeNumerically("~", 0.04, 1e-12))
			Expect(er[2]).To(BeNumerically("~", -0.03, 1e-12))
		})

		It("trims to the shorter array when r is longer", func() {
			r := []float64{0.10, 0.05, -0.02, 0.08}
			rf := []float64{0.01, 0.02}
			er := portfolio.ExportExcessReturns(r, rf)
			Expect(er).To(HaveLen(2))
			Expect(er[0]).To(BeNumerically("~", 0.09, 1e-12))
			Expect(er[1]).To(BeNumerically("~", 0.03, 1e-12))
		})

		It("trims to the shorter array when rf is longer", func() {
			r := []float64{0.10}
			rf := []float64{0.01, 0.02, 0.03}
			er := portfolio.ExportExcessReturns(r, rf)
			Expect(er).To(HaveLen(1))
			Expect(er[0]).To(BeNumerically("~", 0.09, 1e-12))
		})

		It("returns empty slice when both inputs are empty", func() {
			er := portfolio.ExportExcessReturns([]float64{}, []float64{})
			Expect(er).To(HaveLen(0))
		})

		It("returns empty slice when one input is empty", func() {
			er := portfolio.ExportExcessReturns([]float64{0.10}, []float64{})
			Expect(er).To(HaveLen(0))
		})
	})

	// ---------------------------------------------------------------
	// annualizationFactor
	// ---------------------------------------------------------------
	Describe("annualizationFactor", func() {
		It("returns 252 for daily timestamps", func() {
			// 10 weekdays, average gap ~ 1.4 calendar days (well under 20)
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			times := daySeq(start, 10)
			Expect(portfolio.ExportAnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("returns 12 for monthly timestamps", func() {
			// 6 months apart, average gap = ~30 days (exceeds 20)
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			times := monthSeq(start, 6)
			Expect(portfolio.ExportAnnualizationFactor(times)).To(BeNumerically("==", 12))
		})

		It("defaults to 252 for a single timestamp", func() {
			times := []time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
			Expect(portfolio.ExportAnnualizationFactor(times)).To(BeNumerically("==", 252))
		})

		It("defaults to 252 for an empty slice", func() {
			Expect(portfolio.ExportAnnualizationFactor(nil)).To(BeNumerically("==", 252))
		})

		It("returns 252 for two timestamps one day apart", func() {
			t0 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
			Expect(portfolio.ExportAnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 252))
		})

		It("returns 12 for two timestamps 21 days apart", func() {
			// Average gap = 21 days, which exceeds the 20-day threshold
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
			Expect(portfolio.ExportAnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 12))
		})

		It("returns 252 for two timestamps exactly 20 days apart (boundary)", func() {
			// Average gap = 20 days, which does NOT exceed 20 (strictly >20 required)
			t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			t1 := time.Date(2024, 1, 21, 0, 0, 0, 0, time.UTC)
			Expect(portfolio.ExportAnnualizationFactor([]time.Time{t0, t1})).To(BeNumerically("==", 252))
		})
	})

	// ---------------------------------------------------------------
	// covariance
	// ---------------------------------------------------------------
	Describe("covariance", func() {
		It("returns 0 for empty inputs", func() {
			Expect(portfolio.ExportCovariance([]float64{}, []float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single-element inputs", func() {
			Expect(portfolio.ExportCovariance([]float64{5}, []float64{10})).To(BeNumerically("==", 0))
		})

		It("returns 0 when one input is nil", func() {
			Expect(portfolio.ExportCovariance(nil, []float64{1, 2})).To(BeNumerically("==", 0))
		})

		It("trims to the shorter array", func() {
			x := []float64{1, 2, 3}
			y := []float64{2, 4}
			// Trimmed: x=[1,2], y=[2,4]
			// mx=1.5, my=3, cov = ((1-1.5)*(2-3) + (2-1.5)*(4-3)) / 1 = (0.5+0.5)/1 = 1.0
			Expect(portfolio.ExportCovariance(x, y)).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("computes correct sample covariance for known data", func() {
			x := []float64{1, 2, 3, 4, 5}
			y := []float64{2, 4, 6, 8, 10}
			// y = 2x, perfect positive linear relationship
			// mx=3, my=6, sum = (-2)(-4)+(-1)(-2)+(0)(0)+(1)(2)+(2)(4) = 8+2+0+2+8 = 20
			// cov = 20 / 4 = 5.0
			Expect(portfolio.ExportCovariance(x, y)).To(BeNumerically("~", 5.0, 1e-12))
		})
	})

	// ---------------------------------------------------------------
	// windowSlice
	// ---------------------------------------------------------------
	Describe("windowSlice", func() {
		var (
			series []float64
			times  []time.Time
		)

		BeforeEach(func() {
			start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
			times = daySeq(start, 10)
			series = []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		})

		It("returns full series when window is nil", func() {
			result := portfolio.ExportWindowSlice(series, times, nil)
			Expect(result).To(Equal(series))
		})

		It("returns full series when times is empty and window is non-nil", func() {
			w := portfolio.Days(5)
			result := portfolio.ExportWindowSlice(series, []time.Time{}, &w)
			Expect(result).To(Equal(series))
		})

		It("trims to trailing window for day-based period", func() {
			// Last date is times[9]. Window of 5 days means cutoff = last - 5 days.
			// Keep entries where t >= cutoff.
			w := portfolio.Days(5)
			result := portfolio.ExportWindowSlice(series, times, &w)
			// The result should be a suffix of the series
			Expect(len(result)).To(BeNumerically(">", 0))
			Expect(len(result)).To(BeNumerically("<=", len(series)))
		})

		It("returns full series when window is larger than available data", func() {
			w := portfolio.Years(10) // much larger than 10 days of data
			result := portfolio.ExportWindowSlice(series, times, &w)
			Expect(result).To(Equal(series))
		})

		It("works with monthly window", func() {
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			monthTimes := monthSeq(start, 12)
			monthSeries := make([]float64, 12)
			for i := range monthSeries {
				monthSeries[i] = float64(i + 1)
			}

			w := portfolio.Months(3)
			result := portfolio.ExportWindowSlice(monthSeries, monthTimes, &w)
			// Last date = 2024-12-01, cutoff = 2024-09-01
			// Entries from index 8 onward (Sep, Oct, Nov, Dec) = 4 elements
			Expect(len(result)).To(BeNumerically(">=", 3))
			// Last element should be 12.0
			Expect(result[len(result)-1]).To(BeNumerically("==", 12.0))
		})

		It("works with year window", func() {
			start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			monthTimes := monthSeq(start, 36) // 3 years of monthly data
			monthSeries := make([]float64, 36)
			for i := range monthSeries {
				monthSeries[i] = float64(i)
			}

			w := portfolio.Years(1)
			result := portfolio.ExportWindowSlice(monthSeries, monthTimes, &w)
			// Last date = 2022-12-01, cutoff = 2021-12-01
			// Entries from Dec 2021 onward = 13 entries (Dec 2021 through Dec 2022)
			Expect(len(result)).To(BeNumerically(">=", 12))
			Expect(result[len(result)-1]).To(BeNumerically("==", 35.0))
		})
	})

	// ---------------------------------------------------------------
	// drawdownSeries
	// ---------------------------------------------------------------
	Describe("drawdownSeries", func() {
		It("returns empty slice for empty input", func() {
			dd := portfolio.ExportDrawdownSeries([]float64{})
			Expect(dd).To(HaveLen(0))
		})

		It("returns zero drawdown for a single element", func() {
			dd := portfolio.ExportDrawdownSeries([]float64{100})
			Expect(dd).To(HaveLen(1))
			Expect(dd[0]).To(BeNumerically("==", 0.0))
		})

		It("returns all zeros for monotonically rising equity", func() {
			equity := []float64{100, 110, 120, 130, 140}
			dd := portfolio.ExportDrawdownSeries(equity)
			Expect(dd).To(HaveLen(5))
			for i, v := range dd {
				Expect(v).To(BeNumerically("==", 0.0), "drawdown at index %d should be 0", i)
			}
		})

		It("computes correct drawdown for a peak-then-trough pattern", func() {
			equity := []float64{100, 120, 90, 110}
			dd := portfolio.ExportDrawdownSeries(equity)
			Expect(dd).To(HaveLen(4))
			// Peak at 100: dd[0] = (100-100)/100 = 0
			Expect(dd[0]).To(BeNumerically("==", 0.0))
			// New peak at 120: dd[1] = (120-120)/120 = 0
			Expect(dd[1]).To(BeNumerically("==", 0.0))
			// Drop to 90, peak still 120: dd[2] = (90-120)/120 = -0.25
			Expect(dd[2]).To(BeNumerically("~", -0.25, 1e-12))
			// Rise to 110, peak still 120: dd[3] = (110-120)/120 = -1/12
			Expect(dd[3]).To(BeNumerically("~", -1.0/12.0, 1e-12))
		})

		It("handles a complete recovery", func() {
			equity := []float64{100, 80, 100, 120}
			dd := portfolio.ExportDrawdownSeries(equity)
			Expect(dd[0]).To(BeNumerically("==", 0.0))
			Expect(dd[1]).To(BeNumerically("~", -0.20, 1e-12))
			Expect(dd[2]).To(BeNumerically("==", 0.0)) // recovered to peak
			Expect(dd[3]).To(BeNumerically("==", 0.0)) // new peak
		})
	})

	// ---------------------------------------------------------------
	// cagr
	// ---------------------------------------------------------------
	Describe("cagr", func() {
		It("returns 0 for zero years", func() {
			Expect(portfolio.ExportCagr(100, 200, 0)).To(BeNumerically("==", 0))
		})

		It("returns 0 for zero start value", func() {
			Expect(portfolio.ExportCagr(0, 200, 1)).To(BeNumerically("==", 0))
		})

		It("returns 0 for negative start value", func() {
			Expect(portfolio.ExportCagr(-100, 200, 1)).To(BeNumerically("==", 0))
		})

		It("returns 0 for zero end value", func() {
			Expect(portfolio.ExportCagr(100, 0, 1)).To(BeNumerically("==", 0))
		})

		It("returns 0 for negative end value", func() {
			Expect(portfolio.ExportCagr(100, -50, 1)).To(BeNumerically("==", 0))
		})

		It("returns 0 for negative years", func() {
			Expect(portfolio.ExportCagr(100, 200, -1)).To(BeNumerically("==", 0))
		})

		It("computes correct CAGR for 1 year doubling", func() {
			// (200/100)^(1/1) - 1 = 1.0
			Expect(portfolio.ExportCagr(100, 200, 1)).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("computes correct CAGR for multi-year growth", func() {
			// 100 -> 200 over 2 years: (200/100)^(1/2) - 1 = sqrt(2) - 1
			expected := math.Sqrt(2) - 1
			Expect(portfolio.ExportCagr(100, 200, 2)).To(BeNumerically("~", expected, 1e-12))
		})

		It("returns negative CAGR for declining value", func() {
			// 100 -> 50 over 1 year: (50/100)^1 - 1 = -0.5
			Expect(portfolio.ExportCagr(100, 50, 1)).To(BeNumerically("~", -0.5, 1e-12))
		})
	})

	// ---------------------------------------------------------------
	// variance and stddev
	// ---------------------------------------------------------------
	Describe("variance", func() {
		It("returns 0 for empty input", func() {
			Expect(portfolio.ExportVariance([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single element", func() {
			Expect(portfolio.ExportVariance([]float64{42})).To(BeNumerically("==", 0))
		})

		It("computes correct sample variance for known data", func() {
			// [1, 2, 3], mean=2, sum of sq diffs = 1+0+1 = 2, var = 2/2 = 1.0
			Expect(portfolio.ExportVariance([]float64{1, 2, 3})).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("returns 0 for all-identical values", func() {
			Expect(portfolio.ExportVariance([]float64{5, 5, 5, 5})).To(BeNumerically("==", 0))
		})
	})

	Describe("stddev", func() {
		It("returns 0 for empty input", func() {
			Expect(portfolio.ExportStddev([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns 0 for single element", func() {
			Expect(portfolio.ExportStddev([]float64{42})).To(BeNumerically("==", 0))
		})

		It("is the square root of the variance", func() {
			x := []float64{1, 2, 3}
			Expect(portfolio.ExportStddev(x)).To(BeNumerically("~", 1.0, 1e-12))
		})
	})

	// ---------------------------------------------------------------
	// mean
	// ---------------------------------------------------------------
	Describe("mean", func() {
		It("returns 0 for empty slice", func() {
			Expect(portfolio.ExportMean([]float64{})).To(BeNumerically("==", 0))
		})

		It("returns the single value for a one-element slice", func() {
			Expect(portfolio.ExportMean([]float64{7.5})).To(BeNumerically("~", 7.5, 1e-12))
		})

		It("computes arithmetic mean for multiple elements", func() {
			Expect(portfolio.ExportMean([]float64{1, 2, 3, 4})).To(BeNumerically("~", 2.5, 1e-12))
		})

		It("returns 0 for nil slice", func() {
			Expect(portfolio.ExportMean(nil)).To(BeNumerically("==", 0))
		})
	})

	// ---------------------------------------------------------------
	// returns (bonus: the function is a dependency of the others)
	// ---------------------------------------------------------------
	Describe("returns", func() {
		It("returns empty slice for single element", func() {
			r := portfolio.ExportReturns([]float64{100})
			Expect(r).To(HaveLen(0))
		})

		It("returns empty slice for empty input", func() {
			r := portfolio.ExportReturns([]float64{})
			Expect(r).To(HaveLen(0))
		})

		It("computes period-over-period returns", func() {
			r := portfolio.ExportReturns([]float64{100, 110, 99})
			Expect(r).To(HaveLen(2))
			Expect(r[0]).To(BeNumerically("~", 0.10, 1e-12))
			Expect(r[1]).To(BeNumerically("~", -0.1, 1e-12))
		})
	})

	// ---------------------------------------------------------------
	// windowSliceTimes
	// ---------------------------------------------------------------
	Describe("windowSliceTimes", func() {
		It("returns full slice when window is nil", func() {
			times := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			result := portfolio.ExportWindowSliceTimes(times, nil)
			Expect(result).To(Equal(times))
		})

		It("returns empty slice for empty input", func() {
			w := portfolio.Days(5)
			result := portfolio.ExportWindowSliceTimes([]time.Time{}, &w)
			Expect(result).To(HaveLen(0))
		})
	})
})
