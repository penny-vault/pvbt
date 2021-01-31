package strategies_test

import (
	"encoding/json"
	"io/ioutil"
	"main/data"
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
	})

	Describe("Compute momentum scores", func() {
		Context("with full stock history", func() {
			It("should be invested in PRIDX", func() {
				manager.Begin = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
				manager.End = time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
				_, err := adm.Compute(&manager)
				Expect(err).To(BeNil())
				Expect(adm.CurrentSymbol).To(Equal("PRIDX"))
			})
		})
	})
})
