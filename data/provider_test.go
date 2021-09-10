// Copyright 2021 JD Fergason
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
	"fmt"
	"io/ioutil"
	"time"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"main/data"
)

var _ = Describe("Provider", func() {
	var (
		dataProxy data.Manager
		tz        *time.Location
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time

		content, err := ioutil.ReadFile("testdata/VFINX.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/riskfree.csv")
		if err != nil {
			panic(err)
		}

		today := time.Now()
		url := fmt.Sprintf("https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=DGS3MO&cosd=1970-01-01&coed=%d-%02d-%02d&fq=Daily&fam=avg", today.Year(), today.Month(), today.Day())
		httpmock.RegisterResponder("GET", url,
			httpmock.NewBytesResponder(200, content))

		data.InitializeDataManager()

		dataProxy = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

		dataProxy.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
		dataProxy.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)
		dataProxy.Frequency = data.FrequencyMonthly
	})

	Describe("When data framework is initialized", func() {
		Context("with the DGS3MO data", func() {
			It("should be able to retrieve the risk free rate", func() {
				rate := dataProxy.RiskFreeRate(time.Date(1982, 7, 27, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 10.66, 1e-2))

				rate = dataProxy.RiskFreeRate(time.Date(1984, 12, 18, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 7.81, 1e-2))

			})

			It("should be able to retrieve the risk free rate for out-of-order dates", func() {
				rate := dataProxy.RiskFreeRate(time.Date(1982, 7, 27, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 10.66, 1e-2))

				rate = dataProxy.RiskFreeRate(time.Date(1984, 12, 18, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 7.81, 1e-2))

				rate = dataProxy.RiskFreeRate(time.Date(1983, 1, 18, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 7.64, 1e-2))
			})

			It("should be able to retrieve the risk free rate on days FRED returns NaN", func() {
				rate := dataProxy.RiskFreeRate(time.Date(2019, 1, 1, 0, 0, 0, 0, tz))
				Expect(rate).Should(BeNumerically("~", 2.4, 1e-2))
			})

		})
	})
})
