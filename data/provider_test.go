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

package data_test

import (
	"context"
	"time"

	"github.com/jackc/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Provider", func() {
	var (
		ctx       context.Context
		dataProxy data.Manager
		dbPool    pgxmock.PgxConnIface
		tz        *time.Location
	)

	BeforeEach(func() {
		var err error
		tz, err = time.LoadLocation("America/New_York") // New York is the reference time
		Expect(err).To(BeNil())

		ctx = context.Background()

		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// setup database expectations

		// Expect trading days transaction and query
		dbPool.ExpectBegin()
		// NOTE: pgconn.CommandTag is ignored
		dbPool.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))

		rows, err := pgxmockhelper.RowsFromCSV("../testdata/tradingdays.csv", map[string]string{
			"trade_day": "date",
		})
		Expect(err).To(BeNil())
		dbPool.ExpectQuery("SELECT").WillReturnRows(rows)

		// Expect dataframe transaction and query
		dbPool.ExpectBegin()
		// NOTE: pgconn.CommandTag is ignored
		dbPool.ExpectExec("SET ROLE").WillReturnResult(pgconn.CommandTag("SET ROLE"))

		rows, err = pgxmockhelper.RowsFromCSV("../testdata/riskfree.csv", map[string]string{
			"event_date": "date",
			"val":        "float64",
			"close":      "float64",
			"adj_close":  "float64",
		})
		Expect(err).To(BeNil())
		dbPool.ExpectQuery("SELECT").WillReturnRows(rows)

		data.InitializeDataManager()

		dataProxy = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

		dataProxy.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
		dataProxy.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)
		dataProxy.Frequency = data.FrequencyMonthly
	})

	AfterEach(func() {
		dbPool.Close(context.Background())
	})

	DescribeTable("Requesting the risk free rate",
		func(year, month, day int, riskFreeRate float64) {
			rate := dataProxy.RiskFreeRate(ctx, time.Date(year, time.Month(month), day, 0, 0, 0, 0, tz))
			Expect(rate).Should(BeNumerically("~", riskFreeRate, 1e-2))
		},
		Entry("When the date is before first available data", 1980, 1, 1, .05),
		Entry("When the date is the first available data", 1980, 1, 2, .05),
		Entry("When the date is a random day in the middle of the dataset", 1982, 7, 27, 11.12),
		Entry("When the date is the last available date in the dataset", 2022, 6, 16, 1.74),
		Entry("When the date is after the last available date", 2022, 6, 17, 1.74),
		Entry("When the date is on a day where FRED returns NaN", 2019, 1, 1, 2.42),
	)

	Describe("When data framework is initialized", func() {
		Context("with the DGS3MO data", func() {
			It("should be able to retrieve the risk free rate for out-of-order dates", func() {
				rate := dataProxy.RiskFreeRate(ctx, time.Date(1982, 7, 27, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 11.12, 1e-2))

				rate = dataProxy.RiskFreeRate(ctx, time.Date(1984, 12, 18, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 8.08, 1e-2))

				rate = dataProxy.RiskFreeRate(ctx, time.Date(1983, 1, 18, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 7.9, 1e-2))
			})
		})
	})
})
