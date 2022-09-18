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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/pgxmockhelper"
)

var _ = Describe("Manager tests", func() {
	var (
		manager *data.Manager
		dbPool  pgxmock.PgxConnIface
	)

	BeforeEach(func() {
		var err error
		dbPool, err = pgxmock.NewConn()
		Expect(err).To(BeNil())
		database.SetPool(dbPool)

		pgxmockhelper.MockManager(dbPool)
		manager = data.GetManagerInstance()
	})

	Context("when fetching metrics", func() {
		It("it errors for unknown securities", func() {
			securities := []*data.Security{
				{
					Ticker:        "UNKNOWN",
					CompositeFigi: "UNKNOWN",
				},
			}

			metrics := []data.Metric{
				data.MetricClose,
			}
			_, err := manager.GetMetrics(securities, metrics, time.Date(2021, 1, 4, 0, 0, 0, 0, common.GetTimezone()), time.Date(2021, 1, 5, 0, 0, 0, 0, common.GetTimezone()))
			Expect(err).ToNot(BeNil())
		})
	})
})
