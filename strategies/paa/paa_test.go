// Copyright 2021-2025
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

package paa_test

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/penny-vault/pv-api/strategies/paa"
)

var _ = Describe("Paa", Ordered, func() {
	var (
		begin   time.Time
		end     time.Time
		dbPool  pgxmock.PgxConnIface
		err     error
		manager *data.Manager
		vfinx   *data.Security
		pridx   *data.Security
		vustx   *data.Security
		strat   *paa.KellersProtectiveAssetAllocation
		tz      *time.Location
	)

	BeforeAll(func() {
		var err error
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// Expect trading days transaction and query
		pgxmockhelper.MockHolidays(dbPool)
		pgxmockhelper.MockAssets(dbPool)
		pgxmockhelper.MockTradingDays(dbPool)
		manager = data.GetManagerInstance()

		vustx, err = data.SecurityFromTicker("VUSTX")
		Expect(err).To(BeNil())
		vfinx, err = data.SecurityFromTicker("VFINX")
		Expect(err).To(BeNil())
		pridx, err = data.SecurityFromTicker("PRIDX")
		Expect(err).To(BeNil())

		tz = common.GetTimezone()
	})

	BeforeEach(func() {
		jsonParams := `{"riskUniverse": [{"compositeFigi": "BBG000BHTMY2", "ticker": "VFINX"}, {"compositeFigi": "BBG000BBVR08", "ticker": "PRIDX"}], "protectiveUniverse": [{"compositeFigi": "BBG000BCKYB1", "ticker": "VUSTX"}], "protectionFactor": 2, "lookback": 12, "topN": 1}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, err := paa.New(params)
		if err != nil {
			panic(err)
		}
		strat = tmp.(*paa.KellersProtectiveAssetAllocation)

		manager.Reset()
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(2020, time.January, 1, 0, 0, 0, 0, tz)
				end = time.Date(2022, time.January, 1, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"dgs3mo.csv",
					},
					time.Date(2019, time.January, 2, 0, 0, 0, 0, time.UTC), time.Date(2022, time.January, 3, 0, 0, 0, 0, time.UTC),
					"adj_close", "split_factor", "dividend")
			})

			It("should not error", func() {
				_, _, err = strat.Compute(context.Background(), begin, end)
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target).To(HaveLen(24))
			})

			DescribeTable("is invested in the correct assets over time", func(idx int, date time.Time, ticker string) {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				pie := target[idx]
				var asset *data.Security
				switch ticker {
				case "vfinx":
					asset = vfinx
				case "vustx":
					asset = vustx
				case "pridx":
					asset = pridx
				}

				Expect(pie.Date).To(Equal(date), "date")
				qty, ok := pie.Members[*asset]
				Expect(ok).To(BeTrue(), fmt.Sprintf("asset should be %s", asset.Ticker))
				Expect(qty).To(BeNumerically("~", 1.0), "quantity")
			},
				Entry("2020-01-31", 0, time.Date(2020, time.January, 31, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2020-02-28", 1, time.Date(2020, time.February, 28, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-03-31", 2, time.Date(2020, time.March, 31, 16, 0, 0, 0, common.GetTimezone()), "vustx"),
				Entry("2020-04-30", 3, time.Date(2020, time.April, 30, 16, 0, 0, 0, common.GetTimezone()), "vustx"),
				Entry("2020-05-29", 4, time.Date(2020, time.May, 29, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-06-30", 5, time.Date(2020, time.June, 30, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-07-31", 6, time.Date(2020, time.July, 31, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-08-31", 7, time.Date(2020, time.August, 31, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-09-30", 8, time.Date(2020, time.September, 30, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-10-30", 9, time.Date(2020, time.October, 30, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-11-30", 10, time.Date(2020, time.November, 30, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2020-12-31", 11, time.Date(2020, time.December, 31, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2021-01-29", 12, time.Date(2021, time.January, 29, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2021-02-26", 13, time.Date(2021, time.February, 26, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2021-03-31", 14, time.Date(2021, time.March, 31, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2021-04-30", 15, time.Date(2021, time.April, 30, 16, 0, 0, 0, common.GetTimezone()), "pridx"),
				Entry("2021-05-28", 16, time.Date(2021, time.May, 28, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-06-30", 17, time.Date(2021, time.June, 30, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-07-30", 18, time.Date(2021, time.July, 30, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-08-31", 19, time.Date(2021, time.August, 31, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-09-30", 20, time.Date(2021, time.September, 30, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-10-29", 21, time.Date(2021, time.October, 29, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
				Entry("2021-11-30", 22, time.Date(2021, time.November, 30, 16, 0, 0, 0, common.GetTimezone()), "vustx"),
				Entry("2021-12-31", 23, time.Date(2021, time.December, 31, 16, 0, 0, 0, common.GetTimezone()), "vfinx"),
			)

			It("predicted should be PRIDX", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				for k := range predicted.Members {
					Expect(k.Ticker).To(Equal("VFINX"))
				}
			})

			It("predicted should be 2022/01/31", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2022, time.January, 31, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("Check predicted portfolio", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(2020, time.January, 1, 0, 0, 0, 0, tz)
				end = time.Date(2020, time.May, 25, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"dgs3mo.csv",
					},
					time.Date(2019, time.January, 2, 0, 0, 0, 0, time.UTC), time.Date(2020, time.May, 25, 0, 0, 0, 0, time.UTC),
					"adj_close", "split_factor", "dividend")
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target).To(HaveLen(4))
			})

			It("should end on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				val := target.Last()
				Expect(val.Date).To(Equal(time.Date(2020, time.April, 30, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VUSTX to end", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				val := target.Last()
				Expect(val.Members[*vustx]).Should(BeNumerically("~", 1))
			})

			It("PRIDX should be predicted asset", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				for k := range predicted.Members {
					Expect(k.Ticker).To(Equal("PRIDX"))
				}
			})

			It("predicted asset should be 5/29", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2020, time.May, 29, 16, 0, 0, 0, tz)))
			})
		})
	})
})
