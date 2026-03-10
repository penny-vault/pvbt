package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Return Metrics", func() {
	var (
		spy asset.Asset
		bm  asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bm = asset.Asset{CompositeFigi: "BENCH", Ticker: "BENCH"}
	})

	// buildReturnAccount creates an account with the given equity curve.
	// It uses DepositTransaction (positive) and WithdrawalTransaction
	// (negative) to move cash so equity = cash.
	buildReturnAccount := func(dates []time.Time, equity []float64) *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(equity[0]))
		for i, d := range dates {
			if i > 0 {
				diff := equity[i] - equity[i-1]
				if diff > 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.DepositTransaction,
						Amount: diff,
					})
				} else if diff < 0 {
					a.Record(portfolio.Transaction{
						Date:   d,
						Type:   portfolio.WithdrawalTransaction,
						Amount: diff,
					})
				}
			}
			df := buildDF(d, []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df)
		}
		return a
	}

	Describe("TWRR", func() {
		It("compounds sub-period returns for a 5-point equity curve", func() {
			// Equity: [100, 110, 105, 115, 120]
			// Returns: 10/100=0.10, -5/110=-1/22, 10/105=2/21, 5/115=1/23
			// Product: (110/100)*(105/110)*(115/105)*(120/115) = 120/100 = 1.20
			// TWRR = 1.20 - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			a := buildReturnAccount(dates, []float64{100, 110, 105, 115, 120})

			result := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(result).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("computes cumulative return at each point in the series", func() {
			// Equity: [100, 110, 105, 115, 120]
			// Period returns: r0=10/100, r1=-5/110, r2=10/105, r3=5/115
			// Cumulative product at each step:
			//   cum[0] = (110/100) - 1 = 0.10
			//   cum[1] = (110/100)*(105/110) - 1 = 105/100 - 1 = 0.05
			//   cum[2] = (105/100)*(115/105) - 1 = 115/100 - 1 = 0.15
			//   cum[3] = (115/100)*(120/115) - 1 = 120/100 - 1 = 0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			a := buildReturnAccount(dates, []float64{100, 110, 105, 115, 120})

			series := a.PerformanceMetric(portfolio.TWRR).Series()
			Expect(series).To(HaveLen(4))
			Expect(series[0]).To(BeNumerically("~", 0.10, 1e-9))
			Expect(series[1]).To(BeNumerically("~", 0.05, 1e-9))
			Expect(series[2]).To(BeNumerically("~", 0.15, 1e-9))
			Expect(series[3]).To(BeNumerically("~", 0.20, 1e-9))
		})

		It("returns negative total return for a declining equity curve", func() {
			// Equity: [100, 90, 80]
			// Product: (90/100)*(80/90) = 80/100 = 0.80
			// TWRR = 0.80 - 1 = -0.20
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildReturnAccount(dates, []float64{100, 90, 80})

			result := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(result).To(BeNumerically("~", -0.20, 1e-9))
		})

		It("returns 0 for a single data point", func() {
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 1)
			a := buildReturnAccount(dates, []float64{100})

			result := a.PerformanceMetric(portfolio.TWRR).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})
	})

	Describe("MWRR", func() {
		It("matches TWRR when there are no mid-stream cash flows", func() {
			// Build account manually with DividendTransaction for organic
			// growth so MWRR sees only the initial deposit as a cash flow.
			// Equity: [10000, 11000] over 367 days (Jan 2 2024 to Jan 3 2025).
			// Flows: -10000 at t0 (synthetic), +11000 at t1.
			// XIRR solves: -10000 + 11000/(1+r)^(367/365) = 0
			//   (1+r)^(367/365) = 1.1
			//   r = 1.1^(365/367) - 1 = 0.09942880667...
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000))
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			// Organic growth via dividend (not an external cash flow for MWRR).
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   portfolio.DividendTransaction,
				Amount: 1000,
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			result := a.PerformanceMetric(portfolio.MWRR).Value()
			expected := math.Pow(1.1, 365.0/367.0) - 1
			Expect(result).To(BeNumerically("~", expected, 1e-9))
		})

		It("differs from TWRR when there is a mid-stream deposit", func() {
			// Day 0: deposit 10000, equity=10000
			// Day 183 (Jul 3): deposit 500 + deposit 5000 -> equity 15500
			// Day 366 (Jan 3 next year): deposit 1000 -> equity 16500
			//
			// MWRR flows (from MWRR source):
			//   Initial deposit: -10000 at t0 (Jan 2 2024), date is zero so mapped to times[0]
			//   Deposit 500 + deposit 5000: -5500 at t1 (Jul 3 2024)
			//   Deposit 1000: -1000 at t2 (Jan 3 2025)
			//   Terminal value: +16500 at t2 (Jan 3 2025)
			//
			// Days from t0: d0=0, d1=183, d2=366
			// NPV(r) = -10000 + (-5500)/(1+r)^(183/365) + 15500/(1+r)^(366/365) = 0
			dates := []time.Time{
				time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC),
				time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
			}

			a := portfolio.New(portfolio.WithCash(10_000))

			// Day 0: initial state
			df0 := buildDF(dates[0], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df0)

			// Day 183: deposit 500 (growth) then deposit 5000 -> equity 15500
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   portfolio.DepositTransaction,
				Amount: 500, // growth
			})
			a.Record(portfolio.Transaction{
				Date:   dates[1],
				Type:   portfolio.DepositTransaction,
				Amount: 5000, // mid-stream deposit
			})
			df1 := buildDF(dates[1], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df1)

			// Day 366: deposit 1000 (growth) -> equity 16500
			a.Record(portfolio.Transaction{
				Date:   dates[2],
				Type:   portfolio.DepositTransaction,
				Amount: 1000, // growth
			})
			df2 := buildDF(dates[2], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df2)

			// Verify equity curve is as expected.
			Expect(a.EquityCurve()).To(Equal([]float64{10_000, 15_500, 16_500}))

			result := a.PerformanceMetric(portfolio.MWRR).Value()
			twrrResult := a.PerformanceMetric(portfolio.TWRR).Value()

			// TWRR = (15500/10000)*(16500/15500) - 1 = 1.65 - 1 = 0.65
			// Wait -- equity curve is [10000, 15500, 16500], so returns include the deposit.
			// TWRR sees returns = (15500-10000)/10000 = 0.55, (16500-15500)/15500 = 0.0645...
			// TWRR = 1.55 * 1.064516... - 1 = 0.65
			Expect(twrrResult).To(BeNumerically("~", 0.65, 1e-9))

			// MWRR should be different from TWRR due to the mid-stream deposit.
			Expect(result).NotTo(BeNumerically("~", twrrResult, 0.01))
			// MWRR should be positive (portfolio grew).
			Expect(result).To(BeNumerically(">", 0.0))
		})

		It("returns 0 for a single data point", func() {
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 1)
			a := buildReturnAccount(dates, []float64{10_000})

			result := a.PerformanceMetric(portfolio.MWRR).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})
	})

	Describe("ActiveReturn", func() {
		// buildAccountWithBenchmark creates an account with both a portfolio equity
		// curve and benchmark prices.
		buildAccountWithBenchmark := func(
			dates []time.Time,
			equity []float64,
			benchPrices []float64,
		) *portfolio.Account {
			a := portfolio.New(
				portfolio.WithCash(equity[0]),
				portfolio.WithBenchmark(bm),
			)
			for i, d := range dates {
				if i > 0 {
					diff := equity[i] - equity[i-1]
					if diff > 0 {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   portfolio.DepositTransaction,
							Amount: diff,
						})
					} else if diff < 0 {
						a.Record(portfolio.Transaction{
							Date:   d,
							Type:   portfolio.WithdrawalTransaction,
							Amount: diff,
						})
					}
				}
				df := buildDF(d,
					[]asset.Asset{spy, bm},
					[]float64{100, benchPrices[i]},
					[]float64{100, benchPrices[i]},
				)
				a.UpdatePrices(df)
			}
			return a
		}

		It("computes portfolio total return minus benchmark total return", func() {
			// Portfolio equity: [1000, 1100, 1200]
			// Benchmark:        [50,   52,   54]
			//
			// ActiveReturn.Compute uses (end/start)-1 for each:
			//   portReturn  = 1200/1000 - 1 = 0.20
			//   benchReturn = 54/50 - 1     = 0.08
			//   active      = 0.20 - 0.08   = 0.12
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{1000, 1100, 1200},
				[]float64{50, 52, 54},
			)

			result := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			Expect(result).To(BeNumerically("~", 0.12, 1e-9))
		})

		It("returns 0 when portfolio perfectly tracks benchmark", func() {
			// Both have the same percentage returns at each step.
			// Portfolio: [1000, 1100, 1210]  -> returns: 10%, 10%
			// Benchmark: [50,   55,   60.5]  -> returns: 10%, 10%
			//
			// portReturn  = 1210/1000 - 1 = 0.21
			// benchReturn = 60.5/50 - 1   = 0.21
			// active      = 0.0
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{1000, 1100, 1210},
				[]float64{50, 55, 60.5},
			)

			result := a.PerformanceMetric(portfolio.ActiveReturn).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})

		It("computes cumulative active return series as portfolio minus benchmark cumulative returns", func() {
			// Portfolio equity: [100, 110, 105]
			// Benchmark:        [50,  52,  54]
			//
			// Portfolio returns: r0=10/100=0.10, r1=-5/110=-1/22
			// Benchmark returns: r0=2/50=0.04,   r1=2/52=1/26
			//
			// Cumulative portfolio: cum_p[0]=0.10, cum_p[1]=(110/100)*(105/110)-1=0.05
			// Cumulative benchmark: cum_b[0]=0.04, cum_b[1]=(52/50)*(54/52)-1=0.08
			//
			// Series[i] = (cumPort - 1) - (cumBench - 1) = cumPort - cumBench
			//   series[0] = 0.10 - 0.04 = 0.06
			//   series[1] = 0.05 - 0.08 = -0.03
			dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 3)
			a := buildAccountWithBenchmark(dates,
				[]float64{100, 110, 105},
				[]float64{50, 52, 54},
			)

			series := a.PerformanceMetric(portfolio.ActiveReturn).Series()
			Expect(series).To(HaveLen(2))
			Expect(series[0]).To(BeNumerically("~", 0.06, 1e-9))
			Expect(series[1]).To(BeNumerically("~", -0.03, 1e-9))
		})
	})
})
