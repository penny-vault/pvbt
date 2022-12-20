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

package data_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Request tests", func() {
	var (
		manager *data.Manager
		ctx     context.Context
		dbPool  pgxmock.PgxConnIface
	)

	BeforeEach(func() {
		var err error
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		manager = data.GetManagerInstance()
		manager.Reset()

		ctx = context.Background()
	})

	Context("when fetching metrics", func() {
		It("it errors for unknown securities", func() {
			securities := []*data.Security{
				{
					Ticker:        "UNKNOWN",
					CompositeFigi: "UNKNOWN",
				},
			}

			req := data.NewDataRequest(securities...).Metrics(data.MetricClose)

			_, err := req.Between(ctx, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 0, 0, 0, 0, common.GetTimezone()))
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

			// event_date  composite_figi  open      high      low       close     adj_close  split_factor  dividend
			// 2021-01-04  BBG000N9MNX3    719.4600  744.4899  717.1895  729.7700  729.7700   1.000000      0.0000
			// 2021-01-05  BBG000N9MNX3    723.6600  740.8400  719.2000  735.1100  735.1100   1.000000      0.0000

			pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 23, 59, 59, 0, common.GetTimezone()), "close", "split_factor", "dividend")

			req := data.NewDataRequest(securities...).Metrics(data.MetricClose)
			dfMap, err := req.Between(ctx, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 23, 59, 59, 0, common.GetTimezone()))
			Expect(err).To(BeNil(), "error when fetching data")
			df := dfMap.DataFrame()

			Expect(df.Len()).To(Equal(2))
			Expect(df.ColNames).To(Equal([]string{"BBG000N9MNX3:Close"}))
			Expect(df.Vals).To(Equal([][]float64{{
				729.7700, 735.1100,
			}}))
		})

		DescribeTable("check all metrics",
			func(a, b int, metric data.Metric, expectedVals [][]float64) {
				metricColumn := "close"
				switch metric {
				case data.MetricAdjustedClose:
					metricColumn = "adj_close"
				case data.MetricClose:
					metricColumn = "close"
				case data.MetricHigh:
					metricColumn = "high"
				case data.MetricOpen:
					metricColumn = "open"
				case data.MetricLow:
					metricColumn = "low"
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

				req := data.NewDataRequest(securities...).Metrics(metric)
				dfMap, err := req.Between(ctx, begin, end)
				Expect(err).To(BeNil())
				Expect(err).To(BeNil(), "error when fetching data")
				df := dfMap.DataFrame()

				Expect(df.Len()).To(Equal(1))
				Expect(df.ColNames).To(Equal(colNames))
				Expect(df.Vals).To(Equal(expectedVals))
			},
			Entry("can request close price", 4, 4, data.MetricClose, [][]float64{{729.77}}),
			Entry("can request adjusted close price", 4, 4, data.MetricAdjustedClose, [][]float64{{729.77}}),
			Entry("can request open price", 4, 4, data.MetricOpen, [][]float64{{719.46}}),
			Entry("can request high price", 4, 4, data.MetricHigh, [][]float64{{744.4899}}),
			Entry("can request low price", 4, 4, data.MetricLow, [][]float64{{717.1895}}),
		)

		DescribeTable("request On",
			func(a, b, c int, metric data.Metric, expectedVal float64) {
				metricColumn := "close"
				switch metric {
				case data.MetricAdjustedClose:
					metricColumn = "adj_close"
				case data.MetricClose:
					metricColumn = "close"
				case data.MetricHigh:
					metricColumn = "high"
				case data.MetricOpen:
					metricColumn = "open"
				case data.MetricLow:
					metricColumn = "low"
				}

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, a, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, b, 0, 0, 0, 0, common.GetTimezone()), metricColumn, "split_factor", "dividend")

				securities := []*data.Security{
					{
						Ticker:        "TSLA",
						CompositeFigi: "BBG000N9MNX3",
					},
				}

				_, err := manager.GetMetrics(securities, []data.Metric{metric}, time.Date(2021, 1, a, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, b, 0, 0, 0, 0, common.GetTimezone()))
				Expect(err).To(BeNil(), "error when fetching data for cache")

				dt := time.Date(2021, 1, c, 16, 0, 0, 0, tz())
				req := data.NewDataRequest(securities...).Metrics(metric)
				resMap, err := req.On(dt)
				Expect(err).To(BeNil(), "error when fetching from cache")

				for secMec, val := range resMap {
					Expect(secMec.SecurityObject.Ticker).To(Equal("TSLA"))
					Expect(secMec.MetricObject).To(Equal(metric))
					Expect(val).To(BeNumerically("~", expectedVal))
				}
			},
			Entry("can request close price at begin", 4, 11, 4, data.MetricClose, 729.77),
			Entry("can request adjusted close price at begin", 4, 11, 4, data.MetricAdjustedClose, 729.77),
			Entry("can request open price at begin", 4, 11, 4, data.MetricOpen, 719.46),
			Entry("can request high price at begin", 4, 11, 4, data.MetricHigh, 744.4899),
			Entry("can request low price at begin", 4, 11, 4, data.MetricLow, 717.1895),

			Entry("can request close price in middle", 4, 11, 5, data.MetricClose, 735.11),
			Entry("can request adjusted close price in middle", 4, 11, 5, data.MetricAdjustedClose, 735.11),
			Entry("can request open price in middle", 4, 11, 5, data.MetricOpen, 723.66),
			Entry("can request high price in middle", 4, 11, 5, data.MetricHigh, 740.84),
			Entry("can request low price in middle", 4, 11, 5, data.MetricLow, 719.20),
			Entry("can request open price at end", 4, 11, 11, data.MetricOpen, 849.40),
			Entry("can request high price at end", 4, 11, 11, data.MetricHigh, 854.43),
			Entry("can request low price at end", 4, 11, 11, data.MetricLow, 803.6222),
			Entry("can request close price at end", 4, 11, 11, data.MetricClose, 811.19),
			Entry("can request adjusted close price at end", 4, 11, 11, data.MetricAdjustedClose, 811.19),
		)
	})
})
