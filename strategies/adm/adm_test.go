package adm_test

import (
	"fmt"
	"io/ioutil"
	"main/data"
	"main/portfolio"
	"main/strategies/adm"
	"time"

	"github.com/goccy/go-json"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adm", func() {
	var (
		strat   *adm.AcceleratingDualMomentum
		manager data.Manager
	)

	BeforeEach(func() {
		jsonParams := `{"inTickers": ["VFINX", "PRIDX"], "outTicker": "VUSTX"}`
		params := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(jsonParams), &params); err != nil {
			panic(err)
		}

		tmp, _ := adm.New(params)
		strat = tmp.(*adm.AcceleratingDualMomentum)

		manager = data.NewManager(map[string]string{
			"tiingo": "TEST",
		})

		content, err := ioutil.ReadFile("../testdata/TB3MS.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=TB3MS&cosd=1979-07-01&coed=2021-01-01&fq=AdjustedClose&fam=avg",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VUSTX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1979-07-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VUSTX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VUSTX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VFINX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1979-07-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/VFINX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/VFINX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/PRIDX.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1979-07-01&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/PRIDX_2.csv")
		if err != nil {
			panic(err)
		}

		httpmock.RegisterResponder("GET", "https://api.tiingo.com/tiingo/daily/PRIDX/prices?startDate=1989-07-31&endDate=2021-01-01&format=csv&resampleFreq=Monthly&token=TEST",
			httpmock.NewBytesResponder(200, content))

		content, err = ioutil.ReadFile("../testdata/riskfree.csv")
		if err != nil {
			panic(err)
		}

		today := time.Now()
		url := fmt.Sprintf("https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=DTB3&cosd=1970-01-01&coed=%d-%02d-%02d&fq=Daily&fam=avg", today.Year(), today.Month(), today.Day())
		httpmock.RegisterResponder("GET", url,
			httpmock.NewBytesResponder(200, content))

		data.InitializeDataManager()
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			It("should be invested in PRIDX", func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
				p, err := strat.Compute(&manager)
				Expect(err).To(BeNil())

				perf, err := p.CalculatePerformance(manager.End)
				Expect(err).To(BeNil())
				Expect(strat.CurrentSymbol).To(Equal("PRIDX"))

				var begin int64 = 617846400
				Expect(perf.PeriodStart).To(Equal(begin))

				var end int64 = 1609459200
				Expect(perf.PeriodEnd).To(Equal(end))
				Expect(perf.Measurements).Should(HaveLen(379))
				Expect(perf.Measurements[0]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  617846400,
					Value: 10000,
					Holdings: []portfolio.ReportableHolding{
						{
							Ticker:           "VFINX",
							Shares:           589.5892770347044,
							PercentPortfolio: 1,
							Value:            10000,
						},
					},
					RiskFreeValue: 10000,
					PercentReturn: 0,
					Justification: map[string]interface{}{
						"VFINX Score": 11.039635491240361,
						"PRIDX Score": 7.228182302084328,
					},
				}))
				Expect(perf.Measurements[100]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  880675200,
					Value: 42408.60298101431,
					Holdings: []portfolio.ReportableHolding{
						{
							Ticker:           "VFINX",
							Shares:           726.673315772532,
							PercentPortfolio: 1,
							Value:            42408.60298101431,
						},
					},
					RiskFreeValue: 15173.981586783602,
					PercentReturn: 0.045985060690943325,
					Justification: map[string]interface{}{
						"VFINX Score": 6.840593309526736,
						"PRIDX Score": -7.943715719726785,
					},
				}))
				Expect(perf.Measurements[200]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1143763200,
					Value: 343579.75074944284,
					Holdings: []portfolio.ReportableHolding{
						{
							Ticker:           "PRIDX",
							Shares:           13581.990073636443,
							PercentPortfolio: 1,
							Value:            343579.75074944284,
						},
					},
					RiskFreeValue: 19938.252280594555,
					PercentReturn: 0.06929347826087029,
					Justification: map[string]interface{}{
						"VFINX Score": 2.734696874619626,
						"PRIDX Score": 13.388751105525918,
					},
				}))
				Expect(perf.Measurements[300]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1406764800,
					Value: 1.1502482646161707e+06,
					Holdings: []portfolio.ReportableHolding{
						{
							Ticker:           "VFINX",
							Shares:           7287.807720408271,
							PercentPortfolio: 1,
							Value:            1.1502482646161707e+06,
						},
					},
					RiskFreeValue: 21966.917804088513,
					PercentReturn: -0.01388044019244239,
					Justification: map[string]interface{}{
						"VFINX Score": 3.6407964883074544,
						"PRIDX Score": 0.9912254124299698,
					},
				}))

				Expect(perf.Measurements[378]).To(Equal(portfolio.PerformanceMeasurement{
					Time:  1611878400,
					Value: 3.279045827906852e+06,
					Holdings: []portfolio.ReportableHolding{
						{
							Ticker:           "PRIDX",
							Shares:           35145.1857224743,
							PercentPortfolio: 1,
							Value:            3.279045827906852e+06,
						},
					},
					RiskFreeValue: 23244.597277008164,
					PercentReturn: 0.027872645147074993,
					Justification: map[string]interface{}{
						"VFINX Score": 11.242040896256247,
						"PRIDX Score": 18.048957055687936,
					},
				}))
			})
		})
	})
})
