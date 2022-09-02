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

var _ = Describe("Provider", func() {
	var (
		ctx       context.Context
		dataProxy *data.ManagerV0
		dbPool    pgxmock.PgxConnIface
		tz        *time.Location
	)

	BeforeEach(func() {
		var err error
		tz = common.GetTimezone()
		Expect(err).To(BeNil())

		ctx = context.Background()

		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		// setup database expectations
		pgxmockhelper.MockAssets(dbPool)
		pgxmockhelper.MockDBEodQuery(dbPool, []string{"riskfree.csv"},
			time.Date(1969, 12, 25, 0, 0, 0, 0, tz), time.Date(2022, 6, 16, 0, 0, 0, 0, tz),
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 6, 16, 0, 0, 0, 0, time.UTC))
		pgxmockhelper.MockDBCorporateQuery(dbPool, []string{"riskfree_corporate.csv"},
			time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC))
		data.InitializeDataManager()
		dataProxy = data.NewManager()

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
