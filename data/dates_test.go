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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/dataframe"
	"github.com/penny-vault/pv-api/pgxmockhelper"
	"github.com/penny-vault/pv-api/tradecron"
)

var _ = Describe("FilterDates test", func() {
	Describe("When filtering date arrays", func() {
		Context("with 1 year of dates", func() {
			var (
				dates  []time.Time
				dbPool pgxmock.PgxConnIface
			)

			BeforeEach(func() {
				currDate := time.Date(2021, 1, 1, 0, 0, 0, 0, tz())
				dates = make([]time.Time, 365)
				dates[0] = currDate

				for ii := 1; ii < 365; ii++ {
					currDate = currDate.AddDate(0, 0, 1)
					dates[ii] = currDate
				}

				var err error
				dbPool, err = pgxmock.NewConn()
				Expect(err).To(BeNil())
				database.SetPool(dbPool)

				pgxmockhelper.MockHolidays(dbPool)
				tradecron.LoadMarketHolidays()

			})

			It("successfully filters by day", Pending, func() {
				days := data.FilterDays(dataframe.Daily, dates)
				Expect(len(days)).To(Equal(252))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[251]).To(Equal(time.Date(2021, 12, 31, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by week begin", func() {
				days := data.FilterDays(dataframe.WeekBegin, dates)
				fmt.Println(days)
				Expect(len(days)).To(Equal(51))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[50]).To(Equal(time.Date(2021, 12, 27, 0, 0, 0, 0, tz())))
			})

		})
	})
})
