package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// benchAcct builds an Account whose equity curve equals eqCurve, with
// benchmark and risk-free prices fed in via UpdatePrices over daily
// timestamps produced by daySeq.
func benchAcct(eqCurve, bmPrices, rfPrices []float64) *portfolio.Account {
	bm := asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}

	a := portfolio.New(
		portfolio.WithCash(eqCurve[0], time.Time{}),
		portfolio.WithBenchmark(bm),
	)

	dates := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), len(eqCurve))

	for i := range eqCurve {
		if i > 0 {
			diff := eqCurve[i] - eqCurve[i-1]
			if diff > 0 {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.DepositTransaction,
					Amount: diff,
				})
			} else {
				a.Record(portfolio.Transaction{
					Date:   dates[i],
					Type:   portfolio.WithdrawalTransaction,
					Amount: diff,
				})
			}
		}

		a.SetRiskFreeValue(rfPrices[i])
		df := buildDF(dates[i],
			[]asset.Asset{bm},
			[]float64{bmPrices[i]},
			[]float64{bmPrices[i]},
		)
		a.UpdatePrices(df)
	}

	return a
}

var _ = Describe("Benchmark Metrics", func() {

	// ---------------------------------------------------------------
	// Main test: divergent portfolio and benchmark with hand-calculated
	// expected values.
	//
	// Equity curve:      [1000, 1050, 1020, 1080, 1060, 1100]
	// Benchmark prices:  [ 100,  104,  101,  107,  105,  109]
	// Risk-free prices:  [ 100, 100.01, 100.02, 100.03, 100.04, 100.05]
	//
	// Portfolio returns:
	//   r0 = (1050-1000)/1000 =  0.05
	//   r1 = (1020-1050)/1050 = -0.028571428571429
	//   r2 = (1080-1020)/1020 =  0.058823529411765
	//   r3 = (1060-1080)/1080 = -0.018518518518519
	//   r4 = (1100-1060)/1060 =  0.037735849056604
	//
	// Benchmark returns:
	//   r0 = (104-100)/100 =  0.04
	//   r1 = (101-104)/104 = -0.028846153846154
	//   r2 = (107-101)/101 =  0.059405940594059
	//   r3 = (105-107)/107 = -0.018691588785047
	//   r4 = (109-105)/105 =  0.038095238095238
	//
	// Active returns (portfolio - benchmark):
	//   a0 =  0.010000000000000
	//   a1 =  0.000274725274725
	//   a2 = -0.000582411182295
	//   a3 =  0.000173070266528
	//   a4 = -0.000359389038634
	//
	// Total returns:
	//   portfolioReturn = 1100/1000 - 1 = 0.10
	//   benchmarkReturn = 109/100  - 1 = 0.09
	//   riskFreeReturn  = 100.05/100 - 1 = 0.0005
	//
	// annualizationFactor = 252 (daily data, avg gap ~1.4 days)
	// ---------------------------------------------------------------

	Describe("divergent portfolio", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			a = benchAcct(
				[]float64{1000, 1050, 1020, 1080, 1060, 1100},
				[]float64{100, 104, 101, 107, 105, 109},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)
		})

		It("Beta = cov(pR,bR)/var(bR)", func() {
			// cov(pR,bR) = 0.001578154309907
			// var(bR)    = 0.001535776265237
			// Beta       = 1.027593892176544
			v, err := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 1.027593892176544, 1e-10))
		})

		It("Alpha = (mean(R_p-R_f) - beta*mean(R_m-R_f)) * AF", func() {
			// alpha_per_period = mean(R_p-R_f) - beta*mean(R_m-R_f)
			// AF = 5 / (7/365.25) = 260.89
			// alpha_annualized = 0.3672
			v, err := a.PerformanceMetric(portfolio.Alpha).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.3671987732916029, 1e-10))
		})

		It("TrackingError = stddev(activeReturns) * sqrt(AF)", func() {
			// stddev(activeR) = 0.004541503086922
			// AF = 5 / (7/365.25) = 260.89
			// TE = 0.004541503... * sqrt(AF) = 0.07335516666914621
			v, err := a.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.07335516666914621, 1e-10))
		})

		It("InformationRatio = mean(activeR)/stddev(activeR) * sqrt(AF)", func() {
			// mean(activeR)   = 0.001901199064065
			// stddev(activeR) = 0.004541503086922
			// AF = 260.89
			// IR = (0.001901.../0.004541...) * sqrt(AF) = 6.7617494219373295
			v, err := a.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 6.7617494219373295, 1e-8))
		})

		It("Treynor = (portfolioReturn - rfReturn) / beta", func() {
			// treynor = (0.10 - 0.0005) / 1.027594 = 0.096828134886292
			v, err := a.PerformanceMetric(portfolio.Treynor).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.096828134886292, 1e-10))
		})

		It("RSquared = corr(pR,bR)^2", func() {
			// corr = cov(pR,bR)/(stddev(pR)*stddev(bR)) = 0.994054842268791
			// R^2  = 0.988145029438031
			v, err := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.988145029438031, 1e-10))
		})
	})

	// ---------------------------------------------------------------
	// Perfect tracking: portfolio equity = 10 * benchmark.
	// Returns are identical, so active returns are all zero.
	//
	// Beta = 1.0   (cov/var = var/var)
	// Alpha = 0.0  (portfolioReturn == benchmarkReturn when beta=1)
	//   portfolioReturn = 1090/1000 - 1 = 0.09
	//   alpha = 0.09 - (0.0005 + 1.0*(0.09-0.0005)) = 0
	// TrackingError = 0   (stddev of zeros)
	// InformationRatio = 0 (te=0 guard)
	// Treynor = (0.09 - 0.0005)/1.0 = 0.0895
	// RSquared = 1.0  (perfect correlation)
	// ---------------------------------------------------------------

	Describe("perfect tracking", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			bm := []float64{100, 104, 101, 107, 105, 109}
			eq := make([]float64, len(bm))
			for i := range bm {
				eq[i] = bm[i] * 10 // portfolio = 10x benchmark
			}
			a = benchAcct(
				eq,
				bm,
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)
		})

		It("Beta = 1.0", func() {
			v, err := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 1.0, 1e-10))
		})

		It("Alpha = 0.0", func() {
			v, err := a.PerformanceMetric(portfolio.Alpha).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.0, 1e-10))
		})

		It("TrackingError = 0.0", func() {
			v, err := a.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.0, 1e-10))
		})

		It("InformationRatio = 0.0 (te=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.0, 1e-10))
		})

		It("Treynor = (0.09 - 0.0005) / 1.0", func() {
			// portfolioReturn = 1090/1000 - 1 = 0.09
			// riskFreeReturn  = 100.05/100 - 1 = 0.0005
			// beta = 1.0
			v, err := a.PerformanceMetric(portfolio.Treynor).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.0895, 1e-10))
		})

		It("RSquared = 1.0", func() {
			v, err := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 1.0, 1e-10))
		})
	})

	// ---------------------------------------------------------------
	// ComputeSeries tests: all benchmark metrics return nil.
	// ---------------------------------------------------------------

	Describe("ComputeSeries returns nil", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			a = benchAcct(
				[]float64{1000, 1050, 1020, 1080, 1060, 1100},
				[]float64{100, 104, 101, 107, 105, 109},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)
		})

		It("Beta returns nil from ComputeSeries", func() {
			s, err := portfolio.Beta.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("Alpha returns nil from ComputeSeries", func() {
			s, err := portfolio.Alpha.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("TrackingError returns nil from ComputeSeries", func() {
			s, err := portfolio.TrackingError.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("InformationRatio returns nil from ComputeSeries", func() {
			s, err := portfolio.InformationRatio.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("Treynor returns nil from ComputeSeries", func() {
			s, err := portfolio.Treynor.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("RSquared returns nil from ComputeSeries", func() {
			s, err := portfolio.RSquared.ComputeSeries(a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})
	})

	// ---------------------------------------------------------------
	// Edge: zero benchmark variance (flat benchmark at 100).
	// Portfolio grows via dividends: equity = [1000, 1100, 1200, 1300, 1400].
	// Benchmark returns all zero -> variance=0 -> Beta=0.
	// Beta=0 -> Treynor=0.
	// Portfolio stddev > 0, benchmark stddev = 0 -> RSquared=0 (sb==0 guard).
	// ---------------------------------------------------------------

	Describe("zero benchmark variance", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			a = benchAcct(
				[]float64{1000, 1100, 1200, 1300, 1400},
				[]float64{100, 100, 100, 100, 100},
				[]float64{100, 100.01, 100.02, 100.03, 100.04},
			)
		})

		It("Beta = 0", func() {
			v, err := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("Treynor = 0 (beta=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.Treynor).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("RSquared = 0 (benchmark stddev=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("Alpha = mean(R_p-R_f) * AF when beta=0", func() {
			// beta = 0, so alpha = mean(R_p - R_f) * AF
			// 5 points, 4 returns, daySeq Jan 2,3,6,7,8 -> 6 calendar days
			// AF = 4 / (6/365.25) = 243.5
			// alpha = 21.353
			v, err := a.PerformanceMetric(portfolio.Alpha).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 21.35285353509785, 1e-10))
		})
	})

	// ---------------------------------------------------------------
	// Edge: 2-point equity curve (minimum data producing 1 return).
	// With only 1 return, variance denominator n-1=0, so variance=0.
	// Beta=0, Treynor=0, RSquared=0, TrackingError=0, IR=0.
	// Alpha uses total returns: portfolioReturn=0.1, rfReturn=0.0005,
	//   beta=0 -> alpha = 0.1 - 0.0005 = 0.0995.
	// ---------------------------------------------------------------

	Describe("2-point equity curve", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			a = benchAcct(
				[]float64{1000, 1100},
				[]float64{100, 109},
				[]float64{100, 100.05},
			)
		})

		It("Beta = 0 (single return, variance=0)", func() {
			v, err := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("Alpha = 0 (insufficient data for regression)", func() {
			// Only 1 return from 2 data points -- need at least 2 returns.
			v, err := a.PerformanceMetric(portfolio.Alpha).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("TrackingError = 0 (single return, stddev=0)", func() {
			v, err := a.PerformanceMetric(portfolio.TrackingError).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("InformationRatio = 0 (te=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.InformationRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("Treynor = 0 (beta=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.Treynor).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("RSquared = 0 (single return, stddev=0)", func() {
			v, err := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})
	})

	// ---------------------------------------------------------------
	// Edge: zero portfolio stddev (flat equity, varying benchmark).
	// RSquared guard: sp==0 -> returns 0.
	// Beta: cov=0 (flat returns have zero covariance) -> beta=0.
	// ---------------------------------------------------------------

	Describe("zero portfolio variance", func() {
		var a *portfolio.Account

		BeforeEach(func() {
			a = benchAcct(
				[]float64{1000, 1000, 1000, 1000, 1000},
				[]float64{100, 102, 101, 104, 103},
				[]float64{100, 100, 100, 100, 100},
			)
		})

		It("RSquared = 0 (portfolio stddev=0 guard)", func() {
			v, err := a.PerformanceMetric(portfolio.RSquared).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("Beta = 0 (portfolio returns all zero, cov=0, but var(bR)>0)", func() {
			// Portfolio returns = [0,0,0,0]. cov(0s, bR) = 0. var(bR) > 0.
			// beta = 0/var(bR) = 0
			v, err := a.PerformanceMetric(portfolio.Beta).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})
	})
})
