// Copyright 2021-2023
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

package data_test

import (
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Manager tests", func() {
	var (
		manager *data.Manager
		dbPool  pgxmock.PgxConnIface
	)

	BeforeEach(func() {
		var err error
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		manager = data.GetManagerInstance()
		manager.Reset()
	})

	// TODO: add tests that ensure if data is queried on date A but data has not yet been downloaded the cache properly
	// reflects that the data for today is not yet available.

	Context("when fetching metrics", func() {
		It("it errors for unknown securities", func() {
			securities := []*data.Security{
				{
					Ticker:        "UNKNOWN",
					CompositeFigi: "UNKNOWN",
				},
			}

			metrics := []data.Metric{
				data.MetricClose,
			}
			_, err := manager.GetMetrics(securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).ToNot(BeNil())
			Expect(errors.Is(err, data.ErrSecurityNotFound)).To(BeTrue())
		})

		It("fetches TSLA when only ticker is present", func() {
			securities := []*data.Security{
				{
					Ticker:        "TSLA",
					CompositeFigi: "UNKNOWN",
				},
			}

			metrics := []data.Metric{
				data.MetricClose,
			}

			// 2021-01-04  BBG000N9MNX3    719.4600  744.4899  717.1895  729.7700  729.7700   1.000000      0.0000
			// 2021-01-05  BBG000N9MNX3    723.6600  740.8400  719.2000  735.1100  735.1100   1.000000      0.0000

			pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 0, 0, 0, 0, common.GetTimezone()), "close", "split_factor", "dividend")
			dfMap, err := manager.GetMetrics(securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).To(BeNil(), "error when fetching data")
			df := dfMap.DataFrame()

			Expect(df.Len()).To(Equal(2))
			Expect(df.ColNames).To(Equal([]string{"BBG000N9MNX3:Close"}))
			Expect(df.Vals).To(Equal([][]float64{{
				729.7700, 735.1100,
			}}))
		})

		It("it uses the cache when it can", func() {
			securities := []*data.Security{
				{
					Ticker:        "TSLA",
					CompositeFigi: "BBG000N9MNX3",
				},
			}

			metrics := []data.Metric{
				data.MetricClose,
			}

			// 2021-01-04  BBG000N9MNX3    719.4600  744.4899  717.1895  729.7700  729.7700   1.000000      0.0000
			// 2021-01-05  BBG000N9MNX3    723.6600  740.8400  719.2000  735.1100  735.1100   1.000000      0.0000
			// 2021-01-06  BBG000N9MNX3    758.4900  774.0000  749.1000  755.9800  755.9800   1.000000      0.0000
			// 2021-01-07  BBG000N9MNX3    777.6300  816.9900  775.2000  816.0400  816.0400   1.000000      0.0000
			// 2021-01-08  BBG000N9MNX3    856.0000  884.4900  838.3900  880.0200  880.0200   1.000000      0.0000
			// 2021-01-11  BBG000N9MNX3    849.4000  854.4300  803.6222  811.1900  811.1900   1.000000      0.0000

			pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 11, 0, 0, 0, 0, common.GetTimezone()), "close", "split_factor", "dividend")
			_, err := manager.GetMetrics(securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 11, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).To(BeNil())

			dfMap, err := manager.GetMetrics(securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 11, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).To(BeNil(), "error when fetching data")
			df := dfMap.DataFrame()

			Expect(df.Len()).To(Equal(6))
			Expect(df.ColCount()).To(Equal(1))
			Expect(df.ColNames).To(Equal([]string{"BBG000N9MNX3:Close"}))
			Expect(df.Vals).To(Equal([][]float64{{
				729.7700, 735.1100, 755.9800, 816.0400, 880.0200, 811.1900,
			}}))
		})

		DescribeTable("check all metrics",
			func(a, b int, metric data.Metric, expectedVals [][]float64) {
				metricColumn := "close" //nolint:goconst
				switch metric {
				case data.MetricAdjustedClose:
					metricColumn = "adj_close" //nolint:goconst
				case data.MetricClose:
					metricColumn = "close" //nolint:goconst
				case data.MetricHigh:
					metricColumn = "high" //nolint:goconst
				case data.MetricOpen:
					metricColumn = "open" //nolint:goconst
				case data.MetricLow:
					metricColumn = "low" //nolint:goconst
				}

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, a, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, b, 0, 0, 0, 0, common.GetTimezone()), metricColumn, "split_factor", "dividend")

				securities := []*data.Security{
					{
						Ticker:        "TSLA",
						CompositeFigi: "BBG000N9MNX3",
					},
				}

				begin := time.Date(2021, 1, a, 16, 0, 0, 0, tz())
				end := time.Date(2021, 1, b, 16, 0, 0, 0, tz())

				colNames := []string{fmt.Sprintf("BBG000N9MNX3:%s", metric)}

				dfMap, err := manager.GetMetrics(securities, []data.Metric{metric}, begin, end)
				Expect(err).To(BeNil(), "error when fetching data")
				df := dfMap.DataFrame()

				Expect(df.Len()).To(Equal(1))
				Expect(df.ColNames).To(Equal(colNames))
				Expect(df.Vals).To(Equal(expectedVals))
			},
			Entry("can fetch close price", 4, 4, data.MetricClose, [][]float64{{729.77}}),
			Entry("can fetch adjusted close price", 4, 4, data.MetricAdjustedClose, [][]float64{{729.77}}),
			Entry("can fetch open price", 4, 4, data.MetricOpen, [][]float64{{719.46}}),
			Entry("can fetch high price", 4, 4, data.MetricHigh, [][]float64{{744.4899}}),
			Entry("can fetch low price", 4, 4, data.MetricLow, [][]float64{{717.1895}}),
		)

		It("fetches trading days at requested frequency when range ends on weekday", func() {
			begin := time.Date(2021, 8, 1, 0, 0, 0, 0, common.GetTimezone())
			end := time.Date(2021, 8, 4, 0, 0, 0, 0, common.GetTimezone())

			tradingDays := manager.TradingDaysAtFrequency(dataframe.Daily, begin, end)
			Expect(tradingDays).To(HaveLen(3))
		})

		It("fetches trading days at requested frequency when range ends on weekend", func() {
			begin := time.Date(2021, 8, 1, 0, 0, 0, 0, common.GetTimezone())
			end := time.Date(2021, 8, 7, 0, 0, 0, 0, common.GetTimezone())

			tradingDays := manager.TradingDaysAtFrequency(dataframe.Daily, begin, end)
			Expect(tradingDays).To(HaveLen(5))
		})
	})
})
