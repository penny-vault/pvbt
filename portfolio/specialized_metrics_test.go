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
		a := portfolio.New(portfolio.WithCash(equityValues[0], time.Time{}))
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

	// ---------------------------------------------------------------
	// ComputeSeries tests: all specialized metrics return nil.
	// ---------------------------------------------------------------

	Describe("ComputeSeries returns nil", func() {
		It("UlcerIndex returns nil from ComputeSeries", func() {
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			s, err := portfolio.UlcerIndex.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("ValueAtRisk returns nil from ComputeSeries", func() {
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			s, err := portfolio.ValueAtRisk.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("KRatio returns nil from ComputeSeries", func() {
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			s, err := portfolio.KRatio.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("KellerRatio returns nil from ComputeSeries", func() {
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			s, err := portfolio.KellerRatio.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})
	})

	Describe("UlcerIndex", func() {
		It("computes correctly for a 14-point curve with drawdowns", func() {
			// 14-point equity curve with two drawdown episodes.
			// Equity: [100, 105, 110, 108, 112, 115, 113, 118, 120, 116, 119, 122, 121, 125]
			//
			// Rolling peak and percentage drawdown within the 14-period window:
			//   i=0:  peak=100, dd=0
			//   i=1:  peak=105, dd=0
			//   i=2:  peak=110, dd=0
			//   i=3:  peak=110, dd=(108-110)/110*100 = -1.81818
			//   i=4:  peak=112, dd=0
			//   i=5:  peak=115, dd=0
			//   i=6:  peak=115, dd=(113-115)/115*100 = -1.73913
			//   i=7:  peak=118, dd=0
			//   i=8:  peak=120, dd=0
			//   i=9:  peak=120, dd=(116-120)/120*100 = -3.33333
			//   i=10: peak=120, dd=(119-120)/120*100 = -0.83333
			//   i=11: peak=122, dd=0
			//   i=12: peak=122, dd=(121-122)/122*100 = -0.81967
			//   i=13: peak=125, dd=0
			//
			// sumSq = 1.81818^2 + 1.73913^2 + 3.33333^2 + 0.83333^2 + 0.81967^2
			//       = 3.30579 + 3.02456 + 11.11111 + 0.69444 + 0.67186
			//       = 18.80776
			// UI = sqrt(18.80776 / 14) = sqrt(1.34341) = 1.15907
			equity := []float64{100, 105, 110, 108, 112, 115, 113, 118, 120, 116, 119, 122, 121, 125}
			a := buildAccountFromEquity(equity)
			result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 1.15907, 1e-3))
		})

		It("returns zero when equity rises monotonically over 14 periods", func() {
			equity := []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113}
			a := buildAccountFromEquity(equity)
			result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0, 1e-12))
		})

		It("returns zero when fewer than 14 data points", func() {
			a := buildAccountFromEquity([]float64{100, 90, 80})
			result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})
	})

	Describe("ValueAtRisk", func() {
		It("returns the 5th-percentile return for the base curve", func() {
			// 6 returns sorted: [-0.06087, -0.04545, 0.04167, 0.09524, 0.10, 0.11111]
			// idx = floor(0.05 * 6) = 0 -> sorted[0] = -0.06087
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", -0.060869565217391, 1e-9))
		})

		It("returns zero when returns slice is empty (single equity point)", func() {
			a := buildAccountFromEquity([]float64{100})
			result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
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
			result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", -0.046296296296296, 1e-9))
		})
	})

	Describe("KRatio", func() {
		It("computes correctly for the base curve", func() {
			// logVAMI = ln(1000 * cumProd(1+r_i)) for i in [0..5]
			// OLS regression of logVAMI on x=[0,1,2,3,4,5]
			// slope = 0.027913, stderr = 0.010989
			// KRatio = slope / stderr = 0.027913 / 0.010989 = 2.53999 (2003 Kestner revision)
			a := buildAccountFromEquity([]float64{100, 110, 105, 115, 108, 120, 125})
			result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 2.5399944051723757, 1e-9))
		})

		It("returns zero with fewer than 3 returns (3 equity points)", func() {
			// 3 equity points -> 2 returns -> n < 3 guard
			a := buildAccountFromEquity([]float64{100, 110, 120})
			result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero for a single data point", func() {
			a := buildAccountFromEquity([]float64{100})
			result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
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
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.233796296296296, 1e-9))
		})

		It("returns zero when max drawdown exceeds 50%", func() {
			// equity: [100, 120, 40, 50]
			// totalReturn = -0.5, maxDD = 0.6667 (> 0.5)
			// Also totalReturn < 0, so result = 0.
			a := buildAccountFromEquity([]float64{100, 120, 40, 50})
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero when total return is negative", func() {
			// equity: [100, 95, 90, 85]
			// totalReturn = -0.15 -> guard returns 0
			a := buildAccountFromEquity([]float64{100, 95, 90, 85})
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns zero for a single data point", func() {
			a := buildAccountFromEquity([]float64{100})
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})
	})

	Describe("KRatio edge cases", func() {
		It("returns negative KRatio for a declining curve", func() {
			// logVAMI has a negative slope -> KRatio < 0
			a := buildAccountFromEquity([]float64{100, 95, 90, 85, 80})
			result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("<", 0))
		})

		It("returns positive KRatio for a rising curve", func() {
			// logVAMI has a positive slope -> KRatio > 0
			a := buildAccountFromEquity([]float64{100, 105, 110, 115, 120})
			result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("KellerRatio edge cases", func() {
		It("returns zero when max drawdown is exactly 50%", func() {
			// equity: [100, 200, 100]
			// totalReturn = 0.0, maxDD = 0.5
			// totalReturn == 0 -> result = 0 * anything = 0
			a := buildAccountFromEquity([]float64{100, 200, 100})
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 0))
		})

		It("returns non-zero when max drawdown is just under 50%", func() {
			// equity: [100, 200, 101]
			// totalReturn = 0.01 >= 0, maxDD = 0.495 <= 0.5
			// result = 0.01 * (1 - 0.495/0.505) > 0
			a := buildAccountFromEquity([]float64{100, 200, 101})
			result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically(">", 0))
		})
	})

	Describe("UlcerIndex edge cases", func() {
		It("returns correct value for flat-then-drop curve", func() {
			// 14-point curve: 13 flat values then a 10% drop.
			// equity: [100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 90]
			// All dd=0 except last: (90-100)/100*100 = -10.0
			// sumSq = 10^2 = 100
			// UI = sqrt(100 / 14) = sqrt(7.14286) = 2.67261
			equity := []float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 90}
			a := buildAccountFromEquity(equity)
			result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 2.67261, 1e-3))
		})
	})

	Describe("ValueAtRisk edge cases", func() {
		It("returns the only return for a 2-point equity curve", func() {
			// equity: [100, 90]
			// 1 return = -0.1
			// idx = floor(0.05 * 1) = 0 -> sorted[0] = -0.1
			a := buildAccountFromEquity([]float64{100, 90})
			result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", -0.1, 1e-9))
		})
	})

	// ---------------------------------------------------------------
	// High-stress / extreme scenario tests
	// ---------------------------------------------------------------

	Describe("high-stress scenarios", func() {
		Describe("UlcerIndex", func() {
			It("produces a high value for a deep sustained drawdown", func() {
				// 14-point curve that drops steeply from 100 to 30.
				// equity: [100, 95, 90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 30]
				// peak stays at 100 throughout (never recovered).
				// dd (pct): [0, -5, -10, -15, -20, -25, -30, -35, -40, -45, -50, -55, -60, -70]
				// sumSq = 5^2+10^2+15^2+20^2+25^2+30^2+35^2+40^2+45^2+50^2+55^2+60^2+70^2
				//       = 25+100+225+400+625+900+1225+1600+2025+2500+3025+3600+4900
				//       = 21150
				// UI = sqrt(21150 / 14) = sqrt(1510.714) = 38.8678
				equity := []float64{100, 95, 90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 30}
				a := buildAccountFromEquity(equity)
				result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", 38.8678, 1e-2))
				// Much larger than the mild-drawdown UI of ~1.16
				Expect(result).To(BeNumerically(">", 30.0))
			})

			It("captures lingering drawdown during crash-and-recovery", func() {
				// 14-point curve: crash to 30 then partial recovery to 80.
				// equity: [100, 80, 60, 40, 30, 35, 40, 50, 55, 60, 65, 70, 75, 80]
				// peak stays at 100 (never recovered).
				// dd (pct): [0, -20, -40, -60, -70, -65, -60, -50, -45, -40, -35, -30, -25, -20]
				// sumSq = 20^2+40^2+60^2+70^2+65^2+60^2+50^2+45^2+40^2+35^2+30^2+25^2+20^2
				//       = 400+1600+3600+4900+4225+3600+2500+2025+1600+1225+900+625+400
				//       = 27600
				// UI = sqrt(27600 / 14) = sqrt(1971.429) = 44.4010
				equity := []float64{100, 80, 60, 40, 30, 35, 40, 50, 55, 60, 65, 70, 75, 80}
				a := buildAccountFromEquity(equity)
				result, err := a.PerformanceMetric(portfolio.UlcerIndex).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", 44.4010, 1e-2))
				// Even though equity recovers to 80, the drawdown lingers because
				// the peak of 100 is never restored.
				Expect(result).To(BeNumerically(">", 40.0))
			})
		})

		Describe("ValueAtRisk", func() {
			It("picks up a single catastrophic drop in a heavy-tail distribution", func() {
				// equity: [100, 102, 104, 106, 108, 40, 110, 112, 114, 116, 118]
				// 10 returns:
				//   r0=0.02, r1=0.01961, r2=0.01923, r3=0.01887,
				//   r4=(40-108)/108=-0.62963,  (catastrophic drop)
				//   r5=(110-40)/40=1.75,        (recovery)
				//   r6=0.01818, r7=0.01786, r8=0.01754, r9=0.01724
				// sorted: [-0.62963, 0.01724, 0.01754, ..., 1.75]
				// idx = floor(0.05 * 10) = 0 -> sorted[0] = -0.62963
				a := buildAccountFromEquity([]float64{
					100, 102, 104, 106, 108, 40, 110, 112, 114, 116, 118,
				})
				result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", -0.629629629629630, 1e-9))
			})

			It("returns the worst return when all returns are negative", func() {
				// equity: [100, 95, 90, 85, 80, 75, 70, 65, 60, 55, 50]
				// 10 returns, all negative. Each is (next-prev)/prev:
				//   r0=-5/100=-0.05,  r1=-5/95=-0.05263,  r2=-5/90=-0.05556,
				//   r3=-5/85=-0.05882, r4=-5/80=-0.06250, r5=-5/75=-0.06667,
				//   r6=-5/70=-0.07143, r7=-5/65=-0.07692, r8=-5/60=-0.08333,
				//   r9=-5/55=-0.09091
				// sorted ascending: [-0.09091, -0.08333, ..., -0.05]
				// idx = floor(0.05 * 10) = 0 -> sorted[0] = -5/55 = -0.09091
				a := buildAccountFromEquity([]float64{
					100, 95, 90, 85, 80, 75, 70, 65, 60, 55, 50,
				})
				result, err := a.PerformanceMetric(portfolio.ValueAtRisk).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", -5.0/55.0, 1e-9))
			})
		})

		Describe("KRatio", func() {
			It("is positive but small for a highly volatile upward trend", func() {
				// equity: [100, 130, 95, 140, 90, 145, 100, 150]
				// 7 returns with large swings but net upward:
				//   r0=+0.30, r1=-0.26923, r2=+0.47368, r3=-0.35714,
				//   r4=+0.61111, r5=-0.31034, r6=+0.50
				// logVAMI oscillates heavily around the trend line, producing
				// a large standard error that reduces the K-Ratio.
				// OLS on logVAMI: slope=0.04918, stderr=0.08882
				// KRatio = slope / stderr = 0.04918 / 0.08882 = 0.45738 (2003 Kestner revision)
				a := buildAccountFromEquity([]float64{100, 130, 95, 140, 90, 145, 100, 150})
				result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", 0.45737915846086635, 1e-9))
				// Positive but much smaller than the steady-growth case
				Expect(result).To(BeNumerically(">", 0))
				Expect(result).To(BeNumerically("<", 1.0))
			})

			It("is very large for steady consistent growth", func() {
				// equity: [100, 105, 110, 115, 120, 125, 130]
				// 6 returns that decrease slightly (5/100, 5/105, 5/110, ...),
				// producing near-linear logVAMI with tiny residuals.
				// slope = 0.042684, stderr = 0.000666
				// KRatio = 0.042684 / 0.000666 = 64.113 (2003 Kestner revision)
				a := buildAccountFromEquity([]float64{100, 105, 110, 115, 120, 125, 130})
				result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", 64.11253704211674, 1e-6))
				// Orders of magnitude larger than the volatile case
				Expect(result).To(BeNumerically(">", 10))
			})

			It("is strongly negative for a severe crash", func() {
				// equity: [100, 80, 55, 35, 20, 10]
				// 5 returns, all negative and accelerating:
				//   r0=-0.20, r1=-0.3125, r2=-0.36364, r3=-0.42857, r4=-0.50
				// logVAMI has a steep negative slope.
				// slope = -0.14936, stderr = 0.01053
				// KRatio = -0.14936 / 0.01053 = -14.176 (2003 Kestner revision)
				a := buildAccountFromEquity([]float64{100, 80, 55, 35, 20, 10})
				result, err := a.PerformanceMetric(portfolio.KRatio).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", -14.175542222575912, 1e-6))
				Expect(result).To(BeNumerically("<", -2))
			})
		})

		Describe("KellerRatio", func() {
			It("produces a meaningfully positive value for high return with moderate drawdown", func() {
				// equity: [100, 160, 115, 200]
				// totalReturn = (200/100) - 1 = 1.0 (100% return)
				// peaks:  100, 160, 160, 200
				// dd:    [0, 0, (115-160)/160, 0] = [0, 0, -0.28125, 0]
				// maxDD = 0.28125 (well under 50%)
				// K = 1.0 * (1 - 0.28125 / (1 - 0.28125))
				//   = 1.0 * (1 - 0.28125 / 0.71875)
				//   = 1.0 * (1 - 0.391304348)
				//   = 0.608695652173913
				a := buildAccountFromEquity([]float64{100, 160, 115, 200})
				result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("~", 0.608695652173913, 1e-9))
				// A strong positive value reflecting good risk-adjusted return
				Expect(result).To(BeNumerically(">", 0.5))
			})

			It("returns zero when drawdown exceeds 50% despite high total return", func() {
				// equity: [100, 200, 99.8, 300]
				// totalReturn = (300/100) - 1 = 2.0 (200% return)
				// peaks:  100, 200, 200, 300
				// dd:    [0, 0, (99.8-200)/200, 0] = [0, 0, -0.501, 0]
				// maxDD = 0.501 (just over 50% threshold)
				// Since maxDD > 0.5, Keller returns 0 regardless of total return.
				a := buildAccountFromEquity([]float64{100, 200, 99.8, 300})
				result, err := a.PerformanceMetric(portfolio.KellerRatio).Value()
			Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNumerically("==", 0))
			})
		})
	})
})
