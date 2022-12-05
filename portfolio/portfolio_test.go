// Copyright 2021-2022
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
	"context"
	"time"

	"github.com/jackc/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/rs/zerolog/log"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/penny-vault/pv-api/portfolio"
)

var _ = Describe("Portfolio", Ordered, func() {
	var (
		dbPool  pgxmock.PgxConnIface
		manager *data.Manager
		plan    data.PortfolioPlan

		err  error
		p    *portfolio.Portfolio
		perf *portfolio.Performance
		pm   *portfolio.Model
		tz   *time.Location

		vustx *data.Security
		vfinx *data.Security
		pridx *data.Security
	)

	Describe("with a single holding at a time", func() {
		Context("is successfully invested in target portfolio", func() {
			BeforeEach(func() {
				manager = data.GetManagerInstance()
				manager.Reset()
				dbPool, err = pgxmock.NewConn()
				Expect(err).To(BeNil())
				database.SetPool(dbPool)

				vustx, err = data.SecurityFromTicker("VUSTX")
				Expect(err).To(BeNil())
				vfinx, err = data.SecurityFromTicker("VFINX")
				Expect(err).To(BeNil())
				pridx, err = data.SecurityFromTicker("PRIDX")
				Expect(err).To(BeNil())

				tz = common.GetTimezone()

				plan = data.PortfolioPlan{
					{
						Date: time.Date(2019, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 1.0,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2020, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*pridx: 1.0,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2021, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 1.0,
						},
						Justifications: map[string]float64{},
					},
				}

				pm = portfolio.NewPortfolio("Test", time.Date(2019, time.January, 31, 0, 0, 0, 0, tz), 10000)
				p = pm.Portfolio

				// Expect dataframe transaction and query for VFINX
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2019, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 2, 4, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
					time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 2, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2021, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2022, 2, 2, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				err = pm.TargetPortfolio(context.Background(), plan)
			})

			It("should not error", func() {
				if err != nil {
					log.Error().Err(err).Msg("target portfolio errored")
				}
				Expect(err).NotTo(HaveOccurred())
			})

			It("should error if trying to rebalance on a date when transactions have already occurred", func() {
				target := make(map[data.Security]float64)
				vfinx := data.Security{
					CompositeFigi: "BBG000BHTMY2",
					Ticker:        "VFINX",
				}
				target[vfinx] = 1.0
				justification := make([]*portfolio.Justification, 0)

				allocation := &data.SecurityAllocation{
					Date:    time.Date(2019, 5, 1, 0, 0, 0, 0, tz),
					Members: target,
				}

				err = pm.RebalanceTo(context.Background(), allocation, justification)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("start date occurs after through date"))
			})

			It("should have transactions", func() {
				// 1 DEPOSIT    (2019-01-31)
				// 2 BUY VFINX  (2019-01-31)
				// 3 DIVIDEND   (2019-03-20)
				// 4 DIVIDEND   (2019-06-26)
				// 5 DIVIDEND   (2019-09-25)
				// 6 DIVIDEND   (2019-12-20)
				// 7 SELL VFINX (2020-01-31)
				// 8 BUY PRIDX  (2020-01-31)
				// 9 LTC        (2020-12-17)
				// 10 SELL PRIDX (2021-01-31)
				// 11 BUY VFINX  (2021-01-31)
				Expect(p.Transactions).To(HaveLen(11))
			})

			It("first transaction should be a deposit", func() {
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.DepositTransaction))
				Expect(p.Transactions[0].Date).To(Equal(time.Date(2018, 1, 31, 0, 0, 0, 0, tz)))
				Expect(p.Transactions[0].Ticker).To(Equal(data.CashAsset))
				Expect(p.Transactions[0].Shares).To(Equal(10_000.0))
				Expect(p.Transactions[0].TotalValue).Should(BeNumerically("~", 10_000.00, 1e-2))
			})

			It("second transaction should be a buy of VFINX", func() {
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[1].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[1].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[1].Date).To(Equal(time.Date(2018, 1, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[1].Shares).Should(BeNumerically("~", 38.32592, 1e-5))
				Expect(p.Transactions[1].TotalValue).Should(BeNumerically("~", 10000.00, 1e-2))
			})

			It("should have a transaction on 2018-03-23 for the VFINX dividend", func() {
				Expect(p.Transactions[2].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[2].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[2].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[2].Date).To(Equal(time.Date(2018, 3, 23, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[2].Shares).To(Equal(0.0))
				Expect(p.Transactions[2].TotalValue).Should(BeNumerically("~", 39.38755, 1e-5))
			})

			It("should have a transaction on 2018-06-27 for the VFINX dividend", func() {
				Expect(p.Transactions[3].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[3].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[3].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[3].Date).To(Equal(time.Date(2018, 6, 27, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[3].Shares).To(Equal(0.0))
				Expect(p.Transactions[3].TotalValue).Should(BeNumerically("~", 42.09336, 1e-5))
			})

			It("should have a transaction on 2018-09-25 for the VFINX dividend", func() {
				Expect(p.Transactions[4].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[4].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[4].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[4].Date).To(Equal(time.Date(2018, 9, 25, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[4].Shares).To(Equal(0.0))
				Expect(p.Transactions[4].TotalValue).Should(BeNumerically("~", 44.08248, 1e-5))
			})

			It("should have a transaction on 2018-12-14 for the VFINX dividend", func() {
				Expect(p.Transactions[5].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[5].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[5].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[5].Date).To(Equal(time.Date(2018, 12, 14, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[5].Shares).To(Equal(0.0))
				Expect(p.Transactions[5].TotalValue).Should(BeNumerically("~", 46.90327, 1e-5))
			})

			It("should have a transaction on 2019-01-31 SELL of VFINX", func() {
				Expect(p.Transactions[6].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[6].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[6].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[6].Date).To(Equal(time.Date(2019, 1, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[6].Shares).Should(BeNumerically("~", 38.32592, 1e-5))
				Expect(p.Transactions[6].PricePerShare).Should(BeNumerically("~", 249.96, 1e-5))
				Expect(p.Transactions[6].TotalValue).Should(BeNumerically("~", 9579.94788, 1e-5))
			})

			It("should have a transaction on 2019-01-31 BUY of PRIDX", func() {
				Expect(p.Transactions[7].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[7].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[7].CompositeFIGI).To(Equal("BBG000BBVR08"))
				Expect(p.Transactions[7].Date).To(Equal(time.Date(2019, 1, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[7].Shares).Should(BeNumerically("~", 163.19301, 1e-5))
				Expect(p.Transactions[7].PricePerShare).Should(BeNumerically("~", 59.76, 1e-5))
				Expect(p.Transactions[7].TotalValue).Should(BeNumerically("~", 9752.41453, 1e-5))
			})

			It("should have a transaction on 2019-12-17 for the PRIDX dividend", func() {
				Expect(p.Transactions[8].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[8].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[8].CompositeFIGI).To(Equal("BBG000BBVR08"))
				Expect(p.Transactions[8].Date).To(Equal(time.Date(2019, 12, 17, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[8].Shares).To(Equal(0.0))
				Expect(p.Transactions[8].TotalValue).Should(BeNumerically("~", 164.82494, 1e-5))
			})

			It("should have a transaction on 2020-01-31 SELL of PRIDX", func() {
				Expect(p.Transactions[9].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[9].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[9].CompositeFIGI).To(Equal("BBG000BBVR08"))
				Expect(p.Transactions[9].Date).To(Equal(time.Date(2020, 1, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[9].Shares).Should(BeNumerically("~", 163.19301, 1e-5))
				Expect(p.Transactions[9].PricePerShare).Should(BeNumerically("~", 67.16, 1e-5))
				Expect(p.Transactions[9].TotalValue).Should(BeNumerically("~", 10960.04284, 1e-5))
			})

			It("should have a transaction on 2020-01-31 BUY of VFINX", func() {
				Expect(p.Transactions[10].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[10].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[10].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[10].Date).To(Equal(time.Date(2020, 1, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[10].Shares).Should(BeNumerically("~", 37.33052, 1e-5))
				Expect(p.Transactions[10].PricePerShare).Should(BeNumerically("~", 298.01, 1e-5))
				Expect(p.Transactions[10].TotalValue).Should(BeNumerically("~", 11124.86778, 1e-5))
			})
		})

		Context("has stocks with splits", func() {
			BeforeEach(func() {
				ctx := context.Background()
				tsla, err := data.SecurityFromFigi("BBG000N9MNX3")
				Expect(err).To(BeNil())

				plan = data.PortfolioPlan{
					{
						Date: time.Date(2020, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*tsla: 1.0,
						},
						Justifications: map[string]float64{},
					},
				}

				pm = portfolio.NewPortfolio("Test", time.Date(2020, time.January, 31, 0, 0, 0, 0, tz), 10000)
				p = pm.Portfolio

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"},
					time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				err = pm.TargetPortfolio(ctx, plan)
				Expect(err).To(BeNil())
				err = pm.FillCorporateActions(ctx, time.Date(2021, time.January, 1, 0, 0, 0, 0, tz))
				Expect(err).To(BeNil())
				perf = portfolio.NewPerformance(pm.Portfolio)
			})

			It("should have transactions", func() {
				Expect(p.Transactions).To(HaveLen(3))
			})

			It("third transaction should be a SPLIT on 2020-08-31", func() {
				Expect(p.Transactions[2].Kind).To(Equal(portfolio.SplitTransaction))
				Expect(p.Transactions[2].Ticker).To(Equal("TSLA"))
				Expect(p.Transactions[2].CompositeFIGI).To(Equal("BBG000N9MNX3"))
				Expect(p.Transactions[2].Date).To(Equal(time.Date(2020, 8, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[2].Shares).Should(BeNumerically("~", 76.85568, 1e-5))
			})

			It("shouldn't change value after SPLIT on 2020-08-31", func() {
				dbPool.ExpectBegin()
				dbPool.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2020, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 3, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"},
					time.Date(2020, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 3, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				err = perf.CalculateThrough(context.Background(), pm, time.Date(2020, time.November, 30, 0, 0, 0, 0, tz))
				Expect(err).NotTo(HaveOccurred())

				// Friday, August 28, 2020
				Expect(perf.Measurements[146].Time).To(Equal(time.Date(2020, time.August, 28, 23, 59, 59, 999999999, tz)))
				Expect(perf.Measurements[146].Value).Should(BeNumerically("~", 34022.4726, 1e-5))
				Expect(perf.Measurements[146].Holdings[0].Shares).Should(BeNumerically("~", 15.37114, 1e-5))

				// Monday, August 31, 2020
				Expect(perf.Measurements[147].Time).To(Equal(time.Date(2020, time.August, 31, 23, 59, 59, 999999999, tz)))
				Expect(perf.Measurements[147].Value).Should(BeNumerically("~", 38298.72266, 1e-5))

				// Tuesday, September 1, 2020 (NOTE: Holdings lag 1 in measurements)
				Expect(perf.Measurements[148].Holdings[0].Shares).Should(BeNumerically("~", 76.85568, 1e-5))
			})
		})

		Context("calculates performance through 2020-11-30", func() {
			BeforeEach(func() {
				vfinx, err := data.SecurityFromFigi("BBG000BHTMY2")
				Expect(err).To(BeNil(), "vfinx security lookup")
				pridx, err := data.SecurityFromFigi("BBG000BBVR08")
				Expect(err).To(BeNil(), "pridx security lookup")

				plan = data.PortfolioPlan{
					{
						Date: time.Date(2018, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 1.0,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2019, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*pridx: 1.0,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2020, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 1.0,
						},
						Justifications: map[string]float64{},
					},
				}

				pm = portfolio.NewPortfolio("Test", time.Date(2018, time.January, 31, 0, 0, 0, 0, tz), 10000)

				// Expect database transactions
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2018, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2018, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2019, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2019, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
					time.Date(2019, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2019, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
					time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC), time.Date(2020, 7, 31, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				dbPool.ExpectBegin()
				dbPool.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2018, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2019, 2, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2019, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
					time.Date(2019, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2020, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 3, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				err = pm.TargetPortfolio(context.Background(), plan)
				Expect(err).To(BeNil())
				perf = portfolio.NewPerformance(pm.Portfolio)
				err = perf.CalculateThrough(context.Background(), pm, time.Date(2020, time.November, 30, 16, 0, 0, 0, tz))
				Expect(err).To(BeNil())
			})

			It("should have performance measurements", func() {
				Expect(perf.Measurements).To(HaveLen(714))
			})

			It("should have a balance of $10,000 on Jan 31, 2018", func() {
				Expect(perf.Measurements[0].Time).To(Equal(time.Date(2018, 1, 31, 23, 59, 59, 999999999, tz)))
				Expect(perf.Measurements[0].Value).Should(BeNumerically("~", 10_000.0, 1e-5))
				Expect(perf.Measurements[0].BenchmarkValue).Should(BeNumerically("~", 10_000.0, 1e-5))
				Expect(perf.Measurements[0].Holdings[0].Ticker).To(Equal("VFINX"))
				Expect(perf.Measurements[0].Holdings[0].CompositeFIGI).To(Equal("BBG000BHTMY2"))
			})

			It("value should not be calculated on non-trading days", func() {
				Expect(perf.Measurements[3].Time).To(Equal(time.Date(2018, 2, 5, 23, 59, 59, 999999999, tz)))
				Expect(perf.Measurements[3].Value).Should(BeNumerically("~", 9382.18611, 1e-5))
				Expect(perf.Measurements[3].BenchmarkValue).Should(BeNumerically("~", 9382.18, 1e-2))
			})

			It("value should include the dividend released on 2018-03-23", func() {
				Expect(perf.Measurements[36].Time).To(Equal(time.Date(2018, 3, 23, 23, 59, 59, 999999999, tz)))
				Expect(perf.Measurements[36].Value).Should(BeNumerically("~", 9195.83397, 1e-5))
				Expect(perf.Measurements[36].BenchmarkValue).Should(BeNumerically("~", 9195.83, 1e-2))
			})

			It("should have a final measurement on November 30, 2020", func() {
				Expect(perf.Measurements[713].Time).To(Equal(time.Date(2020, 11, 30, 23, 59, 59, 999999999, tz)))
			})
		})
	})

	Describe("with multiple holdings at a time", func() {
		Context("is successfully invested", func() {
			BeforeEach(func() {
				plan = data.PortfolioPlan{
					{
						Date: time.Date(2018, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 1.0,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2019, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*vfinx: 0.25,
							*pridx: 0.5,
							*vustx: 0.25,
						},
						Justifications: map[string]float64{},
					},
					{
						Date: time.Date(2020, time.January, 31, 0, 0, 0, 0, tz),
						Members: map[data.Security]float64{
							*pridx: 1.0,
						},
						Justifications: map[string]float64{},
					},
				}

				pm = portfolio.NewPortfolio("Test", time.Date(2018, time.January, 31, 0, 0, 0, 0, tz), 10000)
				p = pm.Portfolio

				// Setup database
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vustx.csv"},
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), "close", "split_factor", "dividend")

				err = pm.TargetPortfolio(context.Background(), plan)
				Expect(err).To(BeNil())
			})

			It("should have transactions", func() {
				Expect(p.Transactions).To(HaveLen(30))
			})

			It("should have strictly increasing transaction dates", func() {
				last := p.Transactions[0].Date
				for _, trx := range p.Transactions {
					Expect(trx.Date).Should(BeTemporally(">=", last))
					last = trx.Date
				}
			})

			It("first transaction should be a deposit", func() {
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.DepositTransaction))
				Expect(p.Transactions[0].Date).Should(BeTemporally("==", time.Date(2018, 1, 31, 0, 0, 0, 0, tz)))
				Expect(p.Transactions[0].Ticker).To(Equal(data.CashAsset))
				Expect(p.Transactions[0].Shares).To(Equal(10_000.0))
				Expect(p.Transactions[0].TotalValue).Should(BeNumerically("~", 10_000.00, 1e-2))
			})

			It("should buy VFINX on 2018-01-31", func() {
				Expect(p.Transactions[1].Date).To(Equal(time.Date(2018, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[1].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[1].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[1].PricePerShare).Should(BeNumerically("~", 260.92, 1e-2))
				Expect(p.Transactions[1].Shares).Should(BeNumerically("~", 38.32592, 1e-5))
				Expect(p.Transactions[1].TotalValue).Should(BeNumerically("~", 10_000.00, 1e-2))
			})

			It("should sell 75 percent of VFINX on 2019-01-31", func() {
				Expect(p.Transactions[6].Date).To(Equal(time.Date(2019, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[6].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[6].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[6].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[6].PricePerShare).Should(BeNumerically("~", 249.96, 1e-2))
				Expect(p.Transactions[6].Shares).Should(BeNumerically("~", 28.57195, 1e-5))
				Expect(p.Transactions[6].TotalValue).Should(BeNumerically("~", 7141.84424, 1e-5))
			})

			It("should invest 50 percent of the portfolio in PRIDX on 2019-01-31", func() {
				// Buy PRIDX
				// Order of purchases within a given day are not guaranteed
				pridxIdx := 7
				if p.Transactions[pridxIdx].Ticker != "PRIDX" {
					pridxIdx = 8
				}
				Expect(p.Transactions[pridxIdx].Date).To(Equal(time.Date(2019, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[pridxIdx].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[pridxIdx].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[pridxIdx].CompositeFIGI).To(Equal("BBG000BBVR08"))
				Expect(p.Transactions[pridxIdx].Shares).Should(BeNumerically("~", 81.59651, 1e-5))
				Expect(p.Transactions[pridxIdx].TotalValue).Should(BeNumerically("~", 4876.20727, 1e-5))
			})

			It("should invest 25 percent of the portfolio in VUSTX on 2019-01-31", func() {
				// Buy VUSTX
				// Order of purchases within a given day are not guaranteed
				vustxIdx := 8
				if p.Transactions[vustxIdx].Ticker != "VUSTX" {
					vustxIdx = 7
				}

				Expect(p.Transactions[vustxIdx].Date).To(Equal(time.Date(2019, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[vustxIdx].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[vustxIdx].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[vustxIdx].CompositeFIGI).To(Equal("BBG000BCKYB1"))
				Expect(p.Transactions[vustxIdx].Shares).Should(BeNumerically("~", 205.57366, 1e-5))
				Expect(p.Transactions[vustxIdx].TotalValue).Should(BeNumerically("~", 2438.10363, 1e-5))
			})

			It("should have a dividend for VUSTX on 2019-02-28", func() {
				Expect(p.Transactions[9].Date).To(Equal(time.Date(2019, 02, 28, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[9].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[9].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[9].CompositeFIGI).To(Equal("BBG000BCKYB1"))
				Expect(p.Transactions[9].Shares).Should(BeNumerically("~", 0, 1e-5))
				Expect(p.Transactions[9].TotalValue).Should(BeNumerically("~", 5.1804, 1e-2))
			})

			It("should have a dividend for VFINX on 2019-03-20", func() {
				Expect(p.Transactions[10].Date).To(Equal(time.Date(2019, 03, 20, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[10].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[10].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[10].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[10].Shares).Should(BeNumerically("~", 0, 1e-5))
				Expect(p.Transactions[10].TotalValue).Should(BeNumerically("~", 13.55998, 1e-5))
			})

			It("should have a dividend for VUSTX on 2019-03-29", func() {
				Expect(p.Transactions[11].Date).To(Equal(time.Date(2019, 03, 29, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[11].Kind).To(Equal(portfolio.DividendTransaction))
				Expect(p.Transactions[11].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[11].CompositeFIGI).To(Equal("BBG000BCKYB1"))
				Expect(p.Transactions[11].Shares).Should(BeNumerically("~", 0, 1e-5))
				Expect(p.Transactions[11].TotalValue).Should(BeNumerically("~", 5.96163, 1e-5))
			})

			It("should sell VUSTX holdings on 2020-01-31", func() {
				// Sell VUSTX
				// Order of sell transactions on a given day are not ordered -- check the order
				vustxIdx := 27
				if p.Transactions[vustxIdx].Ticker != "VUSTX" {
					vustxIdx = 28
				}

				Expect(p.Transactions[vustxIdx].Date).To(Equal(time.Date(2020, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[vustxIdx].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[vustxIdx].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[vustxIdx].CompositeFIGI).To(Equal("BBG000BCKYB1"))
				Expect(p.Transactions[vustxIdx].Shares).Should(BeNumerically("~", 205.57366, 1e-2))
				Expect(p.Transactions[vustxIdx].TotalValue).Should(BeNumerically("~", 2888.30995, 1e-2))
			})

			It("should sell VFINX holdings on 2020-01-31", func() {
				// Sell VFINX
				// Order of sell transactions on a given day are not ordered -- check the order
				vfinxIdx := 28
				if p.Transactions[vfinxIdx].Ticker != "VFINX" {
					vfinxIdx = 27
				}
				Expect(p.Transactions[vfinxIdx].Date).To(Equal(time.Date(2020, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[vfinxIdx].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[vfinxIdx].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[vfinxIdx].CompositeFIGI).To(Equal("BBG000BHTMY2"))
				Expect(p.Transactions[vfinxIdx].Shares).Should(BeNumerically("~", 9.75398, 1e-2))
				Expect(p.Transactions[vfinxIdx].TotalValue).Should(BeNumerically("~", 2906.78214, 1e-2))
			})

			It("should invest 100 percent of the portfolio in PRIDX on 2020-01-31", func() {
				// Buy PRIDX
				Expect(p.Transactions[29].Date).To(Equal(time.Date(2020, 01, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[29].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[29].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[29].CompositeFIGI).To(Equal("BBG000BBVR08"))
				Expect(p.Transactions[29].Shares).Should(BeNumerically("~", 89.40857, 1e-2))
				Expect(p.Transactions[29].TotalValue).Should(BeNumerically("~", 6004.66897, 1e-5))
			})
		})
	})
})
