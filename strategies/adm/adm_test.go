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
	"github.com/jdfergason/dataframe-go"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adm", func() {
	var (
		dbPool  pgxmock.PgxConnIface
		strat   *adm.AcceleratingDualMomentum
		manager data.Manager
		tz      *time.Location
		target  *dataframe.DataFrame
		err     error
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time

		jsonParams := `{"inTickers": ["VFINX", "PRIDX"], "outTicker": "VUSTX"}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, _ := adm.New(params)
		strat = tmp.(*adm.AcceleratingDualMomentum)

		manager = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

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
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)

				pgxmockhelper.MockDBEodQuery(dbPool,
					[]string{
						"vfinx.csv",
						"pridx.csv",
						"vustx.csv",
						"riskfree.csv",
					},
					time.Date(1979, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

				pgxmockhelper.MockDBCorporateQuery(dbPool,
					[]string{
						"vfinx_corporate.csv",
						"pridx_corporate.csv",
						"vustx_corporate.csv",
						"riskfree_corporate.csv",
					},
					time.Date(1979, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

				target, _, err = strat.Compute(context.Background(), &manager)
			})

			It("should not error", func() {
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				Expect(target.NRows()).To(Equal(379))
			})

			It("should begin on", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				Expect(val[common.DateIdx].(time.Time)).To(Equal(time.Date(1989, time.June, 30, 16, 0, 0, 0, tz)))
			})

			It("should end on", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				Expect(val[common.DateIdx].(time.Time)).To(Equal(time.Date(2020, time.December, 31, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VFINX to start", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})

			It("should be invested in PRIDX to end", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("PRIDX"))
			})

			It("should be invested in PRIDX on 1997-11-28", func() {
				val := target.Row(100, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})

			It("should be invested in PRIDX on 2006-03-31", func() {
				val := target.Row(200, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("PRIDX"))
			})

			It("should be invested in VFINX on 2014-07-31", func() {
				val := target.Row(300, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})
		})
	})
})
