package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Distribution Metrics", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	// cashAccount creates an Account with the given equity curve.
	// Since we hold no positions, equity = cash at each UpdatePrices call.
	// We adjust cash via deposit (positive) or withdrawal (negative)
	// transactions so that the cash balance matches the desired equity value.
	cashAccount := func(equityValues []float64) *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(equityValues[0]))
		dates := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), len(equityValues))

		for i, eq := range equityValues {
			if i > 0 {
				diff := eq - equityValues[i-1]
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
			df := buildDF(dates[i], []asset.Asset{spy}, []float64{100}, []float64{100})
			a.UpdatePrices(df)
		}

		return a
	}

	// --- Primary equity curve (mixed returns) ---
	//
	// Prices: [100, 105, 102, 108, 106, 110, 112]  (7 points -> 6 returns)
	//
	// Returns:
	//   r0 = (105-100)/100 =  0.05
	//   r1 = (102-105)/105 = -0.028571428571428571
	//   r2 = (108-102)/102 =  0.058823529411764705
	//   r3 = (106-108)/108 = -0.018518518518518517
	//   r4 = (110-106)/106 =  0.037735849056603774
	//   r5 = (112-110)/110 =  0.018181818181818180
	//
	// mean   = 0.019608541593373264
	// stddev = 0.036241062531594980  (sample, N-1)
	//
	// Positive returns: r0, r2, r4, r5  (4 of 6)
	// Negative returns: r1, r3           (2 of 6)

	Describe("ExcessKurtosis", func() {
		// Kurtosis formula: sum((r_i - mean)^4) / n / sd^4 - 3
		// where sd = sample stddev (N-1 denominator).
		//
		// Hand-calculated expected value: -1.953889061852303

		It("computes excess kurtosis for a mixed equity curve", func() {
			a := cashAccount([]float64{100, 105, 102, 108, 106, 110, 112})
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("~", -1.953889061852303, 1e-9))
		})

		It("returns zero when fewer than 4 returns", func() {
			// 4 prices -> 3 returns, which is < 4 threshold
			a := cashAccount([]float64{100, 105, 102, 108})
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns zero for constant prices (stddev = 0)", func() {
			a := cashAccount([]float64{100, 100, 100, 100, 100})
			result := a.PerformanceMetric(portfolio.ExcessKurtosis).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})
	})

	Describe("Skewness", func() {
		// Skewness formula: sum((r_i - mean)^3) / n / sd^3
		// where sd = sample stddev (N-1 denominator).
		//
		// Hand-calculated expected value: -0.2553770356869169

		It("computes skewness for a mixed equity curve", func() {
			a := cashAccount([]float64{100, 105, 102, 108, 106, 110, 112})
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", -0.2553770356869169, 1e-9))
		})

		It("returns zero when fewer than 3 returns", func() {
			// 3 prices -> 2 returns, which is < 3 threshold
			a := cashAccount([]float64{100, 105, 102})
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns zero for constant prices (stddev = 0)", func() {
			a := cashAccount([]float64{100, 100, 100, 100, 100})
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("is approximately zero for symmetric returns", func() {
			// Returns: [+0.05, -0.05, +0.03, -0.03, +0.02, -0.02]
			// Paired symmetric returns -> mean ~ 0, sum(d^3) ~ 0 -> skewness ~ 0.
			// Prices built by compounding: 100 * (1+r0) * (1+r1) * ...
			a := cashAccount([]float64{
				100.0,
				105.0,           // +5%
				99.75,           // -5%
				102.7425,        // +3%
				99.660225,       // -3%
				101.6534295,     // +2%
				99.62036091,     // -2%
			})
			result := a.PerformanceMetric(portfolio.Skewness).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-9))
		})
	})

	Describe("NPositivePeriods", func() {
		// Formula: count(r > 0) / len(r)
		// Mixed curve: 4 positive out of 6 returns = 2/3

		It("computes fraction of positive periods", func() {
			a := cashAccount([]float64{100, 105, 102, 108, 106, 110, 112})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 4.0/6.0, 1e-12))
		})

		It("returns 1.0 when all returns are positive", func() {
			// Strictly increasing prices -> all returns positive.
			a := cashAccount([]float64{100, 102, 105, 108, 112})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("returns 0.0 when all returns are negative", func() {
			// Strictly decreasing prices -> all returns negative.
			a := cashAccount([]float64{100, 98, 95, 92, 88})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns 1.0 for a single positive return", func() {
			a := cashAccount([]float64{100, 105})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 1.0, 1e-12))
		})

		It("returns 0.0 when there are no returns (single price)", func() {
			a := cashAccount([]float64{100})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns 0.5 for symmetric returns", func() {
			// 3 positive, 3 negative -> 0.5
			a := cashAccount([]float64{
				100.0, 105.0, 99.75, 102.7425,
				99.660225, 101.6534295, 99.62036091,
			})
			result := a.PerformanceMetric(portfolio.NPositivePeriods).Value()
			Expect(result).To(BeNumerically("~", 0.5, 1e-12))
		})
	})

	Describe("GainLossRatio", func() {
		// Formula: mean(positive_returns) / |mean(negative_returns)|
		//
		// Mixed curve positive returns: [0.05, 0.058823..., 0.037735..., 0.018181...]
		//   mean_pos = 0.041185299162546665
		// Negative returns: [-0.028571..., -0.018518...]
		//   mean_neg = -0.023544973544973542
		// Ratio = 0.041185299162546665 / 0.023544973544973542 = 1.7492183239823191

		It("computes gain/loss ratio for mixed returns", func() {
			a := cashAccount([]float64{100, 105, 102, 108, 106, 110, 112})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 1.7492183239823191, 1e-9))
		})

		It("returns zero when all returns are positive (no losses)", func() {
			a := cashAccount([]float64{100, 102, 105, 108, 112})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns zero when all returns are negative (no gains)", func() {
			a := cashAccount([]float64{100, 98, 95, 92, 88})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns zero with a single return (only one side)", func() {
			a := cashAccount([]float64{100, 105})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})

		It("returns 1.0 for symmetric returns", func() {
			// Paired +/- returns with equal magnitude -> mean_pos = mean_neg -> ratio = 1.0
			a := cashAccount([]float64{
				100.0, 105.0, 99.75, 102.7425,
				99.660225, 101.6534295, 99.62036091,
			})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 1.0, 1e-9))
		})

		It("returns zero when there are no returns", func() {
			a := cashAccount([]float64{100})
			result := a.PerformanceMetric(portfolio.GainLossRatio).Value()
			Expect(result).To(BeNumerically("~", 0.0, 1e-12))
		})
	})
})
