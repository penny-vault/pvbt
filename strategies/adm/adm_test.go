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

package adm_test

import (
	"context"
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
		begin  time.Time
		end    time.Time
		dbPool pgxmock.PgxConnIface
		err    error
		strat  *adm.AcceleratingDualMomentum
		tz     *time.Location
		vustx  *data.Security
		vfinx  *data.Security
		pridx  *data.Security
	)

	BeforeAll(func() {
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// Expect trading days transaction and query
		pgxmockhelper.MockHolidays(dbPool)
		pgxmockhelper.MockAssets(dbPool)
		pgxmockhelper.MockTradingDays(dbPool)
		data.GetManagerInstance()

		vustx, err = data.SecurityFromTicker("VUSTX")
		Expect(err).To(BeNil())
		vfinx, err = data.SecurityFromTicker("VFINX")
		Expect(err).To(BeNil())
		pridx, err = data.SecurityFromTicker("PRIDX")
		Expect(err).To(BeNil())
	})

	BeforeEach(func() {
		tz = common.GetTimezone()
		jsonParams := `{"inTickers": [{"compositeFigi": "BBG000BHTMY2", "ticker": "VFINX"}, {"compositeFigi": "BBG000BBVR08", "ticker": "PRIDX"}], "outTicker": {"compositeFigi": "BBG000BCKYB1", "ticker": "VUSTX"}}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, _ := adm.New(params)
		strat = tmp.(*adm.AcceleratingDualMomentum)
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				end = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"dgs3mo.csv",
					},
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
					"adj_close", "split_factor", "dividend")
			})

			It("should not error", func() {
				_, _, err := strat.Compute(context.Background(), begin, end)
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(len(target)).To(Equal(379))
			})

			It("should begin on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target[0].Date).To(Equal(time.Date(1989, time.June, 30, 16, 0, 0, 0, tz)))
			})

			It("should end on", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(target.Last().Date).To(Equal(time.Date(2020, time.December, 31, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VFINX to start", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[0].Members[*vfinx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("should be invested in PRIDX to end", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target.Last().Members[*pridx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("should be invested in PRIDX on 1997-11-28", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[100].Members[*pridx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("should be invested in PRIDX on 2006-03-31", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[200].Members[*pridx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("should be invested in VFINX on 2014-07-31", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				v, ok := target[300].Members[*vfinx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("predicted should be PRIDX", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				v, ok := predicted.Members[*pridx]
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("predicted date should be 2021/01/29", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2021, time.January, 29, 16, 0, 0, 0, tz)))
			})
		})
	})

	Describe("Check predicted portfolio", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				end = time.Date(2020, time.April, 29, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"dgs3mo.csv",
					},
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, time.April, 29, 0, 0, 0, 0, time.UTC),
					"adj_close", "split_factor", "dividend")
			})

			It("should have length", func() {
				target, _, _ := strat.Compute(context.Background(), begin, end)
				Expect(len(target)).To(Equal(370))
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
				Expect(ok).To(BeTrue())
				Expect(v).To(BeNumerically("~", 1.0))
			})

			It("predicted asset should be 4/30", func() {
				_, predicted, _ := strat.Compute(context.Background(), begin, end)
				Expect(predicted.Date).To(Equal(time.Date(2020, time.April, 30, 16, 0, 0, 0, tz)))
			})
		})
	})
})
