package strategies_test

import (
	"encoding/json"
	"io/ioutil"
	"main/data"
	"main/portfolio"
	"main/strategies"
	"time"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adm", func() {
	var (
		adm     *strategies.AcceleratingDualMomentum
		manager data.Manager
	)

	BeforeEach(func() {
		jsonParams := `{"inTickers": ["VFINX", "PRIDX"], "outTicker": "VUSTX"}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, _ := strategies.NewAcceleratingDualMomentum(params)
		adm = tmp.(*strategies.AcceleratingDualMomentum)

		manager = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

		content, err := ioutil.ReadFile("testdata/TB3MS.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=TB3MS&cosd=1980-01-01&coed=2021-01-01&fq=AdjustedClose&fam=avg",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VUSTX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VUSTX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VFINX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VFINX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/PRIDX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/PRIDX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			It("should be invested in PRIDX", func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
				perf, err := adm.Compute(&manager)
				Expect(err).To(BeNil())
				Expect(adm.CurrentSymbol).To(Equal("PRIDX"))

				var begin int64
				begin = 617846400
				Expect(perf.PeriodStart).To(Equal(begin))

				var end int64
				end = 1609459200
				Expect(perf.PeriodEnd).To(Equal(end))
				Expect(perf.Value).Should(HaveLen(379))
				Expect(perf.Value[0]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  617846400,
					Value: 10000,
				}))
				Expect(perf.Value[100]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  880675200,
					Value: 42408.6029810143,
				}))
				Expect(perf.Value[200]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1143763200,
					Value: 343579.7507494431,
				}))
				Expect(perf.Value[300]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1406764800,
					Value: 1.1502482646161714e+06,
				}))
				Expect(perf.Value[378]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1611878400,
					Value: 3.279045827906852e+06,
				}))
			})
		})
	})
})
