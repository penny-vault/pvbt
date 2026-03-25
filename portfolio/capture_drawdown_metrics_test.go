package portfolio_test

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Capture and Drawdown Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
	})

	// cashAccount builds a cash-only account whose equity curve matches the
	// given equityVals by injecting deposit/withdrawal transactions between
	// each UpdatePrices call. benchVals are the benchmark AdjClose prices.
	cashAccount := func(equityVals, benchVals []float64) *portfolio.Account {
		Expect(len(equityVals)).To(Equal(len(benchVals)))
		dates := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), len(equityVals))

		a := portfolio.New(
			portfolio.WithCash(equityVals[0], time.Time{}),
			portfolio.WithBenchmark(bm),
		)

		for i := range equityVals {
			if i > 0 {
				diff := equityVals[i] - equityVals[i-1]
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   dates[i],
						Type:   asset.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					a.Record(portfolio.Transaction{
						Date:   dates[i],
						Type:   asset.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}

			a.SetRiskFreeValue(100)
			df := buildDF(dates[i],
				[]asset.Asset{spy, bm},
				[]float64{100, benchVals[i]},
				[]float64{100, benchVals[i]},
			)
			a.UpdatePrices(df)
		}

		return a
	}

	// ---------------------------------------------------------------
	// ComputeSeries tests: capture and drawdown metrics return nil.
	// ---------------------------------------------------------------

	Describe("ComputeSeries returns nil", func() {
		It("UpsideCaptureRatio returns nil from ComputeSeries", func() {
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)
			s, err := portfolio.UpsideCaptureRatio.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("DownsideCaptureRatio returns nil from ComputeSeries", func() {
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)
			s, err := portfolio.DownsideCaptureRatio.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})

		It("AvgDrawdown returns nil from ComputeSeries", func() {
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)
			s, err := portfolio.AvgDrawdown.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(s).To(BeNil())
		})
	})

	Describe("UpsideCaptureRatio", func() {
		It("computes the ratio of geometric means in up-benchmark periods", func() {
			// Equity:    [10000, 12000, 10800, 12960]
			// Benchmark: [100,   110,   99,    108.9]
			//
			// Portfolio returns:  [0.2,   -1/10,  0.2  ]
			// Benchmark returns:  [0.1,   -0.1,   0.1  ]  (99->108.9 = 9.9/99 = 0.1)
			//
			// Up periods (bRet > 0): i=0 (pRet=0.2, bRet=0.1), i=2 (pRet=0.2, bRet=0.1)
			//
			// geoP = ((1.2)(1.2))^(1/2) - 1 = 1.44^0.5 - 1 = 1.2 - 1 = 0.2
			// geoB = ((1.1)(1.1))^(1/2) - 1 = 1.21^0.5 - 1 = 1.1 - 1 = 0.1
			// ratio = 0.2 / 0.1 = 2.0
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())

			// geoP = math.Pow(1.2*1.2, 0.5) - 1 = 0.2
			// geoB = math.Pow(1.1*1.1, 0.5) - 1 = 0.1
			// exact floating-point: 2.0
			Expect(v).To(BeNumerically("~", 2.0, 1e-9))
		})

		It("returns 0 when benchmark never rises", func() {
			// Benchmark: [100, 95, 90, 85] -- all returns negative
			// No up periods => returns 0.
			a := cashAccount(
				[]float64{10000, 10500, 10200, 10800},
				[]float64{100, 95, 90, 85},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("returns 0 with a single data point (no returns)", func() {
			a := cashAccount(
				[]float64{10000},
				[]float64{100},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("handles asymmetric capture where portfolio gains less than benchmark", func() {
			// Equity:    [10000, 10500, 10000, 10500]
			// Benchmark: [100,   110,   100,   110  ]
			//
			// Portfolio returns:  [0.05,  -500/10500,  0.05 ]
			// Benchmark returns:  [0.1,   -1/11,       0.1  ]
			//
			// Up periods: i=0 (pRet=0.05, bRet=0.1), i=2 (pRet=0.05, bRet=0.1)
			// geoP = ((1.05)(1.05))^(1/2) - 1 = 1.05 - 1 = 0.05
			// geoB = ((1.1)(1.1))^(1/2) - 1 = 1.1 - 1 = 0.1
			// ratio = 0.05 / 0.1 = 0.5
			a := cashAccount(
				[]float64{10000, 10500, 10000, 10500},
				[]float64{100, 110, 100, 110},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.5, 1e-9))
		})
	})

	Describe("DownsideCaptureRatio", func() {
		It("computes the ratio of geometric means in down-benchmark periods", func() {
			// Equity:    [10000, 12000, 10800, 12960]
			// Benchmark: [100,   110,   99,    108.9]
			//
			// Portfolio returns:  [0.2,   -0.1,   0.2 ]
			// Benchmark returns:  [0.1,   -0.1,   0.1 ]  (110->99 = -11/110 = -0.1)
			//
			// Down periods (bRet < 0): i=1 (pRet=-0.1, bRet=-0.1)
			//
			// geoP = (1 + (-0.1))^(1/1) - 1 = 0.9 - 1 = -0.1
			// geoB = (1 + (-0.1))^(1/1) - 1 = 0.9 - 1 = -0.1
			// ratio = -0.1 / -0.1 = 1.0
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)

			v, err := a.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())

			// (110 -> 99): bRet = (99-110)/110 = -11/110 = -0.1 exactly
			// pRet = (10800-12000)/12000 = -1200/12000 = -0.1 exactly
			Expect(v).To(BeNumerically("~", 1.0, 1e-9))
		})

		It("portfolio loses less than benchmark in down markets", func() {
			// Equity:    [10000, 11000, 10725, 11797.5]
			// Benchmark: [100,   110,   99,    108.9  ]
			//
			// Portfolio returns:  [0.1,   -0.025,  0.1   ]
			// Benchmark returns:  [0.1,   -0.1,    0.1   ]
			//
			// Down periods: i=1 (pRet=-0.025, bRet=-0.1)
			// geoP = (0.975)^1 - 1 = -0.025
			// geoB = (0.9)^1 - 1 = -0.1
			// ratio = -0.025 / -0.1 = 0.25
			a := cashAccount(
				[]float64{10000, 11000, 10725, 11797.5},
				[]float64{100, 110, 99, 108.9},
			)

			v, err := a.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.25, 1e-9))
		})

		It("returns 0 when benchmark never falls", func() {
			// Benchmark: [100, 105, 110, 115] -- all returns positive
			a := cashAccount(
				[]float64{10000, 10200, 10100, 10300},
				[]float64{100, 105, 110, 115},
			)

			v, err := a.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("portfolio doesn't participate in down markets", func() {
			// Equity:    [10000, 11000, 11000, 12100]
			// Benchmark: [100,   110,   99,    108.9]
			//
			// Portfolio returns:  [0.1, 0.0, 0.1]
			// Benchmark returns:  [0.1, -0.1, 0.1]
			//
			// Down periods: i=1 (pRet=0.0, bRet=-0.1)
			// geoP = (1.0)^1 - 1 = 0.0
			// geoB = (0.9)^1 - 1 = -0.1
			// ratio = (0.0 / -0.1) * 100 = 0.0
			a := cashAccount(
				[]float64{10000, 11000, 11000, 12100},
				[]float64{100, 110, 99, 108.9},
			)

			v, err := a.PerformanceMetric(portfolio.DownsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 0.0, 1e-9))
		})
	})

	Describe("AvgDrawdown", func() {
		It("computes mean of all drawdown values", func() {
			// Equity: [10000, 12000, 10800, 12960]
			//
			// drawdownSeries:
			//   i=0: peak=10000, dd=(10000-10000)/10000 = 0
			//   i=1: peak=12000, dd=(12000-12000)/12000 = 0
			//   i=2: peak=12000, dd=(10800-12000)/12000 = -1200/12000 = -0.1
			//   i=3: peak=12960, dd=(12960-12960)/12960 = 0
			//
			// mean([0, 0, -0.1, 0]) = -0.025
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 110, 99, 108.9},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.025, 1e-9))
		})

		It("averages all drawdown values across multiple episodes", func() {
			// Equity: [10000, 12000, 10800, 13000, 11700]
			//
			// drawdownSeries:
			//   i=0: peak=10000, dd=0
			//   i=1: peak=12000, dd=0
			//   i=2: peak=12000, dd=(10800-12000)/12000 = -0.1
			//   i=3: peak=13000, dd=0
			//   i=4: peak=13000, dd=(11700-13000)/13000 = -1300/13000 = -0.1
			//
			// mean([0, 0, -0.1, 0, -0.1]) = -0.04
			a := cashAccount(
				[]float64{10000, 12000, 10800, 13000, 11700},
				[]float64{100, 105, 100, 105, 100},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.04, 1e-9))
		})

		It("averages all drawdown values with different depths", func() {
			// Equity: [10000, 20000, 18000, 20000, 16000]
			//
			// drawdownSeries:
			//   i=0: peak=10000, dd=0
			//   i=1: peak=20000, dd=0
			//   i=2: peak=20000, dd=(18000-20000)/20000 = -0.1
			//   i=3: peak=20000, dd=0
			//   i=4: peak=20000, dd=(16000-20000)/20000 = -0.2
			//
			// mean([0, 0, -0.1, 0, -0.2]) = -0.06
			a := cashAccount(
				[]float64{10000, 20000, 18000, 20000, 16000},
				[]float64{100, 110, 100, 110, 100},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.06, 1e-9))
		})

		It("returns 0 for monotonically rising equity (no drawdowns)", func() {
			// Equity: [10000, 10500, 11000, 11500]
			// drawdownSeries: [0, 0, 0, 0] -- no episodes
			a := cashAccount(
				[]float64{10000, 10500, 11000, 11500},
				[]float64{100, 105, 110, 115},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("returns 0 for a single data point", func() {
			a := cashAccount(
				[]float64{10000},
				[]float64{100},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("handles a multi-period drawdown episode", func() {
			// Equity: [10000, 20000, 18000, 16000, 20000]
			//
			// drawdownSeries:
			//   i=0: peak=10000, dd=0
			//   i=1: peak=20000, dd=0
			//   i=2: peak=20000, dd=(18000-20000)/20000 = -0.1
			//   i=3: peak=20000, dd=(16000-20000)/20000 = -0.2
			//   i=4: peak=20000, dd=0
			//
			// mean([0, 0, -0.1, -0.2, 0]) = -0.06
			a := cashAccount(
				[]float64{10000, 20000, 18000, 16000, 20000},
				[]float64{100, 110, 105, 100, 110},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.06, 1e-9))
		})

		It("handles flat benchmark correctly", func() {
			// Flat benchmark should not affect AvgDrawdown (it only uses equity).
			// Equity: [10000, 12000, 10800, 12960]
			// Benchmark: [100, 100, 100, 100]
			//
			// drawdownSeries: [0, 0, -0.1, 0]
			// mean([0, 0, -0.1, 0]) = -0.025
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960},
				[]float64{100, 100, 100, 100},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.025, 1e-9))
		})
	})

	// Cross-check: verify geometric mean helper indirectly by checking
	// a scenario where up-capture uses more than 2 up periods.
	Describe("UpsideCaptureRatio with 3 up periods", func() {
		It("uses all up-benchmark periods in the geometric mean", func() {
			// Equity:    [10000, 12000, 11400, 13680, 13680, 16416]
			// Benchmark: [100,   110,   104.5, 115.5, 109.725, 120.6975]
			//   -- benchmark returns: [0.1, -0.05, ~0.1053, -0.05, 0.1]
			//   -- but let's keep it simpler.
			//
			// Use: equity  = [10000, 12000, 10800, 12960, 15552]
			//      bench   = [100,   110,   99,    108.9, 119.79]
			//
			// Portfolio returns:  [0.2, -0.1, 0.2, 0.2]
			// Benchmark returns:  [0.1, -0.1, 0.1, 0.1]
			//    (108.9 -> 119.79: 10.89/108.9 = 0.1)
			//
			// Up periods: i=0,2,3 (pRet=[0.2,0.2,0.2], bRet=[0.1,0.1,0.1])
			// geoP = (1.2^3)^(1/3) - 1 = 1.2 - 1 = 0.2
			// geoB = (1.1^3)^(1/3) - 1 = 1.1 - 1 = 0.1
			// ratio = 0.2/0.1 = 2.0
			a := cashAccount(
				[]float64{10000, 12000, 10800, 12960, 15552},
				[]float64{100, 110, 99, 108.9, 119.79},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", 2.0, 1e-6))
		})
	})

	Describe("Capture ratio edge cases", func() {
		It("handles portfolio and benchmark moving in opposite directions", func() {
			// Equity:    [10000, 9000, 11000, 10000]
			// Benchmark: [100,   110,  95,    105  ]
			//
			// Portfolio returns:  [-0.1,  ~0.222, ~-0.0909]
			// Benchmark returns:  [0.1,   -0.1364, ~0.1053 ]
			//
			// Up periods (bRet > 0): i=0 (pRet=-0.1, bRet=0.1), i=2 (pRet~-0.0909, bRet~0.1053)
			// Portfolio declines when benchmark rises => negative upside capture ratio.
			a := cashAccount(
				[]float64{10000, 9000, 11000, 10000},
				[]float64{100, 110, 95, 105},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("<", 0.0))
		})

		It("returns 0 for two data points with no up benchmark periods", func() {
			// Equity:    [10000, 10500]
			// Benchmark: [100,   95   ]
			//
			// Single return: benchmark falls (-0.05), no up periods => 0.
			a := cashAccount(
				[]float64{10000, 10500},
				[]float64{100, 95},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})
	})

	Describe("AvgDrawdown edge cases", func() {
		It("handles flat equity curve", func() {
			// Equity:    [10000, 10000, 10000, 10000]
			// Benchmark: [100,   105,   110,   115  ]
			//
			// drawdownSeries: all zeros, no episodes => AvgDrawdown = 0.
			a := cashAccount(
				[]float64{10000, 10000, 10000, 10000},
				[]float64{100, 105, 110, 115},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(Equal(0.0))
		})

		It("handles continuous drawdown without recovery", func() {
			// Equity:    [10000, 9000, 8000, 7000]
			// Benchmark: [100,   95,   90,   85  ]
			//
			// drawdownSeries:
			//   i=0: peak=10000, dd=0
			//   i=1: peak=10000, dd=(9000-10000)/10000 = -0.1
			//   i=2: peak=10000, dd=(8000-10000)/10000 = -0.2
			//   i=3: peak=10000, dd=(7000-10000)/10000 = -0.3
			//
			// mean([0, -0.1, -0.2, -0.3]) = -0.15
			a := cashAccount(
				[]float64{10000, 9000, 8000, 7000},
				[]float64{100, 95, 90, 85},
			)

			v, err := a.PerformanceMetric(portfolio.AvgDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", -0.15, 1e-9))
		})
	})

	// Verify capture ratios with mixed magnitudes.
	Describe("Capture ratios with non-uniform returns", func() {
		It("computes upside capture with different up-period magnitudes", func() {
			// Equity:    [10000, 11000, 10450, 12540]
			// Benchmark: [100,   105,   99.75, 109.725]
			//
			// Portfolio returns: [0.1, -0.05, 0.2]
			// Benchmark returns: [0.05, -0.05, 0.1]
			//   (105 -> 99.75 = -5.25/105 = -0.05)
			//   (99.75 -> 109.725 = 9.975/99.75 = 0.1)
			//
			// Up periods: i=0 (p=0.1, b=0.05), i=2 (p=0.2, b=0.1)
			// geoP = ((1.1)(1.2))^(1/2) - 1 = (1.32)^0.5 - 1
			// geoB = ((1.05)(1.1))^(1/2) - 1 = (1.155)^0.5 - 1
			// ratio = geoP/geoB
			geoP := math.Pow(1.1*1.2, 0.5) - 1
			geoB := math.Pow(1.05*1.1, 0.5) - 1
			expected := geoP / geoB

			a := cashAccount(
				[]float64{10000, 11000, 10450, 12540},
				[]float64{100, 105, 99.75, 109.725},
			)

			v, err := a.PerformanceMetric(portfolio.UpsideCaptureRatio).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(v).To(BeNumerically("~", expected, 1e-6))
		})
	})
})
