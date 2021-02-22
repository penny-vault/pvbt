package portfolio_test

import (
	"fmt"
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
		dfMulti   *dataframe.DataFrame
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

		content, err = ioutil.ReadFile("testdata/riskfree.csv")
		if err != nil {
			panic(err)
		}

		today := time.Now()
		url := fmt.Sprintf("https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=DTB3&cosd=1970-01-01&coed=%d-%02d-%02d&fq=Daily&fam=avg", today.Year(), today.Month(), today.Day())
		httpmock.RegisterResponder("GET", url,
			httpmock.NewBytesResponder(200, content))

		data.InitializeDataManager()

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

		tickerSeriesMulti := dataframe.NewSeriesMixed(portfolio.TickerName,
			&dataframe.SeriesInit{Size: 3},
			map[string]float64{
				"VFINX": 1.0,
			},
			map[string]float64{
				"VFINX": 0.25,
				"PRIDX": 0.5,
				"VUSTX": 0.25,
			},
			map[string]float64{
				"PRIDX": 1.0,
			},
		)

		dfMulti = dataframe.NewDataFrame(timeSeries, tickerSeriesMulti)
	})

	Describe("When given a portfolio", func() {
		Context("with a single holding at a time", func() {
			It("should have transactions", func() {
				err := p.TargetPortfolio(10000, df1)
				Expect(err).To(BeNil())
				Expect(p.Transactions).To(HaveLen(9))

				// First transaction
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.DepositTransaction))
				Expect(p.Transactions[0].TotalValue).Should(BeNumerically("~", 10000.00, 1e-2))

				// marker transaction
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.MarkerTransaction))

				// buy of VFINX
				Expect(p.Transactions[2].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[2].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[2].Shares).Should(BeNumerically("~", 40.47, 1e-2))
				Expect(p.Transactions[2].TotalValue).Should(BeNumerically("~", 10000.00, 1e-2))

				// Sell transaction should be a Sell of VFINX
				Expect(p.Transactions[4].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[4].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[4].Shares).Should(BeNumerically("~", 40.47, 1e-2))
				Expect(p.Transactions[4].TotalValue).Should(BeNumerically("~", 9754.36, 1e-2))

				// Buy PRIDX
				Expect(p.Transactions[5].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[5].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[5].Shares).Should(BeNumerically("~", 173.02, 1e-2))
				Expect(p.Transactions[5].TotalValue).Should(BeNumerically("~", 9754.36, 1e-2))

				// Sell PRIDX
				Expect(p.Transactions[7].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[7].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[7].Shares).Should(BeNumerically("~", 173.02, 1e-2))
				Expect(p.Transactions[7].TotalValue).Should(BeNumerically("~", 11126.33, 1e-2))

				// Buy VFINX
				Expect(p.Transactions[8].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[8].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[8].Shares).Should(BeNumerically("~", 37.98, 1e-2))
				Expect(p.Transactions[8].TotalValue).Should(BeNumerically("~", 11126.33, 1e-2))

			})
			It("should have valid performance", func() {
				err := p.TargetPortfolio(10000, df1)
				perf, err := p.CalculatePerformance(time.Date(2020, time.November, 30, 0, 0, 0, 0, time.UTC))

				Expect(err).To(BeNil())
				Expect(perf.Measurements).Should(HaveLen(35))
				Expect(perf.Measurements[0]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1517356800,
					Value:         10000,
					RiskFreeValue: 10000,
					Holdings:      "VFINX",
					Justification: map[string]interface{}{},
				}))

				Expect(perf.Measurements[24]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1580428800,
					Value:         11126.332313770672,
					RiskFreeValue: 10407.579998915518,
					Holdings:      "VFINX",
					PercentReturn: -0.017122786477389185,
					Justification: map[string]interface{}{},
				}))

				Expect(perf.Measurements[34]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1606694400,
					Value:         12676.603580175803,
					RiskFreeValue: 10426.84579128732,
					Holdings:      "VFINX",
					PercentReturn: 0.10935637663885389,
					Justification: map[string]interface{}{},
				}))
			})
		})
	})

	Describe("When given a target portfolio", func() {
		Context("with multiple holdings at a time", func() {
			It("should have transactions", func() {
				err := p.TargetPortfolio(10000, dfMulti)
				Expect(err).To(BeNil())
				Expect(p.Transactions).To(HaveLen(11))

				// First transaction
				Expect(p.Transactions[0].Date).To(Equal(time.Date(2018, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[0].Kind).To(Equal(portfolio.DepositTransaction))
				Expect(p.Transactions[0].TotalValue).Should(BeNumerically("~", 10000.00, 1e-2))

				// marker transaction
				Expect(p.Transactions[1].Date).To(Equal(time.Date(2018, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[1].Kind).To(Equal(portfolio.MarkerTransaction))

				// buy of VFINX
				Expect(p.Transactions[2].Date).To(Equal(time.Date(2018, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[2].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[2].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[2].Shares).Should(BeNumerically("~", 40.47, 1e-2))
				Expect(p.Transactions[2].TotalValue).Should(BeNumerically("~", 10000.00, 1e-2))

				// Sell transaction should be a Sell of VFINX
				Expect(p.Transactions[4].Date).To(Equal(time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[4].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[4].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[4].Shares).Should(BeNumerically("~", 30.349, 1e-2))
				Expect(p.Transactions[4].TotalValue).Should(BeNumerically("~", 7315.7718, 1e-2))

				// Buy PRIDX
				// Order of purchases within a given day are not guaranteed
				pridxIdx := 5
				vustxIdx := 6
				if p.Transactions[5].Ticker != "PRIDX" {
					vustxIdx = 5
					pridxIdx = 6
				}
				Expect(p.Transactions[pridxIdx].Date).To(Equal(time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[pridxIdx].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[pridxIdx].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[pridxIdx].Shares).Should(BeNumerically("~", 86.5085, 1e-2))
				Expect(p.Transactions[pridxIdx].TotalValue).Should(BeNumerically("~", 4877.1812, 1e-2))

				// Buy VUSTX
				Expect(p.Transactions[vustxIdx].Date).To(Equal(time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[vustxIdx].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[vustxIdx].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[vustxIdx].Shares).Should(BeNumerically("~", 233.01898, 1e-2))
				Expect(p.Transactions[vustxIdx].TotalValue).Should(BeNumerically("~", 2438.5906, 1e-2))

				// marker transaction
				Expect(p.Transactions[7].Date).To(Equal(time.Date(2020, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[7].Kind).To(Equal(portfolio.MarkerTransaction))

				// Sell VUSTX
				// Order of sell transactions on a given day are not ordered -- check the order
				vustxIdx = 8
				vfinxIdx := 9
				if p.Transactions[8].Ticker != "VUSTX" {
					vfinxIdx = 8
					vustxIdx = 9
				}
				Expect(p.Transactions[vustxIdx].Date).To(Equal(time.Date(2020, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[vustxIdx].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[vustxIdx].Ticker).To(Equal("VUSTX"))
				Expect(p.Transactions[vustxIdx].Shares).Should(BeNumerically("~", 233.01898, 1e-2))
				Expect(p.Transactions[vustxIdx].TotalValue).Should(BeNumerically("~", 2971.2879, 1e-2))

				// Sell VFINX
				Expect(p.Transactions[vfinxIdx].Date).To(Equal(time.Date(2020, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[vfinxIdx].Kind).To(Equal(portfolio.SellTransaction))
				Expect(p.Transactions[vfinxIdx].Ticker).To(Equal("VFINX"))
				Expect(p.Transactions[vfinxIdx].Shares).Should(BeNumerically("~", 10.11637, 1e-2))
				Expect(p.Transactions[vfinxIdx].TotalValue).Should(BeNumerically("~", 2963.7546, 1e-2))

				// Buy PRIDX
				Expect(p.Transactions[10].Date).To(Equal(time.Date(2020, 01, 31, 0, 0, 0, 0, time.UTC)))
				Expect(p.Transactions[10].Kind).To(Equal(portfolio.BuyTransaction))
				Expect(p.Transactions[10].Ticker).To(Equal("PRIDX"))
				Expect(p.Transactions[10].Shares).Should(BeNumerically("~", 92.2913, 1e-2))
				Expect(p.Transactions[10].TotalValue).Should(BeNumerically("~", 5935.0426, 1e-2))

			})
			It("should have valid performance", func() {
				err := p.TargetPortfolio(10000, df1)
				perf, err := p.CalculatePerformance(time.Date(2020, time.November, 30, 0, 0, 0, 0, time.UTC))

				Expect(err).To(BeNil())
				Expect(perf.Measurements).Should(HaveLen(35))
				Expect(perf.Measurements[0]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1517356800,
					Value:         10000,
					RiskFreeValue: 10000,
					Holdings:      "VFINX",
					Justification: map[string]interface{}{},
				}))

				Expect(perf.Measurements[24]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1580428800,
					Value:         11126.332313770672,
					RiskFreeValue: 10407.579998915518,
					Holdings:      "VFINX",
					PercentReturn: -0.017122786477389185,
					Justification: map[string]interface{}{},
				}))

				Expect(perf.Measurements[34]).To(Equal(portfolio.PerformanceMeasurement{
					Time:          1606694400,
					Value:         12676.603580175803,
					RiskFreeValue: 10426.84579128732,
					Holdings:      "VFINX",
					PercentReturn: 0.10935637663885389,
					Justification: map[string]interface{}{},
				}))
			})
		})
	})

})
