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

package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Risk-Adjusted Metrics", func() {
	var (
		spy asset.Asset
		bil asset.Asset
	)

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
		bil = asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
	})

	// buildAccount creates an account with the given SPY and BIL price series.
	// It buys 5 shares of SPY at spyPrices[0], using WithCash = 5 * spyPrices[0].
	// Equity curve = 5 * spyPrices[i] (no remaining cash).
	buildAccount := func(spyPrices, bilPrices []float64) *portfolio.Account {
		n := len(spyPrices)
		times := daySeq(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), n)

		acct := portfolio.New(
			portfolio.WithCash(5*spyPrices[0], time.Time{}),
			portfolio.WithBenchmark(spy),
			portfolio.WithRiskFree(bil),
		)

		acct.Record(portfolio.Transaction{
			Date:   times[0],
			Asset:  spy,
			Type:   portfolio.BuyTransaction,
			Qty:    5,
			Price:  spyPrices[0],
			Amount: -5 * spyPrices[0],
		})

		for i := range n {
			df := buildDF(times[i],
				[]asset.Asset{spy, bil},
				[]float64{spyPrices[i], bilPrices[i]},
				[]float64{spyPrices[i], bilPrices[i]},
			)
			acct.UpdatePrices(df)
		}

		return acct
	}

	// ----------------------------------------------------------------
	// Primary scenario: 6-point equity curve with two dips.
	//
	// SPY prices: [100, 105, 98, 103, 97, 110]
	// BIL prices: [100, 100.01, 100.02, 100.03, 100.04, 100.05]
	// Equity curve (5 * SPY): [500, 525, 490, 515, 485, 550]
	//
	// Equity returns:
	//   r[0] = (525-500)/500  = 0.05
	//   r[1] = (490-525)/525  = -0.066666...
	//   r[2] = (515-490)/490  =  0.051020...
	//   r[3] = (485-515)/515  = -0.058252...
	//   r[4] = (550-485)/485  =  0.134020...
	//
	// RF returns (BIL):
	//   rf[0] = 0.01/100       = 0.0001
	//   rf[1] = 0.01/100.01    ~ 9.999e-5
	//   rf[2] = 0.01/100.02    ~ 9.998e-5
	//   rf[3] = 0.01/100.03    ~ 9.997e-5
	//   rf[4] = 0.01/100.04    ~ 9.996e-5
	//
	// Excess returns: er[i] = r[i] - rf[i]
	//   er[0] ~ 0.04990, er[1] ~ -0.06677,
	//   er[2] ~ 0.05092, er[3] ~ -0.05835, er[4] ~ 0.13392
	//
	// Negative excess returns: er[1], er[3]
	//
	// annualizationFactor = 252 (daily data, avg gap < 20 days)
	// ----------------------------------------------------------------

	Describe("StdDev", func() {
		It("returns annualized standard deviation of equity returns", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// stddev(r) = sqrt(variance(r))
			// mean(r) = (0.05 + (-0.06667) + 0.05102 + (-0.05825) + 0.13402) / 5
			//         = 0.11012 / 5 = 0.02202...
			// variance(r) = sum((r[i]-mean)^2) / 4  (N-1 = 4)
			// stddev(r) ~ 0.08438
			// StdDev = stddev(r) * sqrt(252) ~ 1.33942
			val, err := acct.PerformanceMetric(portfolio.StdDev).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 1.3394, 1e-3))
		})

		It("returns the return series from ComputeSeries", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			series, err := acct.PerformanceMetric(portfolio.StdDev).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(series).To(HaveLen(5))
			Expect(series[0]).To(BeNumerically("~", 0.05, 1e-10))
			Expect(series[1]).To(BeNumerically("~", -0.06667, 1e-4))
		})
	})

	Describe("MaxDrawdown", func() {
		It("returns the largest peak-to-trough decline", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// Drawdown series from equity [500, 525, 490, 515, 485, 550]:
			//   peak=500 -> dd=0
			//   peak=525 -> dd=0
			//   peak=525 -> dd=(490-525)/525 = -0.06667
			//   peak=525 -> dd=(515-525)/525 = -0.01905
			//   peak=525 -> dd=(485-525)/525 = -0.07619
			//   peak=550 -> dd=0
			// MaxDrawdown = -0.07619
			val, err := acct.PerformanceMetric(portfolio.MaxDrawdown).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", -0.07619, 1e-4))
		})

		It("returns the full drawdown series from ComputeSeries", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			series, err := acct.PerformanceMetric(portfolio.MaxDrawdown).Series()
			Expect(err).NotTo(HaveOccurred())
			Expect(series).To(HaveLen(6))
			Expect(series[0]).To(BeNumerically("~", 0.0, 1e-10))
			Expect(series[1]).To(BeNumerically("~", 0.0, 1e-10))
			Expect(series[2]).To(BeNumerically("~", -0.06667, 1e-4))
			Expect(series[3]).To(BeNumerically("~", -0.01905, 1e-4))
			Expect(series[4]).To(BeNumerically("~", -0.07619, 1e-4))
			Expect(series[5]).To(BeNumerically("~", 0.0, 1e-10))
		})
	})

	Describe("DownsideDeviation", func() {
		It("returns annualized stddev of negative excess returns", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// Negative excess returns: er[1] ~ -0.06677, er[3] ~ -0.05835
			// mean(neg) = (-0.06677 + -0.05835) / 2 ~ -0.06256
			// variance(neg) = ((neg[0]-mean)^2 + (neg[1]-mean)^2) / 1
			// stddev(neg) ~ 0.005951
			// DownsideDeviation = stddev(neg) * sqrt(252) ~ 0.09445
			val, err := acct.PerformanceMetric(portfolio.DownsideDeviation).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 0.09445, 1e-3))
		})
	})

	Describe("Sharpe", func() {
		It("returns annualized risk-adjusted return", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// mean(er) ~ 0.02440
			// stddev(er) ~ 0.08438 (same as stddev(r) approximately)
			// Sharpe = mean(er)/stddev(er) * sqrt(252) ~ 4.1249
			val, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 4.1249, 1e-2))
		})
	})

	Describe("Sortino", func() {
		It("returns annualized return per unit of downside deviation", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// mean(er) ~ 0.02193
			// downside_deviation = sqrt(sum(min(er_i,0)^2) / N) ~ 0.03965
			// Sortino = mean(er)/dd * sqrt(252) ~ 8.778
			val, err := acct.PerformanceMetric(portfolio.Sortino).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 8.778, 0.01))
		})
	})

	Describe("Calmar", func() {
		It("returns CAGR divided by max drawdown magnitude", func() {
			acct := buildAccount(
				[]float64{100, 105, 98, 103, 97, 110},
				[]float64{100, 100.01, 100.02, 100.03, 100.04, 100.05},
			)

			// daySeq from Jan 2 2025 (Thu): Jan 2, 3, 6, 7, 8, 9
			// years = (Jan 9 - Jan 2).days / 365.25 = 7 / 365.25 ~ 0.01916
			// cagr = (550/500)^(1/0.01916) - 1 ~ 143.48
			// maxDD = 0.07619
			// Calmar = 143.48 / 0.07619 ~ 1883.2
			val, err := acct.PerformanceMetric(portfolio.Calmar).Value()
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 1883.2, 1.0))
		})
	})

	// ----------------------------------------------------------------
	// Edge cases
	// ----------------------------------------------------------------

	Context("edge cases", func() {
		Context("flat equity curve (all same values)", func() {
			It("returns StdDev = 0", func() {
				// SPY stays at 100 -> equity stays at 500
				acct := buildAccount(
					[]float64{100, 100, 100, 100, 100},
					[]float64{100, 100, 100, 100, 100},
				)
				v, err := acct.PerformanceMetric(portfolio.StdDev).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns MaxDrawdown = 0", func() {
				acct := buildAccount(
					[]float64{100, 100, 100, 100, 100},
					[]float64{100, 100, 100, 100, 100},
				)
				v, err := acct.PerformanceMetric(portfolio.MaxDrawdown).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns Sharpe = 0 when stddev of excess returns is 0", func() {
				// Flat SPY and flat BIL -> all returns 0, all excess returns 0
				acct := buildAccount(
					[]float64{100, 100, 100, 100, 100},
					[]float64{100, 100, 100, 100, 100},
				)
				v, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns Calmar = 0 when max drawdown is 0", func() {
				acct := buildAccount(
					[]float64{100, 100, 100, 100, 100},
					[]float64{100, 100, 100, 100, 100},
				)
				v, err := acct.PerformanceMetric(portfolio.Calmar).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})
		})

		Context("monotonically rising curve", func() {
			It("returns MaxDrawdown = 0", func() {
				acct := buildAccount(
					[]float64{100, 102, 104, 106, 108},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				v, err := acct.PerformanceMetric(portfolio.MaxDrawdown).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns Calmar = 0 when no drawdown exists", func() {
				acct := buildAccount(
					[]float64{100, 102, 104, 106, 108},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				v, err := acct.PerformanceMetric(portfolio.Calmar).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})
		})

		Context("all excess returns positive", func() {
			It("returns DownsideDeviation = 0", func() {
				// Large positive equity returns dwarf tiny RF returns
				acct := buildAccount(
					[]float64{100, 110, 121, 133, 146},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				v, err := acct.PerformanceMetric(portfolio.DownsideDeviation).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns Sortino = 0", func() {
				acct := buildAccount(
					[]float64{100, 110, 121, 133, 146},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				v, err := acct.PerformanceMetric(portfolio.Sortino).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})
		})

		Context("negative mean excess return", func() {
			It("returns negative Sharpe", func() {
				// Declining equity [100,95,90,85,80] with rising risk-free [100,101,102,103,104]
				// Equity returns are all negative; RF returns are positive -> excess returns all negative
				// mean(er) < 0, so Sharpe < 0
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 101, 102, 103, 104},
				)
				val, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNumerically("<", 0.0))
			})
		})

		Context("all returns negative (declining equity)", func() {
			It("returns DownsideDeviation > 0", func() {
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				val, err := acct.PerformanceMetric(portfolio.DownsideDeviation).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNumerically(">", 0.0))
			})

			It("returns negative Sortino", func() {
				acct := buildAccount(
					[]float64{100, 95, 90, 85, 80},
					[]float64{100, 100.01, 100.02, 100.03, 100.04},
				)
				val, err := acct.PerformanceMetric(portfolio.Sortino).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNumerically("<", 0.0))
			})
		})

		Context("two data points (single return)", func() {
			It("returns StdDev = 0 (sample variance with N-1=0)", func() {
				acct := buildAccount(
					[]float64{100, 110},
					[]float64{100, 100.01},
				)
				v, err := acct.PerformanceMetric(portfolio.StdDev).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})

			It("returns Sharpe = 0 when stddev is 0", func() {
				acct := buildAccount(
					[]float64{100, 110},
					[]float64{100, 100.01},
				)
				v, err := acct.PerformanceMetric(portfolio.Sharpe).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})
		})

		Context("single data point", func() {
			It("returns 0 for all metrics", func() {
				acct := buildAccount(
					[]float64{100},
					[]float64{100},
				)

				v, err := acct.PerformanceMetric(portfolio.StdDev).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
				v, err = acct.PerformanceMetric(portfolio.MaxDrawdown).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
				v, err = acct.PerformanceMetric(portfolio.DownsideDeviation).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
				v, err = acct.PerformanceMetric(portfolio.Sharpe).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
				v, err = acct.PerformanceMetric(portfolio.Sortino).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
				v, err = acct.PerformanceMetric(portfolio.Calmar).Value()
				Expect(err).NotTo(HaveOccurred())
				Expect(v).To(Equal(0.0))
			})
		})
	})

	Describe("Missing configuration errors", func() {
		Context("when risk-free rate is not configured", func() {
			It("returns ErrNoRiskFreeRate for risk-free-dependent metrics", func() {
				acct := buildAccountFromEquity([]float64{100, 105, 98, 103, 97, 110})
				for _, m := range []portfolio.PerformanceMetric{
					portfolio.Sharpe,
					portfolio.Sortino,
					portfolio.SmartSharpe,
					portfolio.SmartSortino,
					portfolio.ProbabilisticSharpe,
					portfolio.DownsideDeviation,
					portfolio.Treynor,
					portfolio.Alpha,
				} {
					v, err := m.Compute(acct, nil)
					Expect(err).To(MatchError(portfolio.ErrNoRiskFreeRate), m.Name())
					Expect(v).To(Equal(0.0), m.Name())
				}
			})
		})

		Context("when benchmark is not configured", func() {
			It("returns ErrNoBenchmark for benchmark-dependent metrics", func() {
				acct := buildAccountFromEquity([]float64{100, 105, 98, 103, 97, 110})
				for _, m := range []portfolio.PerformanceMetric{
					portfolio.Beta,
					portfolio.TrackingError,
					portfolio.InformationRatio,
					portfolio.RSquared,
					portfolio.UpsideCaptureRatio,
					portfolio.DownsideCaptureRatio,
					portfolio.ActiveReturn,
				} {
					v, err := m.Compute(acct, nil)
					Expect(err).To(MatchError(portfolio.ErrNoBenchmark), m.Name())
					Expect(v).To(Equal(0.0), m.Name())
				}
			})
		})

		Context("aggregate methods with missing configuration", func() {
			It("returns partial Summary with joined errors", func() {
				acct := buildAccountFromEquity([]float64{100, 105, 98, 103, 97, 110})
				s, err := acct.Summary()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("risk-free rate not configured"))
				// TWRR, MWRR, Calmar, MaxDrawdown, StdDev should still compute
				Expect(s.TWRR).NotTo(Equal(0.0))
				// Sharpe, Sortino should be zero (errored)
				Expect(s.Sharpe).To(Equal(0.0))
				Expect(s.Sortino).To(Equal(0.0))
			})

			It("returns partial RiskMetrics with joined errors", func() {
				acct := buildAccountFromEquity([]float64{100, 105, 98, 103, 97, 110})
				r, err := acct.RiskMetrics()
				Expect(err).To(HaveOccurred())
				// Beta, Alpha, etc. should be zero (errored)
				Expect(r.Beta).To(Equal(0.0))
				Expect(r.Alpha).To(Equal(0.0))
			})
		})
	})
})
