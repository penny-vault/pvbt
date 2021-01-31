package portfolio_test

import (
	"io/ioutil"
	"time"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rocketlaunchr/dataframe-go"

	"main/data"
	"main/portfolio"
)

var _ = Describe("Portfolio", func() {
	var (
		p         portfolio.Portfolio
		df1       *dataframe.DataFrame
		dataProxy data.Manager
	)

	BeforeEach(func() {
		content, err := ioutil.ReadFile("testdata/VUSTX.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VFINX.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/PRIDX.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1980-01-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		// -- Portfolio performance data

		content, err = ioutil.ReadFile("testdata/VUSTX_2.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=2018-01-31&endDate=2020-11-30&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/VFINX_2.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=2018-01-31&endDate=2020-11-30&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("testdata/PRIDX_2.csv")
		if err != nil {
			panic(err)
		}
		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=2018-01-31&endDate=2020-11-30&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		dataProxy = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})
		dataProxy.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
		dataProxy.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
		dataProxy.Frequency = data.FrequencyMonthly

		p = portfolio.NewPortfolio("Test", &dataProxy)

		timeSeries := dataframe.NewSeriesTime(data.DateIdx, &dataframe.SeriesInit{Size: 3}, []time.Time{
			time.Date(2018, time.January, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2019, time.January, 31, 0, 0, 0, 0, time.UTC),
			time.Date(2020, time.January, 31, 0, 0, 0, 0, time.UTC),
		})

		tickerSeries := dataframe.NewSeriesString(portfolio.TickerName, &dataframe.SeriesInit{Size: 3}, []string{
			"VFINX",
			"PRIDX",
			"VFINX",
		})

		df1 = dataframe.NewDataFrame(timeSeries, tickerSeries)
	})

	Describe("When given a portfolio", func() {
		Context("with target investments", func() {
			It("should have transactions", func() {
				err := p.TargetPortfolio(10000, df1)
				Expect(err).To(BeNil())
				Expect(p.Transactions).To(HaveLen(5))

				// First transaction should be a buy of VFINX
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[0].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[0].Shares).Should(BeNumerically("~", 40.47, 1e-2))

				// Sell transaction should be a Sell of VFINX
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[1].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[1].Shares).Should(BeNumerically("~", 40.47, 1e-2))
				Expect(p.Transactions[1].TotalValue).Should(BeNumerically("~", 9754.36, 1e-2))

				// Buy PRIDX
				Expect(p.Transactions[2].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[2].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[2].Shares).Should(BeNumerically("~", 173.02, 1e-2))

				// Sell PRIDX
				Expect(p.Transactions[3].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[3].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[3].Shares).Should(BeNumerically("~", 173.02, 1e-2))
				Expect(p.Transactions[3].TotalValue).Should(BeNumerically("~", 11126.33, 1e-2))

				// Buy VFINX
				Expect(p.Transactions[4].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[4].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[4].Shares).Should(BeNumerically("~", 37.98, 1e-2))
				Expect(p.Transactions[4].TotalValue).Should(BeNumerically("~", 11126.33, 1e-2))

			})
			It("should have valid performance", func() {
				err := p.TargetPortfolio(10000, df1)
				perf, err := p.Performance(time.Date(2020, time.November, 30, 0, 0, 0, 0, time.UTC))
				Expect(err).To(BeNil())
				Expect(perf.Value).Should(HaveLen(35))
				Expect(perf.Value[0]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1517356800,
					Value: 10000,
				}))

				Expect(perf.Value[24]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1580428800,
					Value: 11126.332313770672,
				}))

				Expect(perf.Value[34]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1606694400,
					Value: 12676.603580175803,
				}))
			})
		})
	})

})
