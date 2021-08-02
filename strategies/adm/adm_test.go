package adm_test

import (
	"fmt"
	"io/ioutil"
	"main/common"
	"main/data"
	"main/strategies/adm"
	"time"

	"github.com/goccy/go-json"
	"github.com/rocketlaunchr/dataframe-go"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adm", func() {
	var (
		strat   *adm.AcceleratingDualMomentum
		manager data.Manager
		tz      *time.Location
		target  *dataframe.DataFrame
		err     error
	)

	BeforeEach(func() {
		tz, _ = time.LoadLocation("America/New_York") // New York is the reference time

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
		httpmock.RegisterResponder("GET", "https://fred.stlouisfed.org/graph/fredgraph.csv?mode=fred&id=TB3MS&cosd=1979-07-01&coed=2021-01-01&fq=Close&fam=avg",
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
			BeforeEach(func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, tz)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, tz)
				target, err = strat.Compute(&manager)
			})

			It("should not error", func() {
				Expect(err).To(BeNil())
			})

			It("should have length", func() {
				Expect(target.NRows()).To(Equal(379))
			})

			It("should begin on", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				Expect(val[data.DateIdx].(time.Time)).To(Equal(time.Date(1989, time.July, 31, 16, 0, 0, 0, tz)))
			})

			It("should end on", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				Expect(val[data.DateIdx].(time.Time)).To(Equal(time.Date(2021, time.January, 29, 16, 0, 0, 0, tz)))
			})

			It("should be invested in VFINX to start", func() {
				val := target.Row(0, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})

			It("should be invested in PRIDX to end", func() {
				n := target.NRows()
				val := target.Row(n-1, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("PRIDX"))
			})

			It("should be invested in PRIDX on 1997-11-28", func() {
				val := target.Row(100, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})

			It("should be invested in PRIDX on 2006-03-31", func() {
				val := target.Row(200, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("PRIDX"))
			})

			It("should be invested in VFINX on 2014-07-31", func() {
				val := target.Row(300, true, dataframe.SeriesName)
				Expect(val[common.TickerName].(string)).To(Equal("VFINX"))
			})
		})
	})
})
