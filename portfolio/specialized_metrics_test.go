package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Specialized Metrics", func() {
	spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}

	// buildAccountFromEquity creates an Account whose equity curve matches the
	// given values exactly. Since we hold no securities, total value = cash,
	// so we use deposit/withdrawal transactions to move cash to the desired
	// level before each UpdatePrices call.
	buildAccountFromEquity := func(equityValues []float64) *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(equityValues[0]))
		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		dates := daySeq(start, len(equityValues))

		for i, v := range equityValues {
			if i > 0 {
				diff := v - equityValues[i-1]
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   dates[i],
						Type:   portfolio.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					a.Record(portfolio.Transaction{
						Date:   dates[i],
						Type:   portfolio.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}
			df := buildDF(dates[i], []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	// -----------------------------------------------------------------------
	// Shared equity curve: [100, 110, 105, 115, 108, 120, 125]
	//
	// Returns (6 values):
	//   r0 = (110-100)/100 =  0.10000
	//   r1 = (105-110)/110 = -0.04545
	//   r2 = (115-105)/105 =  0.09524
	//   r3 = (108-115)/115 = -0.06087
	//   r4 = (120-108)/108 =  0.11111
	//   r5 = (125-120)/120 =  0.04167
	//
	// Drawdown series (peak tracking):
	//   peaks:  100, 110, 110, 115, 115, 120, 125
	//   dd:     [0,   0,  -0.04545,  0,  -0.06087,  0,  0]
	// -----------------------------------------------------------------------

	Describe("UlcerIndex", func() {
		It("computes correctly for a curve with drawdowns", func() {
			// UlcerIndex = sqrt(mean(dd^2))
			// sumSq = 0.04545^2 + 0.06087^2 = 0.002066 + 0.003705 = 0.005771
			// mean  = 0.005771 / 7 = 0.000824
			// UI    = sqrt(0.000824) = 0.028713
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("~", 0.028713410685187, 1e-9))
		})

		It("returns zero when equity rises monotonically (no drawdowns)", func() {
			a := buildAccountFromEquity([]float64{100, 110, 120, 130, 140})
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("~", 0, 1e-12))
		})

		It("returns zero for a single data point", func() {
			a := buildAccountFromEquity([]float64{100})
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("==", 0))
		})
	})

	Describe("ValueAtRisk", func() {
		It("returns the 5th-percentile return for the base curve", func() {
			// 6 returns sorted: [-0.06087, -0.04545, 0.04167, 0.09524, 0.10, 0.11111]
			// idx = floor(0.05 * 6) = 0 -> sorted[0] = -0.06087
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("~", -0.060869565217391, 1e-9))
		})

		It("returns zero when returns slice is empty (single equity point)", func() {
			a := buildAccountFromEquity([]float64{100})
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("picks the correct sorted percentile for a 20-return series", func() {
			// 21-point equity curve yielding 20 returns.
			// Sorted returns: [-0.04762, -0.04630, -0.04505, -0.04386, ...]
			// idx = floor(0.05 * 20) = 1 -> sorted[1] = -0.04630
			equity := []float64{
				100, 102, 99, 103, 101, 105, 100, 106, 104, 108,
				103, 109, 107, 111, 106, 112, 110, 114, 109, 115, 113,
			}
			a := buildAccountFromEquity(equity)
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("~", -0.046296296296296, 1e-9))
		})
	})

	Describe("KRatio", func() {
		It("computes correctly for the base curve", func() {
			// logVAMI = ln(1000 * cumProd(1+r_i)) for i in [0..5]
			// OLS regression of logVAMI on x=[0,1,2,3,4,5]
			// slope = 0.027913, stderr = 0.010989
			// KRatio = slope / (n * stderr) = 0.027913 / (6 * 0.010989) = 0.42333
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically("~", 0.423332400862063, 1e-9))
		})

		It("returns zero with fewer than 3 returns (3 equity points)", func() {
			// 3 equity points -> 2 returns -> n < 3 guard
			a := buildAccountFromEquity([]float64{100, 110, 120})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero for a single data point", func() {
			a := buildAccountFromEquity([]float64{100})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})
	})

	Describe("KellerRatio", func() {
		It("computes correctly for the base curve", func() {
			// totalReturn = (125/100) - 1 = 0.25
			// maxDD = 0.06087 (< 0.5)
			// K = 0.25 * (1 - 0.06087 / (1 - 0.06087))
			//   = 0.25 * (1 - 0.06087 / 0.93913)
			//   = 0.25 * (1 - 0.06482)
			//   = 0.25 * 0.93518 = 0.23380
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("~", 0.233796296296296, 1e-9))
		})

		It("returns zero when max drawdown exceeds 50%", func() {
			// equity: [100, 120, 40, 50]
			// totalReturn = -0.5, maxDD = 0.6667 (> 0.5)
			// Also totalReturn < 0, so result = 0.
			a := buildAccountFromEquity([]float64{100, 120, 40, 50})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero when total return is negative", func() {
			// equity: [100, 95, 90, 85]
			// totalReturn = -0.15 -> guard returns 0
			a := buildAccountFromEquity([]float64{100, 95, 90, 85})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero for a single data point", func() {
			a := buildAccountFromEquity([]float64{100})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})
	})

	Describe("KRatio edge cases", func() {
		It("returns negative KRatio for a declining curve", func() {
			// logVAMI has a negative slope -> KRatio < 0
			a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically("<", 0))
		})

		It("returns positive KRatio for a rising curve", func() {
			// logVAMI has a positive slope -> KRatio > 0
			a := buildAccountFromEquity([]float64{100, 105, 110, 115, 120})
			result := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("KellerRatio edge cases", func() {
		It("returns zero when max drawdown is exactly 50%", func() {
			// equity: [100, 200, 100]
			// totalReturn = 0.0, maxDD = 0.5
			// totalReturn == 0 -> result = 0 * anything = 0
			a := buildAccountFromEquity([]float64{100, 200, 100})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns non-zero when max drawdown is just under 50%", func() {
			// equity: [100, 200, 101]
			// totalReturn = 0.01 >= 0, maxDD = 0.495 <= 0.5
			// result = 0.01 * (1 - 0.495/0.505) > 0
			a := buildAccountFromEquity([]float64{100, 200, 101})
			result := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("UlcerIndex edge cases", func() {
		It("returns correct value for flat-then-drop curve", func() {
			// equity: [100, 100, 100, 90]
			// dd = [0, 0, 0, -0.1]
			// sumSq = 0.01, mean = 0.0025, UI = sqrt(0.0025) = 0.05
			a := buildAccountFromEquity([]float64{100, 100, 100, 90})
			result := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(result).To(BeNumerically("~", 0.05, 1e-9))
		})
	})

	Describe("ValueAtRisk edge cases", func() {
		It("returns the only return for a 2-point equity curve", func() {
			// equity: [100, 90]
			// 1 return = -0.1
			// idx = floor(0.05 * 1) = 0 -> sorted[0] = -0.1
			a := buildAccountFromEquity([]float64{100, 90})
			result := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(result).To(BeNumerically("~", -0.1, 1e-9))
		})
	})
})
