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

package data_test

import (
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

			It("successfully filters by day", func() {
				days := data.FilterDays(dataframe.Daily, dates)
				Expect(len(days)).To(Equal(252))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[251]).To(Equal(time.Date(2021, 12, 31, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by week begin", func() {
				days := data.FilterDays(dataframe.WeekBegin, dates)
				Expect(len(days)).To(Equal(52))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2021, 1, 11, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2021, 1, 19, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2021, 1, 25, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2021, 2, 1, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2021, 2, 8, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2021, 2, 16, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2021, 2, 22, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2021, 3, 1, 0, 0, 0, 0, tz())))
				Expect(days[9]).To(Equal(time.Date(2021, 3, 8, 0, 0, 0, 0, tz())))
				Expect(days[10]).To(Equal(time.Date(2021, 3, 15, 0, 0, 0, 0, tz())))
				Expect(days[11]).To(Equal(time.Date(2021, 3, 22, 0, 0, 0, 0, tz())))
				Expect(days[12]).To(Equal(time.Date(2021, 3, 29, 0, 0, 0, 0, tz())))
				Expect(days[13]).To(Equal(time.Date(2021, 4, 5, 0, 0, 0, 0, tz())))
				Expect(days[14]).To(Equal(time.Date(2021, 4, 12, 0, 0, 0, 0, tz())))
				Expect(days[15]).To(Equal(time.Date(2021, 4, 19, 0, 0, 0, 0, tz())))
				Expect(days[16]).To(Equal(time.Date(2021, 4, 26, 0, 0, 0, 0, tz())))
				Expect(days[17]).To(Equal(time.Date(2021, 5, 3, 0, 0, 0, 0, tz())))
				Expect(days[18]).To(Equal(time.Date(2021, 5, 10, 0, 0, 0, 0, tz())))
				Expect(days[19]).To(Equal(time.Date(2021, 5, 17, 0, 0, 0, 0, tz())))
				Expect(days[20]).To(Equal(time.Date(2021, 5, 24, 0, 0, 0, 0, tz())))
				Expect(days[21]).To(Equal(time.Date(2021, 6, 1, 0, 0, 0, 0, tz())))
				Expect(days[22]).To(Equal(time.Date(2021, 6, 7, 0, 0, 0, 0, tz())))
				Expect(days[23]).To(Equal(time.Date(2021, 6, 14, 0, 0, 0, 0, tz())))
				Expect(days[24]).To(Equal(time.Date(2021, 6, 21, 0, 0, 0, 0, tz())))
				Expect(days[25]).To(Equal(time.Date(2021, 6, 28, 0, 0, 0, 0, tz())))
				Expect(days[26]).To(Equal(time.Date(2021, 7, 6, 0, 0, 0, 0, tz())))
				Expect(days[27]).To(Equal(time.Date(2021, 7, 12, 0, 0, 0, 0, tz())))
				Expect(days[28]).To(Equal(time.Date(2021, 7, 19, 0, 0, 0, 0, tz())))
				Expect(days[29]).To(Equal(time.Date(2021, 7, 26, 0, 0, 0, 0, tz())))
				Expect(days[30]).To(Equal(time.Date(2021, 8, 2, 0, 0, 0, 0, tz())))
				Expect(days[31]).To(Equal(time.Date(2021, 8, 9, 0, 0, 0, 0, tz())))
				Expect(days[32]).To(Equal(time.Date(2021, 8, 16, 0, 0, 0, 0, tz())))
				Expect(days[33]).To(Equal(time.Date(2021, 8, 23, 0, 0, 0, 0, tz())))
				Expect(days[34]).To(Equal(time.Date(2021, 8, 30, 0, 0, 0, 0, tz())))
				Expect(days[35]).To(Equal(time.Date(2021, 9, 7, 0, 0, 0, 0, tz())))
				Expect(days[36]).To(Equal(time.Date(2021, 9, 13, 0, 0, 0, 0, tz())))
				Expect(days[37]).To(Equal(time.Date(2021, 9, 20, 0, 0, 0, 0, tz())))
				Expect(days[38]).To(Equal(time.Date(2021, 9, 27, 0, 0, 0, 0, tz())))
				Expect(days[39]).To(Equal(time.Date(2021, 10, 4, 0, 0, 0, 0, tz())))
				Expect(days[40]).To(Equal(time.Date(2021, 10, 11, 0, 0, 0, 0, tz())))
				Expect(days[41]).To(Equal(time.Date(2021, 10, 18, 0, 0, 0, 0, tz())))
				Expect(days[42]).To(Equal(time.Date(2021, 10, 25, 0, 0, 0, 0, tz())))
				Expect(days[43]).To(Equal(time.Date(2021, 11, 1, 0, 0, 0, 0, tz())))
				Expect(days[44]).To(Equal(time.Date(2021, 11, 8, 0, 0, 0, 0, tz())))
				Expect(days[45]).To(Equal(time.Date(2021, 11, 15, 0, 0, 0, 0, tz())))
				Expect(days[46]).To(Equal(time.Date(2021, 11, 22, 0, 0, 0, 0, tz())))
				Expect(days[47]).To(Equal(time.Date(2021, 11, 29, 0, 0, 0, 0, tz())))
				Expect(days[48]).To(Equal(time.Date(2021, 12, 6, 0, 0, 0, 0, tz())))
				Expect(days[49]).To(Equal(time.Date(2021, 12, 13, 0, 0, 0, 0, tz())))
				Expect(days[50]).To(Equal(time.Date(2021, 12, 20, 0, 0, 0, 0, tz())))
				Expect(days[51]).To(Equal(time.Date(2021, 12, 27, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by week end", func() {
				days := data.FilterDays(dataframe.WeekEnd, dates)
				Expect(len(days)).To(Equal(52))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 8, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2021, 1, 15, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2021, 1, 22, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2021, 1, 29, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2021, 2, 5, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2021, 2, 12, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2021, 2, 19, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2021, 2, 26, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2021, 3, 5, 0, 0, 0, 0, tz())))
				Expect(days[9]).To(Equal(time.Date(2021, 3, 12, 0, 0, 0, 0, tz())))
				Expect(days[10]).To(Equal(time.Date(2021, 3, 19, 0, 0, 0, 0, tz())))
				Expect(days[11]).To(Equal(time.Date(2021, 3, 26, 0, 0, 0, 0, tz())))
				Expect(days[12]).To(Equal(time.Date(2021, 4, 1, 0, 0, 0, 0, tz())))
				Expect(days[13]).To(Equal(time.Date(2021, 4, 9, 0, 0, 0, 0, tz())))
				Expect(days[14]).To(Equal(time.Date(2021, 4, 16, 0, 0, 0, 0, tz())))
				Expect(days[15]).To(Equal(time.Date(2021, 4, 23, 0, 0, 0, 0, tz())))
				Expect(days[16]).To(Equal(time.Date(2021, 4, 30, 0, 0, 0, 0, tz())))
				Expect(days[17]).To(Equal(time.Date(2021, 5, 7, 0, 0, 0, 0, tz())))
				Expect(days[18]).To(Equal(time.Date(2021, 5, 14, 0, 0, 0, 0, tz())))
				Expect(days[19]).To(Equal(time.Date(2021, 5, 21, 0, 0, 0, 0, tz())))
				Expect(days[20]).To(Equal(time.Date(2021, 5, 28, 0, 0, 0, 0, tz())))
				Expect(days[21]).To(Equal(time.Date(2021, 6, 4, 0, 0, 0, 0, tz())))
				Expect(days[22]).To(Equal(time.Date(2021, 6, 11, 0, 0, 0, 0, tz())))
				Expect(days[23]).To(Equal(time.Date(2021, 6, 18, 0, 0, 0, 0, tz())))
				Expect(days[24]).To(Equal(time.Date(2021, 6, 25, 0, 0, 0, 0, tz())))
				Expect(days[25]).To(Equal(time.Date(2021, 7, 2, 0, 0, 0, 0, tz())))
				Expect(days[26]).To(Equal(time.Date(2021, 7, 9, 0, 0, 0, 0, tz())))
				Expect(days[27]).To(Equal(time.Date(2021, 7, 16, 0, 0, 0, 0, tz())))
				Expect(days[28]).To(Equal(time.Date(2021, 7, 23, 0, 0, 0, 0, tz())))
				Expect(days[29]).To(Equal(time.Date(2021, 7, 30, 0, 0, 0, 0, tz())))
				Expect(days[30]).To(Equal(time.Date(2021, 8, 6, 0, 0, 0, 0, tz())))
				Expect(days[31]).To(Equal(time.Date(2021, 8, 13, 0, 0, 0, 0, tz())))
				Expect(days[32]).To(Equal(time.Date(2021, 8, 20, 0, 0, 0, 0, tz())))
				Expect(days[33]).To(Equal(time.Date(2021, 8, 27, 0, 0, 0, 0, tz())))
				Expect(days[34]).To(Equal(time.Date(2021, 9, 3, 0, 0, 0, 0, tz())))
				Expect(days[35]).To(Equal(time.Date(2021, 9, 10, 0, 0, 0, 0, tz())))
				Expect(days[36]).To(Equal(time.Date(2021, 9, 17, 0, 0, 0, 0, tz())))
				Expect(days[37]).To(Equal(time.Date(2021, 9, 24, 0, 0, 0, 0, tz())))
				Expect(days[38]).To(Equal(time.Date(2021, 10, 1, 0, 0, 0, 0, tz())))
				Expect(days[39]).To(Equal(time.Date(2021, 10, 8, 0, 0, 0, 0, tz())))
				Expect(days[40]).To(Equal(time.Date(2021, 10, 15, 0, 0, 0, 0, tz())))
				Expect(days[41]).To(Equal(time.Date(2021, 10, 22, 0, 0, 0, 0, tz())))
				Expect(days[42]).To(Equal(time.Date(2021, 10, 29, 0, 0, 0, 0, tz())))
				Expect(days[43]).To(Equal(time.Date(2021, 11, 5, 0, 0, 0, 0, tz())))
				Expect(days[44]).To(Equal(time.Date(2021, 11, 12, 0, 0, 0, 0, tz())))
				Expect(days[45]).To(Equal(time.Date(2021, 11, 19, 0, 0, 0, 0, tz())))
				Expect(days[46]).To(Equal(time.Date(2021, 11, 26, 0, 0, 0, 0, tz())))
				Expect(days[47]).To(Equal(time.Date(2021, 12, 3, 0, 0, 0, 0, tz())))
				Expect(days[48]).To(Equal(time.Date(2021, 12, 10, 0, 0, 0, 0, tz())))
				Expect(days[49]).To(Equal(time.Date(2021, 12, 17, 0, 0, 0, 0, tz())))
				Expect(days[50]).To(Equal(time.Date(2021, 12, 23, 0, 0, 0, 0, tz())))
				Expect(days[51]).To(Equal(time.Date(2021, 12, 31, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by month begin", func() {
				days := data.FilterDays(dataframe.MonthBegin, dates)
				Expect(len(days)).To(Equal(12))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2021, 2, 1, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2021, 3, 1, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2021, 4, 1, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2021, 5, 3, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2021, 6, 1, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2021, 7, 1, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2021, 8, 2, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2021, 9, 1, 0, 0, 0, 0, tz())))
				Expect(days[9]).To(Equal(time.Date(2021, 10, 1, 0, 0, 0, 0, tz())))
				Expect(days[10]).To(Equal(time.Date(2021, 11, 1, 0, 0, 0, 0, tz())))
				Expect(days[11]).To(Equal(time.Date(2021, 12, 1, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by month end", func() {
				days := data.FilterDays(dataframe.MonthEnd, dates)
				Expect(len(days)).To(Equal(12))
				Expect(days[0]).To(Equal(time.Date(2021, 1, 29, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2021, 2, 26, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2021, 3, 31, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2021, 4, 30, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2021, 5, 28, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2021, 6, 30, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2021, 7, 30, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2021, 8, 31, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2021, 9, 30, 0, 0, 0, 0, tz())))
				Expect(days[9]).To(Equal(time.Date(2021, 10, 29, 0, 0, 0, 0, tz())))
				Expect(days[10]).To(Equal(time.Date(2021, 11, 30, 0, 0, 0, 0, tz())))
				Expect(days[11]).To(Equal(time.Date(2021, 12, 31, 0, 0, 0, 0, tz())))
			})
		})

		Context("with 10 years of dates", func() {
			var (
				dates  []time.Time
				dbPool pgxmock.PgxConnIface
			)

			BeforeEach(func() {
				currDate := time.Date(2010, 1, 1, 0, 0, 0, 0, tz())
				dates = make([]time.Time, 3650)
				dates[0] = currDate

				for ii := 1; ii < 3650; ii++ {
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

			It("successfully filters by year begin", func() {
				days := data.FilterDays(dataframe.YearBegin, dates)
				Expect(len(days)).To(Equal(10))
				Expect(days[0]).To(Equal(time.Date(2010, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2011, 1, 3, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2012, 1, 3, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2013, 1, 2, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2014, 1, 2, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2015, 1, 2, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2016, 1, 4, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2017, 1, 3, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2018, 1, 2, 0, 0, 0, 0, tz())))
				Expect(days[9]).To(Equal(time.Date(2019, 1, 2, 0, 0, 0, 0, tz())))
			})

			It("successfully filters by year end", func() {
				days := data.FilterDays(dataframe.YearEnd, dates)
				Expect(len(days)).To(Equal(9))
				Expect(days[0]).To(Equal(time.Date(2010, 12, 31, 0, 0, 0, 0, tz())))
				Expect(days[1]).To(Equal(time.Date(2011, 12, 30, 0, 0, 0, 0, tz())))
				Expect(days[2]).To(Equal(time.Date(2012, 12, 31, 0, 0, 0, 0, tz())))
				Expect(days[3]).To(Equal(time.Date(2013, 12, 31, 0, 0, 0, 0, tz())))
				Expect(days[4]).To(Equal(time.Date(2014, 12, 31, 0, 0, 0, 0, tz())))
				Expect(days[5]).To(Equal(time.Date(2015, 12, 31, 0, 0, 0, 0, tz())))
				Expect(days[6]).To(Equal(time.Date(2016, 12, 30, 0, 0, 0, 0, tz())))
				Expect(days[7]).To(Equal(time.Date(2017, 12, 29, 0, 0, 0, 0, tz())))
				Expect(days[8]).To(Equal(time.Date(2018, 12, 31, 0, 0, 0, 0, tz())))
			})
		})
	})
})
