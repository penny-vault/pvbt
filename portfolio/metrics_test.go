// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
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

package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/portfolio"
)

var _ = Describe("Metrics", func() {
	var (
		perf *portfolio.Performance
		tz   *time.Location
		mc   [][]float64
	)

	BeforeEach(func() {
		tz = common.GetTimezone()
	})

	Describe("when calculating active return", func() {
		Context("with 10 years of performance", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2521; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 4.0
					benchmarkValue += 3.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.ActiveReturn(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.ActiveReturn(2531))).Should(BeTrue())
			})

			It("should have active return for 1-day", func() {
				Expect(perf.ActiveReturn(1)).Should(BeNumerically("~", 2.838e-05))
			})

			It("should have active return for 1-month", func() {
				Expect(perf.ActiveReturn(21)).Should(BeNumerically("~", 0.000600222919))
			})

			It("should have active return for 1-yr", func() {
				Expect(perf.ActiveReturn(252)).Should(BeNumerically("~", 0.00786306073))
			})

			It("should have active return for 2-yr", func() {
				Expect(perf.ActiveReturn(504)).Should(BeNumerically("~", 0.00828326496))
			})

			It("should have active return for 10-yr", func() {
				Expect(perf.ActiveReturn(2520)).Should(BeNumerically("~", 0.0142840859))
			})
		})
	})

	Describe("when calculating alpha", func() {
		Context("with simulated performance data with portfolioValue = benchmarkValue", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				riskFreeValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
						RiskFreeValue:  riskFreeValue,
					})

					value += 10.0
					benchmarkValue += 10.0
					riskFreeValue += 1.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.Alpha(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.Alpha(2531))).Should(BeTrue())
			})

			It("should have alpha for 1-yr", func() {
				Expect(perf.Alpha(252)).Should(BeNumerically("~", 0))
			})

			It("should have alpha for 3-yr", func() {
				Expect(perf.Alpha(756)).Should(BeNumerically("~", 0))
			})

			It("should have alpha for 5-yr", func() {
				Expect(perf.Alpha(1260)).Should(BeNumerically("~", 0))
			})
		})
	})

	Describe("when calculating average draw-down", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}

			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.AverageDrawDown(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.AverageDrawDown(11, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have drawdowns when only one drawdown occurs", func() {
				Expect(perf.AverageDrawDown(4, portfolio.STRATEGY)).Should(BeNumerically("~", -0.08909091))
			})

		})

		Context("with multiple drawdowns in simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})
			It("should have average drawdowns when multiple drawdowns occur", func() {
				Expect(perf.AverageDrawDown(9, portfolio.STRATEGY)).Should(BeNumerically("~", -0.06816035))
			})
		})
	})

	Describe("when calculating all draw-down", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should be 0 for period of 0", func() {
				Expect(perf.AllDrawDowns(0, portfolio.STRATEGY)).To(HaveLen(0))
			})

			It("should be 0 for period of 1", func() {
				Expect(perf.AllDrawDowns(1, portfolio.STRATEGY)).To(HaveLen(0))
			})

			It("should be 2 for period greater than # of measurements", func() {
				Expect(perf.AllDrawDowns(11, portfolio.STRATEGY)).To(HaveLen(2))
			})

			It("should have 2 drawdowns", func() {
				Expect(perf.AllDrawDowns(9, portfolio.STRATEGY)).To(HaveLen(2))
			})

			It("should have drawdown[0] with start date of 2/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[0]
				Expect(dd.Begin).To(Equal(time.Date(2010, 2, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[0] with end date of 4/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[0]
				Expect(dd.End).To(Equal(time.Date(2010, 3, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[0] with LossPercent of -.08", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[0]
				Expect(dd.LossPercent).Should(BeNumerically("~", -0.08909091))
			})

			It("should have drawdown[0] with Recovery time of 4/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[0]
				Expect(dd.Recovery).To(Equal(time.Date(2010, 4, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[1] with start date of 6/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[1]
				Expect(dd.Begin).To(Equal(time.Date(2010, 6, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[1] with end date of 9/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[1]
				Expect(dd.End).To(Equal(time.Date(2010, 9, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[1] with LossPercent of -.05", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[1]
				Expect(dd.LossPercent).Should(BeNumerically("~", -0.04722979))
			})

			It("should have drawdown[1] with Recovery time of 10/1/2010", func() {
				dd := perf.AllDrawDowns(9, portfolio.STRATEGY)[1]
				Expect(dd.Recovery).To(Equal(time.Date(2010, 10, 1, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("when calculating average ulcer index", func() {
		Context("with simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:       time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						UlcerIndex: 1.0,
					},
					{
						Time:       time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						UlcerIndex: 2.0,
					},
					{
						Time:       time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						UlcerIndex: 3.0,
					},
					{
						Time:       time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						UlcerIndex: 4.0,
					},
					{
						Time:       time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						UlcerIndex: 5.0,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.AvgUlcerIndex(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.AvgUlcerIndex(2531))).Should(BeTrue())
			})

			It("should have value over 2 periods", func() {
				Expect(perf.AvgUlcerIndex(2)).Should(BeNumerically("~", 4.0))
			})

			It("should have value over whole window", func() {
				Expect(perf.AvgUlcerIndex(4)).Should(BeNumerically("~", 3.0))
			})
		})
	})

	Describe("when calculating beta", func() {
		Context("with simulated performance data with portfolioValue = benchmarkValue", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 10.0
					benchmarkValue += 10.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.Beta(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.Beta(2531))).Should(BeTrue())
			})

			It("should have beta for 1-yr", func() {
				Expect(perf.Beta(252)).Should(BeNumerically("~", 1))
			})

			It("should have beta for 3-yr", func() {
				Expect(perf.Beta(756)).Should(BeNumerically("~", 1))
			})

			It("should have beta for 5-yr", func() {
				Expect(perf.Beta(1260)).Should(BeNumerically("~", 1))
			})
		})
	})

	Describe("when calculating calmar ratio", func() {
		Context("with simulated performance data with portfolioValue = benchmarkValue", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 10.0
					benchmarkValue += 10.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.CalmarRatio(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.CalmarRatio(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have value for 1-yr", func() {
				Expect(perf.CalmarRatio(252, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0771349862))
			})

			It("should have value for 3-yr", func() {
				Expect(perf.CalmarRatio(756, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0840169073))
			})

			It("should have value for 5-yr", func() {
				Expect(perf.CalmarRatio(1260, portfolio.STRATEGY)).Should(BeNumerically("~", 0.092710428))
			})
		})

		Context("with simulated performance data that has draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.CalmarRatio(9, portfolio.STRATEGY)).Should(BeNumerically("~", 1.13367347))
			})
		})

	})

	Describe("when calculating downside deviation", func() {
		Context("with simulated performance data constant loss", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 1, 1, 16, 0, 0, 0, tz),
						Value:               10_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 2, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 3, 1, 16, 0, 0, 0, tz),
						Value:               10_150.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_150.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 4, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 5, 1, 16, 0, 0, 0, tz),
						Value:               10_275.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_275.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 6, 1, 16, 0, 0, 0, tz),
						Value:               10_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 7, 1, 16, 0, 0, 0, tz),
						Value:               10_300.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_300.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 8, 1, 16, 0, 0, 0, tz),
						Value:               10_700.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_700.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 9, 1, 16, 0, 0, 0, tz),
						Value:               10_750.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_750.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 10, 1, 16, 0, 0, 0, tz),
						Value:               10_800.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_800.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 11, 1, 16, 0, 0, 0, tz),
						Value:               10_900.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_900.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 12, 1, 16, 0, 0, 0, tz),
						Value:               10_950.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_950.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 1, 1, 16, 0, 0, 0, tz),
						Value:               11_000.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_000.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 2, 1, 16, 0, 0, 0, tz),
						Value:               11_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 3, 1, 16, 0, 0, 0, tz),
						Value:               11_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 4, 1, 16, 0, 0, 0, tz),
						Value:               11_500.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_500.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 5, 1, 16, 0, 0, 0, tz),
						Value:               11_550.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_550.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 6, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 7, 1, 16, 0, 0, 0, tz),
						Value:               11_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 8, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.DownsideDeviation(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.DownsideDeviation(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have value", func() {
				Expect(perf.DownsideDeviation(19, portfolio.STRATEGY)).Should(BeNumerically("~", 0.01616526))
			})
		})
	})

	Describe("when calculating withdrawal rates", func() {
		Context("with 24 months of simulated monthly returns", func() {
			BeforeEach(func() {
				timeSeries := []float64{
					.008, .01, .007, -.005, .0089, .02, -.03, .04, .01, -.09, .08, .05,
					.006, .02, .009, -.002, .0089, .0004, -.002, .01, .01, -.01, .008, .008,
				}
				mc = portfolio.CircularBootstrap(timeSeries, 12, 5000, 360)
			})

			It("should have a dynamic withdrawal rate", func() {
				d := portfolio.DynamicWithdrawalRate(mc, 0.03)
				Expect(d).Should(BeNumerically("~", 0.058, 1e-3))
			})

			It("should have a perpetual withdrawal rate", func() {
				d := portfolio.PerpetualWithdrawalRate(mc, 0.03)
				Expect(d).Should(BeNumerically("~", .056, 1e-3))
			})

			It("should have a safe withdrawal rate", func() {
				d := portfolio.SafeWithdrawalRate(mc, 0.03)
				Expect(d).Should(BeNumerically("~", .07, 1e-3))
			})
		})
	})

	Describe("when calculating excess kurtosis", func() {
		Context("with simulated performance data with constant increase", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 10.0
					benchmarkValue += 5.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.ExcessKurtosis(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.ExcessKurtosis(2531))).Should(BeTrue())
			})

			It("should have value for 1-yr", func() {
				Expect(perf.ExcessKurtosis(252)).Should(BeNumerically("~", -1.2))
			})

			It("should have value for 3-yr", func() {
				Expect(perf.ExcessKurtosis(756)).Should(BeNumerically("~", -1.2))
			})

			It("should have value for 5-yr", func() {
				Expect(perf.ExcessKurtosis(1260)).Should(BeNumerically("~", -1.2))
			})
		})

		Context("with simulated performance data that has draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.ExcessKurtosis(9)).Should(BeNumerically("~", -0.72976815))
			})
		})

	})

	Describe("when calculating information ratio", func() {
		Context("with simulated performance data having a constant increase", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2018, 12, 3, 16, 0, 0, 0, tz),
						Value:          10_000.00,
						BenchmarkValue: 10_000.00,
					},
					{
						Time:           time.Date(2018, 12, 4, 16, 0, 0, 0, tz),
						Value:          9_995.36929844871,
						BenchmarkValue: 9_921.3630406291,
					},
					{
						Time:           time.Date(2018, 12, 05, 16, 0, 0, 0, tz),
						Value:          9_930.53947673072,
						BenchmarkValue: 9_750.98296199214,
					},
					{
						Time:           time.Date(2018, 12, 06, 16, 0, 0, 0, tz),
						Value:          9_818.24496411206,
						BenchmarkValue: 9_724.77064220184,
					},
					{
						Time:           time.Date(2018, 12, 07, 16, 0, 0, 0, tz),
						Value:          9_848.34452419541,
						BenchmarkValue: 9_777.19528178244,
					},
					{
						Time:           time.Date(2018, 12, 10, 16, 0, 0, 0, tz),
						Value:          9_738.36536235239,
						BenchmarkValue: 9_947.5753604194,
					},
					{
						Time:           time.Date(2018, 12, 11, 16, 0, 0, 0, tz),
						Value:          9_792.77610557999,
						BenchmarkValue: 9_868.9384010485,
					},
					{
						Time:           time.Date(2018, 12, 12, 16, 0, 0, 0, tz),
						Value:          9_945.5892567724,
						BenchmarkValue: 9_711.66448230669,
					},
					{
						Time:           time.Date(2018, 12, 13, 16, 0, 0, 0, tz),
						Value:          9_993.05394767307,
						BenchmarkValue: 9_711.66448230669,
					},
					{
						Time:           time.Date(2018, 12, 14, 16, 0, 0, 0, tz),
						Value:          10_004.6307015513,
						BenchmarkValue: 9_750.98296199214,
					},
					{
						Time:           time.Date(2018, 12, 17, 16, 0, 0, 0, tz),
						Value:          10_042.8339893494,
						BenchmarkValue: 9_777.19528178244,
					},
					{
						Time:           time.Date(2018, 12, 18, 16, 0, 0, 0, tz),
						Value:          10_034.7302616347,
						BenchmarkValue: 9_633.02752293578,
					},
					{
						Time:           time.Date(2018, 12, 19, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_462.64744429882,
					},
					{
						Time:           time.Date(2018, 12, 20, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_528.17824377457,
					},
					{
						Time:           time.Date(2018, 12, 21, 16, 0, 0, 0, tz),
						Value:          9_938.64320444554,
						BenchmarkValue: 9_541.28440366972,
					},
					{
						Time:           time.Date(2018, 12, 24, 16, 0, 0, 0, tz),
						Value:          9_880.75943505448,
						BenchmarkValue: 9_554.39056356487,
					},
					{
						Time:           time.Date(2018, 12, 26, 16, 0, 0, 0, tz),
						Value:          9_910.85899513783,
						BenchmarkValue: 9_515.07208387942,
					},
					{
						Time:           time.Date(2018, 12, 27, 16, 0, 0, 0, tz),
						Value:          9_952.5353090994,
						BenchmarkValue: 9_541.28440366972,
					},
					{
						Time:           time.Date(2018, 12, 28, 16, 0, 0, 0, tz),
						Value:          10_006.946052327,
						BenchmarkValue: 9_685.45216251638,
					},
					{
						Time:           time.Date(2018, 12, 31, 16, 0, 0, 0, tz),
						Value:          10_070.9662957698,
						BenchmarkValue: 9_659.23984272608,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.InformationRatio(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.InformationRatio(2531))).Should(BeTrue())
			})

			It("should have value", func() {
				Expect(perf.InformationRatio(19)).Should(BeNumerically("~", 2.52819494))
			})
		})
	})

	Describe("when calculating keller ratio", func() {
		Context("with simulated performance data with portfolioValue = benchmarkValue", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 10.0
					benchmarkValue += 10.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.KellerRatio(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.KellerRatio(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have value for 1-yr", func() {
				Expect(perf.KellerRatio(252, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0771349862))
			})

			It("should have value for 3-yr", func() {
				Expect(perf.KellerRatio(756, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0840169073))
			})

			It("should have value for 5-yr", func() {
				Expect(perf.KellerRatio(1260, portfolio.STRATEGY)).Should(BeNumerically("~", 0.092710428))
			})
		})

		Context("with simulated performance data that has draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.KellerRatio(9, portfolio.STRATEGY)).Should(BeNumerically("~", 0.1092621))
			})
		})

	})

	Describe("when calculating kratio", func() {
		Context("with simulated performance data with portfolioValue = benchmarkValue", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
					})

					value += 10.0
					benchmarkValue += 10.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.KRatio(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.KRatio(2531))).Should(BeTrue())
			})

			It("should have value for 1-yr", func() {
				Expect(perf.KRatio(252)).Should(BeNumerically("~", 0.0137464349))
			})

			It("should have value for 3-yr", func() {
				Expect(perf.KRatio(756)).Should(BeNumerically("~", 0.0079365079))
			})

			It("should have value for 5-yr", func() {
				Expect(perf.KRatio(1260)).Should(BeNumerically("~", 0.00614759))
			})
		})

		Context("with simulated performance data that has draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.KRatio(9)).Should(BeNumerically("~", 0.0727393))
			})
		})

	})

	Describe("when calculating max draw-down", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should be nil for period of 0", func() {
				Expect(perf.MaxDrawDown(0, portfolio.STRATEGY)).To(BeNil())
			})

			It("should not be nil for period greater than # of measurements", func() {
				Expect(perf.MaxDrawDown(11, portfolio.STRATEGY)).ToNot(BeNil())
			})

			It("should not be nil for period matching # of measurements", func() {
				Expect(perf.MaxDrawDown(10, portfolio.STRATEGY)).ToNot(BeNil())
			})

			It("should have drawdown[0] with start date of 2/1/2010", func() {
				dd := perf.MaxDrawDown(9, portfolio.STRATEGY)
				Expect(dd.Begin).To(Equal(time.Date(2010, 2, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[0] with end date of 4/1/2010", func() {
				dd := perf.MaxDrawDown(9, portfolio.STRATEGY)
				Expect(dd.End).To(Equal(time.Date(2010, 3, 1, 16, 0, 0, 0, tz)))
			})

			It("should have drawdown[0] with LossPercent of -.08", func() {
				dd := perf.MaxDrawDown(9, portfolio.STRATEGY)
				Expect(dd.LossPercent).Should(BeNumerically("~", -0.08909091))
			})

			It("should have drawdown[0] with Recovery time of 4/1/2010", func() {
				dd := perf.MaxDrawDown(9, portfolio.STRATEGY)
				Expect(dd.Recovery).To(Equal(time.Date(2010, 4, 1, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("when calculating money-weighted rate of return", func() {
		Context("with 10 years of constant return", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					})

					value += 4.0
					benchmarkValue += 3.0
					date = date.AddDate(0, 0, 1)
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.MWRR(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.MWRR(2521, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have MWRR for 1-day", func() {
				Expect(perf.MWRR(1, portfolio.STRATEGY)).Should(BeNumerically("~", 0.00019928))
			})

			It("should have MWRR for 5-day", func() {
				Expect(perf.MWRR(5, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0009972))
			})

			It("should have MWRR for the full period", func() {
				Expect(perf.MWRR(2519, portfolio.STRATEGY)).Should(BeNumerically("~", 0.1063351))
			})
		})
		Context("with 3 years of simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2011, 1, 1, 16, 0, 0, 0, tz),
						Value:          11_020.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2012, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 11_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2012, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2013, 1, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
				}
			})

			It("should have MWRR", func() {
				Expect(perf.MWRR(4, portfolio.STRATEGY)).Should(BeNumerically("~", 0.0635))
			})
		})
		Context("with 4 months of simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_020.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 11_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
				}
			})

			It("should have MWRR", func() {
				Expect(perf.MWRR(4, portfolio.STRATEGY)).Should(BeNumerically("~", 0.2039559))
			})
		})
		Context("with 3 years of simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2011, 1, 1, 16, 0, 0, 0, tz),
						Value:          11_020.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2012, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 11_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2012, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
					{
						Time:           time.Date(2013, 1, 1, 16, 0, 0, 0, tz),
						Value:          8_005.0,
						TotalDeposited: 10_010.0,
						TotalWithdrawn: 1_000.0,
					},
				}
			})

			It("should have negative MWRR", func() {
				Expect(perf.MWRR(4, portfolio.STRATEGY)).Should(BeNumerically("~", -.0354))
			})
		})
	})

	Describe("when calculating year-to-date performance", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				year := time.Now().Year()
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(year-1, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year-1, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_250.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year-1, 6, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year-1, 12, 31, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year, 1, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year, 3, 1, 16, 0, 0, 0, tz),
						Value:          11_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year, 6, 1, 16, 0, 0, 0, tz),
						Value:          12_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year, 9, 1, 16, 0, 0, 0, tz),
						Value:          12_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(year, 12, 1, 16, 0, 0, 0, tz),
						Value:          13_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have an MWRR YTD", func() {
				Expect(perf.MWRRYtd(portfolio.STRATEGY)).To(BeNumerically("~", 0.20930233))
			})

			It("should have an TWRR YTD", func() {
				Expect(perf.TWRRYtd(portfolio.STRATEGY)).To(BeNumerically("~", 0.20930233))
			})
		})
	})

	Describe("when calculating net profit", func() {
		Context("with simulated performance data with constant increase", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				deposited := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				yearDays := 0
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						TotalDeposited: deposited,
					})

					value += 10.0
					benchmarkValue += 5.0
					yearDays++
					if yearDays == 252 {
						perf.Measurements[len(perf.Measurements)-1].Time = time.Date(date.Year(), 12, 31, 16, 0, 0, 0, tz)
						date = time.Date(date.Year()+1, 1, 1, 16, 0, 0, 0, tz)
						yearDays = 0
					} else {
						date = date.AddDate(0, 0, 1)
					}
				}
			})

			It("should have value for net profit", func() {
				Expect(perf.NetProfit()).Should(BeNumerically("~", 25_190.0))
			})

			It("should have value for net profit percent", func() {
				Expect(perf.NetProfitPercent()).Should(BeNumerically("~", 2.519))
			})
		})

		Context("with simulated performance data that has draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 2, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 3, 1, 16, 0, 0, 0, tz),
						Value:          10_020.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 4, 1, 16, 0, 0, 0, tz),
						Value:          11_000.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 5, 1, 16, 0, 0, 0, tz),
						Value:          11_005.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 6, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 7, 1, 16, 0, 0, 0, tz),
						Value:          10_500.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 8, 1, 16, 0, 0, 0, tz),
						Value:          10_750.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 9, 1, 16, 0, 0, 0, tz),
						Value:          10_490.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
					{
						Time:           time.Date(2010, 10, 1, 16, 0, 0, 0, tz),
						Value:          11_010.0,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.NetProfit()).Should(BeNumerically("~", 1_010))
			})
		})

	})

	Describe("when calculating sharpe ratio", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 1, 1, 16, 0, 0, 0, tz),
						Value:               10_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 2, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 3, 1, 16, 0, 0, 0, tz),
						Value:               10_150.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_150.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 4, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 5, 1, 16, 0, 0, 0, tz),
						Value:               10_275.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_275.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 6, 1, 16, 0, 0, 0, tz),
						Value:               10_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 7, 1, 16, 0, 0, 0, tz),
						Value:               10_300.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_300.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 8, 1, 16, 0, 0, 0, tz),
						Value:               10_700.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_700.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 9, 1, 16, 0, 0, 0, tz),
						Value:               10_750.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_750.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 10, 1, 16, 0, 0, 0, tz),
						Value:               10_800.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_800.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 11, 1, 16, 0, 0, 0, tz),
						Value:               10_900.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_900.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 12, 1, 16, 0, 0, 0, tz),
						Value:               10_950.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_950.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 1, 1, 16, 0, 0, 0, tz),
						Value:               11_000.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_000.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 2, 1, 16, 0, 0, 0, tz),
						Value:               11_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 3, 1, 16, 0, 0, 0, tz),
						Value:               11_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 4, 1, 16, 0, 0, 0, tz),
						Value:               11_500.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_500.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 5, 1, 16, 0, 0, 0, tz),
						Value:               11_550.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_550.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 6, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 7, 1, 16, 0, 0, 0, tz),
						Value:               11_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 8, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.SharpeRatio(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.SharpeRatio(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have an annualized value", func() {
				Expect(perf.SharpeRatio(19, portfolio.STRATEGY)).Should(BeNumerically("~", 1.934508623))
			})
		})
	})

	Describe("when calculating skew", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 1, 1, 16, 0, 0, 0, tz),
						Value:               10_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 2, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 3, 1, 16, 0, 0, 0, tz),
						Value:               10_150.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_150.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 4, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 5, 1, 16, 0, 0, 0, tz),
						Value:               10_275.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_275.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 6, 1, 16, 0, 0, 0, tz),
						Value:               10_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 7, 1, 16, 0, 0, 0, tz),
						Value:               10_300.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_300.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 8, 1, 16, 0, 0, 0, tz),
						Value:               10_700.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_700.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 9, 1, 16, 0, 0, 0, tz),
						Value:               10_750.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_750.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 10, 1, 16, 0, 0, 0, tz),
						Value:               10_800.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_800.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 11, 1, 16, 0, 0, 0, tz),
						Value:               10_900.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_900.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 12, 1, 16, 0, 0, 0, tz),
						Value:               10_950.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_950.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 1, 1, 16, 0, 0, 0, tz),
						Value:               11_000.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_000.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 2, 1, 16, 0, 0, 0, tz),
						Value:               11_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 3, 1, 16, 0, 0, 0, tz),
						Value:               11_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 4, 1, 16, 0, 0, 0, tz),
						Value:               11_500.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_500.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 5, 1, 16, 0, 0, 0, tz),
						Value:               11_550.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_550.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 6, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 7, 1, 16, 0, 0, 0, tz),
						Value:               11_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 8, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.Skew(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.Skew(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have an annualized value", func() {
				Expect(perf.Skew(19, portfolio.STRATEGY)).Should(BeNumerically("~", 0.86379712))
			})
		})
	})

	Describe("when calculating sortino ratio", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 1, 1, 16, 0, 0, 0, tz),
						Value:               10_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 2, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 3, 1, 16, 0, 0, 0, tz),
						Value:               10_150.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_150.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 4, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 5, 1, 16, 0, 0, 0, tz),
						Value:               10_275.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_275.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 6, 1, 16, 0, 0, 0, tz),
						Value:               10_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 7, 1, 16, 0, 0, 0, tz),
						Value:               10_300.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_300.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 8, 1, 16, 0, 0, 0, tz),
						Value:               10_700.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_700.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 9, 1, 16, 0, 0, 0, tz),
						Value:               10_750.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_750.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 10, 1, 16, 0, 0, 0, tz),
						Value:               10_800.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_800.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 11, 1, 16, 0, 0, 0, tz),
						Value:               10_900.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_900.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 12, 1, 16, 0, 0, 0, tz),
						Value:               10_950.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_950.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 1, 1, 16, 0, 0, 0, tz),
						Value:               11_000.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_000.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 2, 1, 16, 0, 0, 0, tz),
						Value:               11_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 3, 1, 16, 0, 0, 0, tz),
						Value:               11_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 4, 1, 16, 0, 0, 0, tz),
						Value:               11_500.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_500.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 5, 1, 16, 0, 0, 0, tz),
						Value:               11_550.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_550.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 6, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 7, 1, 16, 0, 0, 0, tz),
						Value:               11_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 8, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.SortinoRatio(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.SortinoRatio(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have an annualized value", func() {
				Expect(perf.SortinoRatio(19, portfolio.STRATEGY)).Should(BeNumerically("~", 5.66738124))
			})
		})
	})

	Describe("when calculating standard deviation", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 1, 1, 16, 0, 0, 0, tz),
						Value:               10_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 2, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 3, 1, 16, 0, 0, 0, tz),
						Value:               10_150.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_150.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 4, 1, 16, 0, 0, 0, tz),
						Value:               10_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 5, 1, 16, 0, 0, 0, tz),
						Value:               10_275.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_275.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 6, 1, 16, 0, 0, 0, tz),
						Value:               10_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 7, 1, 16, 0, 0, 0, tz),
						Value:               10_300.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_300.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 8, 1, 16, 0, 0, 0, tz),
						Value:               10_700.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_700.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 9, 1, 16, 0, 0, 0, tz),
						Value:               10_750.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_750.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 10, 1, 16, 0, 0, 0, tz),
						Value:               10_800.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_800.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 11, 1, 16, 0, 0, 0, tz),
						Value:               10_900.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_900.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2018, 12, 1, 16, 0, 0, 0, tz),
						Value:               10_950.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 10_950.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 1, 1, 16, 0, 0, 0, tz),
						Value:               11_000.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_000.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 2, 1, 16, 0, 0, 0, tz),
						Value:               11_100.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_100.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 3, 1, 16, 0, 0, 0, tz),
						Value:               11_200.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_200.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 4, 1, 16, 0, 0, 0, tz),
						Value:               11_500.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_500.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 5, 1, 16, 0, 0, 0, tz),
						Value:               11_550.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_550.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 6, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 7, 1, 16, 0, 0, 0, tz),
						Value:               11_400.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_400.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
					{
						Time:                time.Date(2019, 8, 1, 16, 0, 0, 0, tz),
						Value:               11_600.00,
						RiskFreeValue:       10_000.00,
						StrategyGrowthOf10K: 11_600.00,
						RiskFreeGrowthOf10K: 10_000.00,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.StdDev(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.StdDev(2531, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should have an annualized value", func() {
				Expect(perf.StdDev(19, portfolio.STRATEGY)).Should(BeNumerically("~", 0.04117953))
			})
		})
	})

	Describe("when calculating time-weighted rate of return", func() {
		Context("with 10 years of constant return performance", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				date := time.Date(2010, 1, 1, 16, 0, 0, 0, tz)
				value := 10_000.00
				benchmarkValue := 10_000.00
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
				for ii := 0; ii < 2520; ii++ {
					perf.Measurements = append(perf.Measurements, &portfolio.PerformanceMeasurement{
						Time:           date,
						Value:          value,
						BenchmarkValue: benchmarkValue,
						TotalDeposited: 10_000.0,
						TotalWithdrawn: 0.0,
					})

					value += 4.0
					benchmarkValue += 3.0
					date = date.AddDate(0, 0, 1)
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.TWRR(0, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.TWRR(2521, portfolio.STRATEGY))).Should(BeTrue())
			})

			It("should be .01% for 1-day", func() {
				Expect(perf.TWRR(1, portfolio.STRATEGY)).Should(BeNumerically("~", 0.00019928))
			})

			It("should be 0.08% for 5-day", func() {
				Expect(perf.TWRR(5, portfolio.STRATEGY)).Should(BeNumerically("~", 0.00099721))
			})

			It("should be 0.4% for 1-month", func() {
				Expect(perf.TWRR(21, portfolio.STRATEGY)).Should(BeNumerically("~", 0.00420168))
			})

			It("should be 100.76% for the full period", func() {
				Expect(perf.TWRR(2519, portfolio.STRATEGY)).Should(BeNumerically("~", 0.1063351))
			})
		})

		Context("with 10 periods of varied returns and deposits/withdraws through the period", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2010, 1, 1, 16, 0, 0, 0, tz),
						Value:          10_000.00,
						TotalDeposited: 10_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 2, 16, 0, 0, 0, tz),
						Value:          10_200.00,
						TotalDeposited: 10_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 3, 16, 0, 0, 0, tz),
						Value:          9_690.00,
						TotalDeposited: 10_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 4, 16, 0, 0, 0, tz),
						Value:          10_319.85,
						TotalDeposited: 10_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 5, 16, 0, 0, 0, tz),
						Value:          15_577.85,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 6, 16, 0, 0, 0, tz),
						Value:          15_110.51,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 0.00,
					},
					{
						Time:           time.Date(2010, 1, 7, 16, 0, 0, 0, tz),
						Value:          6_808.30,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 8_000.00,
					},
					{
						Time:           time.Date(2010, 1, 8, 16, 0, 0, 0, tz),
						Value:          7_148.72,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 8_000.00,
					},
					{
						Time:           time.Date(2010, 1, 9, 16, 0, 0, 0, tz),
						Value:          7_363.18,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 8_000.00,
					},
					{
						Time:           time.Date(2010, 1, 10, 16, 0, 0, 0, tz),
						Value:          7_657.70,
						TotalDeposited: 15_000.00,
						TotalWithdrawn: 8_000.00,
					},
				}
			})

			It("should have return of ~13%", func() {
				Expect(perf.TWRR(9, portfolio.STRATEGY)).Should(BeNumerically("~", 0.13098, 1e-5))
			})
		})
	})

	Describe("when calculating top-10 draw downs", func() {
		Context("with simulated data that has 12 draw downs", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:  time.Date(2020, 1, 1, 16, 0, 0, 0, tz),
						Value: 10_000,
					},
					{
						Time:  time.Date(2020, 1, 2, 16, 0, 0, 0, tz),
						Value: 10_110,
					}, // Draw Down #1
					{
						Time:  time.Date(2020, 1, 3, 16, 0, 0, 0, tz),
						Value: 10_090,
					},
					{
						Time:  time.Date(2020, 1, 4, 16, 0, 0, 0, tz),
						Value: 10_080,
					},
					{
						Time:  time.Date(2020, 1, 5, 16, 0, 0, 0, tz),
						Value: 10_090,
					},
					{
						Time:  time.Date(2020, 1, 6, 16, 0, 0, 0, tz),
						Value: 10_120,
					}, // Draw Down #2
					{
						Time:  time.Date(2020, 1, 7, 16, 0, 0, 0, tz),
						Value: 10_110,
					},
					{
						Time:  time.Date(2020, 1, 8, 16, 0, 0, 0, tz),
						Value: 10_090,
					},
					{
						Time:  time.Date(2020, 1, 9, 16, 0, 0, 0, tz),
						Value: 10_080,
					},
					{
						Time:  time.Date(2020, 1, 10, 16, 0, 0, 0, tz),
						Value: 10_090,
					},
					{
						Time:  time.Date(2020, 1, 11, 16, 0, 0, 0, tz),
						Value: 10_130,
					},
					{
						Time:  time.Date(2020, 1, 12, 16, 0, 0, 0, tz),
						Value: 10_140,
					}, // Draw Down #3
					{
						Time:  time.Date(2020, 1, 13, 16, 0, 0, 0, tz),
						Value: 10_010,
					},
					{
						Time:  time.Date(2020, 1, 14, 16, 0, 0, 0, tz),
						Value: 9_000,
					},
					{
						Time:  time.Date(2020, 1, 15, 16, 0, 0, 0, tz),
						Value: 9_900,
					},
					{
						Time:  time.Date(2020, 1, 16, 16, 0, 0, 0, tz),
						Value: 11_000,
					}, // Draw Down #4
					{
						Time:  time.Date(2020, 1, 17, 16, 0, 0, 0, tz),
						Value: 10_900,
					},
					{
						Time:  time.Date(2020, 1, 18, 16, 0, 0, 0, tz),
						Value: 10_910,
					},
					{
						Time:  time.Date(2020, 1, 19, 16, 0, 0, 0, tz),
						Value: 11_001,
					}, // Draw Down #5
					{
						Time:  time.Date(2020, 1, 20, 16, 0, 0, 0, tz),
						Value: 11_000,
					},
					{
						Time:  time.Date(2020, 1, 21, 16, 0, 0, 0, tz),
						Value: 10_500,
					},
					{
						Time:  time.Date(2020, 1, 22, 16, 0, 0, 0, tz),
						Value: 11_100,
					}, // Draw Down #6
					{
						Time:  time.Date(2020, 1, 23, 16, 0, 0, 0, tz),
						Value: 11_050,
					},
					{
						Time:  time.Date(2020, 1, 24, 16, 0, 0, 0, tz),
						Value: 11_020,
					},
					{
						Time:  time.Date(2020, 1, 25, 16, 0, 0, 0, tz),
						Value: 11_200,
					},
					{
						Time:  time.Date(2020, 1, 26, 16, 0, 0, 0, tz),
						Value: 11_500,
					}, // Draw Down #7
					{
						Time:  time.Date(2020, 1, 27, 16, 0, 0, 0, tz),
						Value: 11_400,
					},
					{
						Time:  time.Date(2020, 1, 28, 16, 0, 0, 0, tz),
						Value: 11_300,
					},
					{
						Time:  time.Date(2020, 1, 29, 16, 0, 0, 0, tz),
						Value: 11_600,
					},
					{
						Time:  time.Date(2020, 1, 30, 16, 0, 0, 0, tz),
						Value: 11_650,
					}, // Draw Down #8
					{
						Time:  time.Date(2020, 2, 1, 16, 0, 0, 0, tz),
						Value: 11_600,
					},
					{
						Time:  time.Date(2020, 2, 2, 16, 0, 0, 0, tz),
						Value: 11_620,
					},
					{
						Time:  time.Date(2020, 2, 3, 16, 0, 0, 0, tz),
						Value: 11_800,
					}, // Draw Down #9
					{
						Time:  time.Date(2020, 2, 4, 16, 0, 0, 0, tz),
						Value: 11_700,
					},
					{
						Time:  time.Date(2020, 2, 5, 16, 0, 0, 0, tz),
						Value: 11_750,
					},
					{
						Time:  time.Date(2020, 2, 6, 16, 0, 0, 0, tz),
						Value: 11_900,
					},
					{
						Time:  time.Date(2020, 2, 7, 16, 0, 0, 0, tz),
						Value: 12_000,
					}, // Draw Down #10
					{
						Time:  time.Date(2020, 2, 8, 16, 0, 0, 0, tz),
						Value: 11_999,
					},
					{
						Time:  time.Date(2020, 2, 9, 16, 0, 0, 0, tz),
						Value: 11_998,
					},
					{
						Time:  time.Date(2020, 2, 10, 16, 0, 0, 0, tz),
						Value: 12_005,
					},
					{
						Time:  time.Date(2020, 2, 11, 16, 0, 0, 0, tz),
						Value: 13_000,
					}, // Draw Down #11 (max)
					{
						Time:  time.Date(2020, 2, 12, 16, 0, 0, 0, tz),
						Value: 12_500,
					},
					{
						Time:  time.Date(2020, 2, 13, 16, 0, 0, 0, tz),
						Value: 11_000,
					},
					{
						Time:  time.Date(2020, 2, 14, 16, 0, 0, 0, tz),
						Value: 10_000,
					},
					{
						Time:  time.Date(2020, 2, 15, 16, 0, 0, 0, tz),
						Value: 9_000,
					},
					{
						Time:  time.Date(2020, 2, 16, 16, 0, 0, 0, tz),
						Value: 8_000,
					},
					{
						Time:  time.Date(2020, 2, 17, 16, 0, 0, 0, tz),
						Value: 10_000,
					},
					{
						Time:  time.Date(2020, 2, 19, 16, 0, 0, 0, tz),
						Value: 12_000,
					},
					{
						Time:  time.Date(2020, 2, 20, 16, 0, 0, 0, tz),
						Value: 14_000,
					},
					{
						Time:  time.Date(2020, 2, 21, 16, 0, 0, 0, tz),
						Value: 15_000,
					}, // Draw Down #12
					{
						Time:  time.Date(2020, 2, 22, 16, 0, 0, 0, tz),
						Value: 14_900,
					},
					{
						Time:  time.Date(2020, 2, 23, 16, 0, 0, 0, tz),
						Value: 14_950,
					},
					{
						Time:  time.Date(2020, 2, 24, 16, 0, 0, 0, tz),
						Value: 15_100,
					},
				}
			})

			It("should only have 10 draw downs returned", func() {
				Expect(perf.Top10DrawDowns(uint(len(perf.Measurements))-1, portfolio.STRATEGY)).To(HaveLen(10))
			})

			It("should be sorted from max to min", func() {
				ddArr := perf.Top10DrawDowns(uint(len(perf.Measurements))-1, portfolio.STRATEGY)
				dd0 := ddArr[0]
				Expect(dd0.Begin).To(Equal(time.Date(2020, 2, 11, 16, 0, 0, 0, tz)))
				dd1 := ddArr[9]
				Expect(dd1.Begin).To(Equal(time.Date(2020, 1, 6, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("when calculating tracking error", func() {
		Context("with simulated performance data having a constant increase", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2018, 12, 3, 16, 0, 0, 0, tz),
						Value:          10_000.00,
						BenchmarkValue: 10_000.00,
					},
					{
						Time:           time.Date(2018, 12, 4, 16, 0, 0, 0, tz),
						Value:          9_995.36929844871,
						BenchmarkValue: 9_921.3630406291,
					},
					{
						Time:           time.Date(2018, 12, 05, 16, 0, 0, 0, tz),
						Value:          9_930.53947673072,
						BenchmarkValue: 9_750.98296199214,
					},
					{
						Time:           time.Date(2018, 12, 06, 16, 0, 0, 0, tz),
						Value:          9_818.24496411206,
						BenchmarkValue: 9_724.77064220184,
					},
					{
						Time:           time.Date(2018, 12, 07, 16, 0, 0, 0, tz),
						Value:          9_848.34452419541,
						BenchmarkValue: 9_777.19528178244,
					},
					{
						Time:           time.Date(2018, 12, 10, 16, 0, 0, 0, tz),
						Value:          9_738.36536235239,
						BenchmarkValue: 9_947.5753604194,
					},
					{
						Time:           time.Date(2018, 12, 11, 16, 0, 0, 0, tz),
						Value:          9_792.77610557999,
						BenchmarkValue: 9_868.9384010485,
					},
					{
						Time:           time.Date(2018, 12, 12, 16, 0, 0, 0, tz),
						Value:          9_945.5892567724,
						BenchmarkValue: 9_711.66448230669,
					},
					{
						Time:           time.Date(2018, 12, 13, 16, 0, 0, 0, tz),
						Value:          9_993.05394767307,
						BenchmarkValue: 9_711.66448230669,
					},
					{
						Time:           time.Date(2018, 12, 14, 16, 0, 0, 0, tz),
						Value:          10_004.6307015513,
						BenchmarkValue: 9_750.98296199214,
					},
					{
						Time:           time.Date(2018, 12, 17, 16, 0, 0, 0, tz),
						Value:          10_042.8339893494,
						BenchmarkValue: 9_777.19528178244,
					},
					{
						Time:           time.Date(2018, 12, 18, 16, 0, 0, 0, tz),
						Value:          10_034.7302616347,
						BenchmarkValue: 9_633.02752293578,
					},
					{
						Time:           time.Date(2018, 12, 19, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_462.64744429882,
					},
					{
						Time:           time.Date(2018, 12, 20, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_528.17824377457,
					},
					{
						Time:           time.Date(2018, 12, 21, 16, 0, 0, 0, tz),
						Value:          9_938.64320444554,
						BenchmarkValue: 9_541.28440366972,
					},
					{
						Time:           time.Date(2018, 12, 24, 16, 0, 0, 0, tz),
						Value:          9_880.75943505448,
						BenchmarkValue: 9_554.39056356487,
					},
					{
						Time:           time.Date(2018, 12, 26, 16, 0, 0, 0, tz),
						Value:          9_910.85899513783,
						BenchmarkValue: 9_515.07208387942,
					},
					{
						Time:           time.Date(2018, 12, 27, 16, 0, 0, 0, tz),
						Value:          9_952.5353090994,
						BenchmarkValue: 9_541.28440366972,
					},
					{
						Time:           time.Date(2018, 12, 28, 16, 0, 0, 0, tz),
						Value:          10_006.946052327,
						BenchmarkValue: 9_685.45216251638,
					},
					{
						Time:           time.Date(2018, 12, 31, 16, 0, 0, 0, tz),
						Value:          10_070.9662957698,
						BenchmarkValue: 9_659.23984272608,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.TrackingError(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.TrackingError(2531))).Should(BeTrue())
			})

			It("should have value", func() {
				Expect(perf.TrackingError(19)).Should(BeNumerically("~", 0.0136356))
			})
		})
	})

	Describe("when calculating treynor ratio", func() {
		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:           time.Date(2018, 12, 3, 16, 0, 0, 0, tz),
						Value:          10_000.00,
						BenchmarkValue: 10_000.00,
						RiskFreeValue:  10_001,
					},
					{
						Time:           time.Date(2018, 12, 4, 16, 0, 0, 0, tz),
						Value:          9_995.36929844871,
						BenchmarkValue: 9_921.3630406291,
						RiskFreeValue:  10_002,
					},
					{
						Time:           time.Date(2018, 12, 05, 16, 0, 0, 0, tz),
						Value:          9_930.53947673072,
						BenchmarkValue: 9_750.98296199214,
						RiskFreeValue:  10_003,
					},
					{
						Time:           time.Date(2018, 12, 06, 16, 0, 0, 0, tz),
						Value:          9_818.24496411206,
						BenchmarkValue: 9_724.77064220184,
						RiskFreeValue:  10_004,
					},
					{
						Time:           time.Date(2018, 12, 07, 16, 0, 0, 0, tz),
						Value:          9_848.34452419541,
						BenchmarkValue: 9_777.19528178244,
						RiskFreeValue:  10_005,
					},
					{
						Time:           time.Date(2018, 12, 10, 16, 0, 0, 0, tz),
						Value:          9_738.36536235239,
						BenchmarkValue: 9_947.5753604194,
						RiskFreeValue:  10_006,
					},
					{
						Time:           time.Date(2018, 12, 11, 16, 0, 0, 0, tz),
						Value:          9_792.77610557999,
						BenchmarkValue: 9_868.9384010485,
						RiskFreeValue:  10_007,
					},
					{
						Time:           time.Date(2018, 12, 12, 16, 0, 0, 0, tz),
						Value:          9_945.5892567724,
						BenchmarkValue: 9_711.66448230669,
						RiskFreeValue:  10_008,
					},
					{
						Time:           time.Date(2018, 12, 13, 16, 0, 0, 0, tz),
						Value:          9_993.05394767307,
						BenchmarkValue: 9_711.66448230669,
						RiskFreeValue:  10_009,
					},
					{
						Time:           time.Date(2018, 12, 14, 16, 0, 0, 0, tz),
						Value:          10_004.6307015513,
						BenchmarkValue: 9_750.98296199214,
						RiskFreeValue:  10_010,
					},
					{
						Time:           time.Date(2018, 12, 17, 16, 0, 0, 0, tz),
						Value:          10_042.8339893494,
						BenchmarkValue: 9_777.19528178244,
						RiskFreeValue:  10_011,
					},
					{
						Time:           time.Date(2018, 12, 18, 16, 0, 0, 0, tz),
						Value:          10_034.7302616347,
						BenchmarkValue: 9_633.02752293578,
						RiskFreeValue:  10_012,
					},
					{
						Time:           time.Date(2018, 12, 19, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_462.64744429882,
						RiskFreeValue:  10_013,
					},
					{
						Time:           time.Date(2018, 12, 20, 16, 0, 0, 0, tz),
						Value:          10_067.1451724937,
						BenchmarkValue: 9_528.17824377457,
						RiskFreeValue:  10_014,
					},
					{
						Time:           time.Date(2018, 12, 21, 16, 0, 0, 0, tz),
						Value:          9_938.64320444554,
						BenchmarkValue: 9_541.28440366972,
						RiskFreeValue:  10_015,
					},
					{
						Time:           time.Date(2018, 12, 24, 16, 0, 0, 0, tz),
						Value:          9_880.75943505448,
						BenchmarkValue: 9_554.39056356487,
						RiskFreeValue:  10_016,
					},
					{
						Time:           time.Date(2018, 12, 26, 16, 0, 0, 0, tz),
						Value:          9_910.85899513783,
						BenchmarkValue: 9_515.07208387942,
						RiskFreeValue:  10_017,
					},
					{
						Time:           time.Date(2018, 12, 27, 16, 0, 0, 0, tz),
						Value:          9_952.5353090994,
						BenchmarkValue: 9_541.28440366972,
						RiskFreeValue:  10_018,
					},
					{
						Time:           time.Date(2018, 12, 28, 16, 0, 0, 0, tz),
						Value:          10_006.946052327,
						BenchmarkValue: 9_685.45216251638,
						RiskFreeValue:  10_019,
					},
					{
						Time:           time.Date(2018, 12, 31, 16, 0, 0, 0, tz),
						Value:          10_070.9662957698,
						BenchmarkValue: 9_659.23984272608,
						RiskFreeValue:  10_020,
					},
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.TreynorRatio(0))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.TreynorRatio(2531))).Should(BeTrue())
			})

			It("should have an annualized value", func() {
				Expect(perf.TreynorRatio(19)).Should(BeNumerically("~", -0.00177697))
			})
		})
	})

	Describe("when calculating ulcer index", func() {
		Context("with no measurements", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{}
			})

			It("should be NaN", func() {
				Expect(math.IsNaN(perf.UlcerIndex())).Should(BeTrue())
			})
		})

		Context("with simulated performance data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = []*portfolio.PerformanceMeasurement{
					{
						Time:                time.Date(2018, 12, 3, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_000.00,
						BenchmarkValue:      10_000.00,
						RiskFreeValue:       10_001,
					},
					{
						Time:                time.Date(2018, 12, 4, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_995.36929844871,
						BenchmarkValue:      9_921.3630406291,
						RiskFreeValue:       10_002,
					},
					{
						Time:                time.Date(2018, 12, 05, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_930.53947673072,
						BenchmarkValue:      9_750.98296199214,
						RiskFreeValue:       10_003,
					},
					{
						Time:                time.Date(2018, 12, 06, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_818.24496411206,
						BenchmarkValue:      9_724.77064220184,
						RiskFreeValue:       10_004,
					},
					{
						Time:                time.Date(2018, 12, 07, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_848.34452419541,
						BenchmarkValue:      9_777.19528178244,
						RiskFreeValue:       10_005,
					},
					{
						Time:                time.Date(2018, 12, 10, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_738.36536235239,
						BenchmarkValue:      9_947.5753604194,
						RiskFreeValue:       10_006,
					},
					{
						Time:                time.Date(2018, 12, 11, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_792.77610557999,
						BenchmarkValue:      9_868.9384010485,
						RiskFreeValue:       10_007,
					},
					{
						Time:                time.Date(2018, 12, 12, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_945.5892567724,
						BenchmarkValue:      9_711.66448230669,
						RiskFreeValue:       10_008,
					},
					{
						Time:                time.Date(2018, 12, 13, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_993.05394767307,
						BenchmarkValue:      9_711.66448230669,
						RiskFreeValue:       10_009,
					},
					{
						Time:                time.Date(2018, 12, 14, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_004.6307015513,
						BenchmarkValue:      9_750.98296199214,
						RiskFreeValue:       10_010,
					},
					{
						Time:                time.Date(2018, 12, 17, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_042.8339893494,
						BenchmarkValue:      9_777.19528178244,
						RiskFreeValue:       10_011,
					},
					{
						Time:                time.Date(2018, 12, 18, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_034.7302616347,
						BenchmarkValue:      9_633.02752293578,
						RiskFreeValue:       10_012,
					},
					{
						Time:                time.Date(2018, 12, 19, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_067.1451724937,
						BenchmarkValue:      9_462.64744429882,
						RiskFreeValue:       10_013,
					},
					{
						Time:                time.Date(2018, 12, 20, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_067.1451724937,
						BenchmarkValue:      9_528.17824377457,
						RiskFreeValue:       10_014,
					},
					{
						Time:                time.Date(2018, 12, 21, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_938.64320444554,
						BenchmarkValue:      9_541.28440366972,
						RiskFreeValue:       10_015,
					},
					{
						Time:                time.Date(2018, 12, 24, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_880.75943505448,
						BenchmarkValue:      9_554.39056356487,
						RiskFreeValue:       10_016,
					},
					{
						Time:                time.Date(2018, 12, 26, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_910.85899513783,
						BenchmarkValue:      9_515.07208387942,
						RiskFreeValue:       10_017,
					},
					{
						Time:                time.Date(2018, 12, 27, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 9_952.5353090994,
						BenchmarkValue:      9_541.28440366972,
						RiskFreeValue:       10_018,
					},
					{
						Time:                time.Date(2018, 12, 28, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_006.946052327,
						BenchmarkValue:      9_685.45216251638,
						RiskFreeValue:       10_019,
					},
					{
						Time:                time.Date(2018, 12, 31, 16, 0, 0, 0, tz),
						StrategyGrowthOf10K: 10_070.9662957698,
						BenchmarkValue:      9_659.23984272608,
						RiskFreeValue:       10_020,
					},
				}
			})

			It("should have a value", func() {
				Expect(perf.UlcerIndex()).Should(BeNumerically("~", 0.80743547))
			})
		})
	})

	Describe("when calculating ulcer index by percentile", func() {
		Context("with simulated data", func() {
			BeforeEach(func() {
				perf = &portfolio.Performance{}
				perf.Measurements = make([]*portfolio.PerformanceMeasurement, 100)
				for ii := 0; ii < 100; ii++ {
					perf.Measurements[ii] = &portfolio.PerformanceMeasurement{
						UlcerIndex: float32(ii + 1.0),
					}
				}
			})

			It("should be NaN for period of 0", func() {
				Expect(math.IsNaN(perf.UlcerIndexPercentile(0, .5))).Should(BeTrue())
			})

			It("should be NaN for period greater than # of measurements", func() {
				Expect(math.IsNaN(perf.UlcerIndexPercentile(2531, .5))).Should(BeTrue())
			})

			It("should be NaN for out-of lower percentile", func() {
				Expect(math.IsNaN(perf.UlcerIndexPercentile(100, -0.1))).Should(BeTrue())
			})

			It("should be NaN for out-of upper percentile", func() {
				Expect(math.IsNaN(perf.UlcerIndexPercentile(100, 1.5))).Should(BeTrue())
			})

			It("should have median Ulcer index of 50", func() {
				Expect(perf.UlcerIndexPercentile(99, .50)).Should(BeNumerically("~", 50.0))
			})

			It("should have q1 Ulcer index of 25", func() {
				Expect(perf.UlcerIndexPercentile(99, .25)).Should(BeNumerically("~", 25.0))
			})

			It("should have q3 Ulcer index of 75", func() {
				Expect(perf.UlcerIndexPercentile(99, .75)).Should(BeNumerically("~", 75.0))
			})

			It("should have 90% Ulcer index of 90", func() {
				Expect(perf.UlcerIndexPercentile(99, .9)).Should(BeNumerically("~", 90.0))
			})

			It("should have not overrun for 0%", func() {
				Expect(perf.UlcerIndexPercentile(99, 0.0)).Should(BeNumerically("~", 1.0))
			})

			It("should have not overrun for 100%", func() {
				Expect(perf.UlcerIndexPercentile(99, 1.0)).Should(BeNumerically("~", 100.0))
			})
		})
	})
})
