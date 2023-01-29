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

package adm_test

import (
	"context"
	"fmt"
	"time"

	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/penny-vault/pv-api/strategies/adm"

	"github.com/goccy/go-json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adm", Ordered, func() {
	var (
		begin   time.Time
		end     time.Time
		dbPool  pgxmock.PgxConnIface
		err     error
		strat   *adm.AcceleratingDualMomentum
		tz      *time.Location
		vustx   *data.Security
		vfinx   *data.Security
		pridx   *data.Security
		manager *data.Manager
	)

	BeforeAll(func() {
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
	})

	BeforeEach(func() {
		tz = common.GetTimezone()
		jsonParams := `{"inTickers": [{"compositeFigi": "BBG000BHTMY2", "ticker": "VFINX"}, {"compositeFigi": "BBG000BBVR08", "ticker": "PRIDX"}], "outTickers": [{"compositeFigi": "BBG000BCKYB1", "ticker": "VUSTX"}]}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, err := adm.New(params)
		Expect(err).To(BeNil())
		strat = tmp.(*adm.AcceleratingDualMomentum)
		manager.Reset()
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(2019, time.July, 1, 0, 0, 0, 0, tz)
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
				_, _, err := strat.Compute(context.Background(), begin, end)
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(len(target)).To(Equal(30))
			})

			It("should begin on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target[0].Date).To(Equal(time.Date(2019, time.July, 31, 16, 0, 0, 0, tz)))
			})

			It("should end on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target.Last().Date).To(Equal(time.Date(2021, time.December, 31, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VFINX to start", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[0].Members[*vfinx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("should be invested in VFINX to end", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target.Last().Members[*vfinx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("be fully invested in correct assets over investment period", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)

				expectedDates := []time.Time{
					time.Date(2019, 7, 31, 16, 0, 0, 0, tz),
					time.Date(2019, 8, 30, 16, 0, 0, 0, tz),
					time.Date(2019, 9, 30, 16, 0, 0, 0, tz),
					time.Date(2019, 10, 31, 16, 0, 0, 0, tz),
					time.Date(2019, 11, 29, 16, 0, 0, 0, tz),
					time.Date(2019, 12, 31, 16, 0, 0, 0, tz),
					time.Date(2020, 1, 31, 16, 0, 0, 0, tz),
					time.Date(2020, 2, 28, 16, 0, 0, 0, tz),
					time.Date(2020, 3, 31, 16, 0, 0, 0, tz),
					time.Date(2020, 4, 30, 16, 0, 0, 0, tz),
					time.Date(2020, 5, 29, 16, 0, 0, 0, tz),
					time.Date(2020, 6, 30, 16, 0, 0, 0, tz),
					time.Date(2020, 7, 31, 16, 0, 0, 0, tz),
					time.Date(2020, 8, 31, 16, 0, 0, 0, tz),
					time.Date(2020, 9, 30, 16, 0, 0, 0, tz),
					time.Date(2020, 10, 30, 16, 0, 0, 0, tz),
					time.Date(2020, 11, 30, 16, 0, 0, 0, tz),
					time.Date(2020, 12, 31, 16, 0, 0, 0, tz),
					time.Date(2021, 1, 29, 16, 0, 0, 0, tz),
					time.Date(2021, 2, 26, 16, 0, 0, 0, tz),
					time.Date(2021, 3, 31, 16, 0, 0, 0, tz),
					time.Date(2021, 4, 30, 16, 0, 0, 0, tz),
					time.Date(2021, 5, 28, 16, 0, 0, 0, tz),
					time.Date(2021, 6, 30, 16, 0, 0, 0, tz),
					time.Date(2021, 7, 30, 16, 0, 0, 0, tz),
					time.Date(2021, 8, 31, 16, 0, 0, 0, tz),
					time.Date(2021, 9, 30, 16, 0, 0, 0, tz),
					time.Date(2021, 10, 29, 16, 0, 0, 0, tz),
					time.Date(2021, 11, 30, 16, 0, 0, 0, tz),
					time.Date(2021, 12, 31, 16, 0, 0, 0, tz),
				}

				expectedSecurities := []data.Security{
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*pridx,
					*vfinx,
					*vustx,
					*vustx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*pridx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
					*vfinx,
				}

				for idx := range expectedDates {
					v, ok := target[idx].Members[expectedSecurities[idx]]
					var actual data.Security
					for k := range target[idx].Members {
						actual = k
						break
					}
					Expect(ok).To(BeTrue(), fmt.Sprintf("[%d] securities match (%s != %s)", idx, expectedSecurities[idx].Ticker, actual.Ticker))
					Expect(v).To(BeNumerically("~", 1.0), fmt.Sprintf("[%d] percent matches", idx))
					Expect(target[idx].Date).To(Equal(expectedDates[idx]), fmt.Sprintf("[%d] date", idx))
				}
			})

			It("predicted should be VFINX", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				v, ok := predicted.Members[*vfinx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("predicted date should be 2022/01/31", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2022, time.January, 31, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("Check predicted portfolio", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(2019, time.July, 1, 0, 0, 0, 0, tz)
				end = time.Date(2020, time.April, 29, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"dgs3mo.csv",
					},
					time.Date(2019, time.January, 2, 0, 0, 0, 0, time.UTC), time.Date(2020, time.April, 30, 0, 0, 0, 0, time.UTC),
					"adj_close", "split_factor", "dividend")
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(len(target)).To(Equal(9))
			})

			It("should end on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target.Last().Date).To(Equal(time.Date(2020, time.March, 31, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VUSTX to end", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[len(target)-1].Members[*vustx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("PRIDX should be predicted asset", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				v, ok := predicted.Members[*pridx]
				var actual string
				for k := range predicted.Members {
					actual = k.Ticker
				}
				Expect(ok).To(BeTrue(), fmt.Sprintf("check security: PRIDX != %s", actual))
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("predicted asset should be 4/30", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2020, time.April, 30, 16, 0, 0, 0, tz)))
			})
		})
	})
})
