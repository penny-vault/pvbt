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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("PVDB tests", func() {
	var (
		dbPool pgxmock.PgxConnIface
		pvdb   *data.PvDb
		ctx    context.Context
	)

	BeforeEach(func() {
		var err error
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)
		pvdb = data.NewPvDb()
		ctx = context.Background()
	})

	Context("when interacting with pvdb", func() {
		It("it fetches TSLA when only ticker is present", func() {
			securities := []*data.Security{
				{
					Ticker:        "TSLA",
					CompositeFigi: "UNKNOWN",
				},
			}

			metrics := []data.Metric{
				data.MetricClose,
			}

			security, err := data.SecurityFromFigi("BBG000N9MNX3")
			Expect(err).To(BeNil(), "security from figi")

			securityMetric := data.SecurityMetric{
				SecurityObject: *security,
				MetricObject:   data.MetricClose,
			}

			// event_date  composite_figi  open      high      low       close     adj_close  split_factor  dividend
			// 2021-01-04  BBG000N9MNX3    719.4600  744.4899  717.1895  729.7700  729.7700   1.000000      0.0000
			// 2021-01-05  BBG000N9MNX3    723.6600  740.8400  719.2000  735.1100  735.1100   1.000000      0.0000

			pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 23, 59, 59, 999999999, common.GetTimezone()), "close")
			df, err := pvdb.GetEOD(ctx, securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 23, 59, 59, 999999999, common.GetTimezone()))
			Expect(err).To(BeNil())
			Expect(len(df)).To(Equal(1))
			Expect(df[securityMetric.String()].Vals[0]).To(Equal([]float64{
				729.7700, 735.1100,
			}))
		})

		It("it does not error when no data is available", func() {
			securities := []*data.Security{
				{
					Ticker:        "TSLA",
					CompositeFigi: "UNKNOWN",
				},
			}

			security, err := data.SecurityFromFigi("BBG000N9MNX3")
			Expect(err).To(BeNil(), "security from figi")

			securityMetric := data.SecurityMetric{
				SecurityObject: *security,
				MetricObject:   data.MetricClose,
			}

			metrics := []data.Metric{
				data.MetricClose,
			}

			pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, 3, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 3, 0, 0, 0, 0, common.GetTimezone()), "close", "split_factor", "dividend")
			res, err := pvdb.GetEOD(ctx, securities, metrics, time.Date(2021, 1, 3, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 3, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).To(BeNil())
			Expect(res[securityMetric.String()]).To(BeNil())
		})

		DescribeTable("check all metrics",
			func(a, b int, metric data.Metric, expectedVals []float64) {
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

				pgxmockhelper.MockDBEodQuery(dbPool, []string{"tsla.csv"}, time.Date(2021, 1, a, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, b, 0, 0, 0, 0, common.GetTimezone()), metricColumn)

				securities := []*data.Security{
					{
						Ticker:        "TSLA",
						CompositeFigi: "BBG000N9MNX3",
					},
				}

				security, err := data.SecurityFromFigi("BBG000N9MNX3")
				Expect(err).To(BeNil(), "security from figi")

				securityMetric := data.SecurityMetric{
					SecurityObject: *security,
					MetricObject:   metric,
				}

				begin := time.Date(2021, 1, a, 0, 0, 0, 0, tz())
				end := time.Date(2021, 1, b, 0, 0, 0, 0, tz())

				df, err := pvdb.GetEOD(ctx, securities, []data.Metric{metric}, begin, end)
				Expect(err).To(BeNil())
				Expect(len(df)).To(Equal(1))
				Expect(df[securityMetric.String()].Vals[0]).To(Equal(expectedVals))
			},
			Entry("can fetch close price", 4, 4, data.MetricClose, []float64{729.77}),
			Entry("can fetch adjusted close price", 4, 4, data.MetricAdjustedClose, []float64{729.77}),
			Entry("can fetch open price", 4, 4, data.MetricOpen, []float64{719.46}),
			Entry("can fetch high price", 4, 4, data.MetricHigh, []float64{744.4899}),
			Entry("can fetch low price", 4, 4, data.MetricLow, []float64{717.1895}),
		)
	})
})
