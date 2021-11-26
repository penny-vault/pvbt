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

package daa_test

import (
	"fmt"
	"io/ioutil"
	"main/common"
	"main/data"
	"main/strategies/daa"
	"time"

	"github.com/goccy/go-json"
	"github.com/rocketlaunchr/dataframe-go"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Daa", func() {
	var (
		strat   *daa.KellersDefensiveAssetAllocation
		manager data.Manager
		tz      *time.Location
		target  *dataframe.DataFrame
		err     error
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time

		jsonParams := `{"riskUniverse": ["VFINX", "PRIDX"], "cashUniverse": ["VUSTX"], "protectiveUniverse": ["VUSTX"], "breadth": 1, "topT": 1}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, err := daa.New(params)
		if err != nil {
			panic(err)
		}
		strat = tmp.(*daa.KellersDefensiveAssetAllocation)

		manager = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

		content, err := ioutil.ReadFile("../testdata/TB3MS.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=GS3M&cosd=1979-07-01&coed=2021-01-01&fq=Close&fam=avg",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VUSTX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1979-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VUSTX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1990-01-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VFINX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1979-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VFINX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1990-01-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/PRIDX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1979-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/PRIDX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1990-01-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/riskfree.csv")
		if err != nil {
			panic(err)
		}

		today := time.Now()
		url := fmt.Sprintf("https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=DGS3MO&cosd=1970-01-01&coed=%d-%02d-%02d&fq=Daily&fam=avg", today.Year(), today.Month(), today.Day())
		httpmock.RegisterResponder("GET", url,
			httpmock.NewBytesResponder(200, content))

		data.InitializeDataManager()
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			BeforeEach(func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)
				target, err = strat.Compute(&manager)
			})

			It("should not error", func() {
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				Expect(target.NRows()).To(Equal(373))
			})

			It("should begin on", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				Expect(val[common.DateIdx].(time.Time)).To(Equal(time.Date(1990, time.January, 31, 16, 0, 0, 0, tz)))
			})

			It("should end on", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				Expect(val[common.DateIdx].(time.Time)).To(Equal(time.Date(2021, time.January, 29, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VUSTX to start", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				t := val[common.TickerName].(map[string]float64)
				Expect(t["VUSTX"]).Should(BeNumerically("~", 1))
			})

			It("should be invested in VUSTX to end", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				t := val[common.TickerName].(map[string]float64)
				Expect(t["VUSTX"]).Should(BeNumerically("~", 1))
			})

			It("should be invested in PRIDX on 1997-11-28", func() {
				val := target.Row(100, true, dataframe.SeriesName)
				t := val[common.TickerName].(map[string]float64)
				Expect(t["PRIDX"]).Should(BeNumerically("~", 1))
			})

			It("should be invested in VFINX on 2006-03-31", func() {
				val := target.Row(200, true, dataframe.SeriesName)
				t := val[common.TickerName].(map[string]float64)
				Expect(t["VFINX"]).Should(BeNumerically("~", 1))
			})

			It("should be invested in VFINX on 2014-07-31", func() {
				val := target.Row(300, true, dataframe.SeriesName)
				t := val[common.TickerName].(map[string]float64)
				Expect(t["VFINX"]).Should(BeNumerically("~", 1))
			})
		})
	})
})
