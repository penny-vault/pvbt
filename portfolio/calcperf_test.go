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
	"context"
	"time"

	"github.com/jdfergason/dataframe-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/penny-vault/pv-api/portfolio"
)

var _ = Describe("Portfolio Continuous Update", func() {
	var (
		dataProxy data.Manager
		dbPool    pgxmock.PgxConnIface
		df        *dataframe.DataFrame
		err       error
		p         *portfolio.Portfolio
		//perf      *portfolio.Performance
		pm *portfolio.Model
		tz *time.Location
	)

	BeforeEach(func() {
		tz, err = time.LoadLocation("America/New_York") // New York is the reference time
		Expect(err).To(BeNil())

		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// Expect trading days transaction and query
		pgxmockhelper.MockDBEodQuery(dbPool, []string{"riskfree.csv"},
			time.Date(1969, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC),
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC))
		pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"riskfree_corporate.csv"},
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC))
		data.InitializeDataManager()

		dataProxy = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})
	})

	Describe("with multiple holdings at a time", func() {
		Context("is successfully invested", func() {
			BeforeEach(func() {
				/*
					timeSeries := dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Size: 3}, []time.Time{
						time.Date(2018, time.January, 31, 0, 0, 0, 0, tz),
						time.Date(2019, time.January, 31, 0, 0, 0, 0, tz),
						time.Date(2020, time.January, 31, 0, 0, 0, 0, tz),
					})

					tickerSeriesMulti := dataframe.NewSeriesMixed(common.TickerName,
						&dataframe.SeriesInit{Size: 3},
						map[string]float64{
							"VFINX": 1.0,
						},
						map[string]float64{
							"VFINX": 0.25,
							"PRIDX": 0.5,
							"VUSTX": 0.25,
						},
						map[string]float64{
							"PRIDX": 1.0,
						},
					)
				*/
				timeSeries := dataframe.NewSeriesTime(common.DateIdx, &dataframe.SeriesInit{Size: 1}, []time.Time{
					time.Date(2018, time.January, 31, 0, 0, 0, 0, tz),
				})

				tickerSeriesMulti := dataframe.NewSeriesMixed(common.TickerName,
					&dataframe.SeriesInit{Size: 1},
					map[string]float64{
						"VFINX": 1.0,
					},
				)

				df = dataframe.NewDataFrame(timeSeries, tickerSeriesMulti)
				pm = portfolio.NewPortfolio("Test", dataProxy.Begin, 10000, &dataProxy)
				p = pm.Portfolio
				dataProxy.Begin = time.Date(2018, time.January, 1, 0, 0, 0, 0, tz)
				dataProxy.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)
				dataProxy.Frequency = data.FrequencyDaily

				// Setup database
				pgxmockhelper.MockDBEodQuery(dbPool, []string{"vfinx.csv"},
					time.Date(2017, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 8, 0, 0, 0, 0, time.UTC),
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
				pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"vfinx_corporate.csv"},
					time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC))

				_, err := dataProxy.GetDataFrame(context.Background(), data.MetricClose, "VFINX")
				Expect(err).To(BeNil())

				/*
					pgxmockhelper.MockDBEodQuery(dbPool, []string{"pridx.csv"},
						time.Date(2017, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 8, 0, 0, 0, 0, time.UTC),
						time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
					pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"pridx_corporate.csv"},
						time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC))

					_, err = dataProxy.GetDataFrame(context.Background(), data.MetricClose, "PRIDX")
					Expect(err).To(BeNil())

					pgxmockhelper.MockDBEodQuery(dbPool, []string{"vustx.csv"},
						time.Date(2017, 12, 25, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 8, 0, 0, 0, 0, time.UTC),
						time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
					pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"vustx_corporate.csv"},
						time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC))

					_, err = dataProxy.GetDataFrame(context.Background(), data.MetricClose, "VUSTX")
					Expect(err).To(BeNil())
				*/

				err = pm.TargetPortfolio(context.Background(), df)
				Expect(err).To(BeNil())
				//perf = portfolio.NewPerformance(pm.Portfolio)
				//err = perf.CalculateThrough(context.Background(), pm, time.Date(2018, time.January, 31, 18, 0, 0, 0, tz))
				//Expect(err).To(BeNil())
			})

			It("should have exactly 2 transactions", func() {
				Expect(p.Transactions).To(HaveLen(2))
			})

			It("should have a deposit on 1/31/2018", func() {
				Expect(p.Transactions[0].Date).To(Equal(time.Date(2018, time.January, 31, 0, 0, 0, 0, tz)))
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.DepositTransaction))
			})

			It("should have a buy on 1/31/2018", func() {
				Expect(p.Transactions[1].Date).To(Equal(time.Date(2018, time.January, 31, 16, 0, 0, 0, tz)))
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.BuyTransaction))
			})

		})
	})
})
