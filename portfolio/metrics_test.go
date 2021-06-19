package portfolio_test

import (
	"io/ioutil"

	"github.com/goccy/go-json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"main/portfolio"
)

var _ = Describe("Metrics", func() {
	var (
		perf  portfolio.Performance
		perf2 portfolio.Performance
	)

	BeforeEach(func() {
		jsonBlob, err := ioutil.ReadFile("testdata/returns.json")
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(jsonBlob, &perf)
		if err != nil {
			panic(err)
		}

		jsonBlob, err = ioutil.ReadFile("testdata/adm-vfinx_pridx_vustx.json")
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(jsonBlob, &perf2)
		if err != nil {
			panic(err)
		}

	})

	Describe("When given a performance struct", func() {
		Context("with portfolio returns", func() {
			It("should have draw downs", func() {
				drawDowns, _, _ := perf.DrawDowns()

				Expect(drawDowns).NotTo(BeNil())
				Expect(drawDowns).To(HaveLen(10))

				Expect(drawDowns[0].Begin).Should(BeEquivalentTo(649494000))
				Expect(drawDowns[0].End).Should(BeEquivalentTo(652172400))
				Expect(drawDowns[0].Recovery).Should(BeEquivalentTo(754729200))
				Expect(drawDowns[0].LossPercent).Should(BeNumerically("~", -0.2694, 1e-2))

				Expect(drawDowns[1].Begin).Should(BeEquivalentTo(951894000))
				Expect(drawDowns[1].End).Should(BeEquivalentTo(970383600))
				Expect(drawDowns[1].Recovery).Should(BeEquivalentTo(1064991600))
				Expect(drawDowns[1].LossPercent).Should(BeNumerically("~", -0.2464, 1e-2))

				Expect(drawDowns[2].Begin).Should(BeEquivalentTo(1577862000))
				Expect(drawDowns[2].End).Should(BeEquivalentTo(1583046000))
				Expect(drawDowns[2].Recovery).Should(BeEquivalentTo(1604214000))
				Expect(drawDowns[2].LossPercent).Should(BeNumerically("~", -0.1963, 1e-2))

				Expect(drawDowns[3].Begin).Should(BeEquivalentTo(1193900400))
				Expect(drawDowns[3].End).Should(BeEquivalentTo(1199170800))
				Expect(drawDowns[3].Recovery).Should(BeEquivalentTo(1267426800))
				Expect(drawDowns[3].LossPercent).Should(BeNumerically("~", -0.16804, 1e-2))

				Expect(drawDowns[4].Begin).Should(BeEquivalentTo(1304233200))
				Expect(drawDowns[4].End).Should(BeEquivalentTo(1320130800))
				Expect(drawDowns[4].Recovery).Should(BeEquivalentTo(1362121200))
				Expect(drawDowns[4].LossPercent).Should(BeNumerically("~", -0.1666, 1e-2))

				Expect(drawDowns[5].Begin).Should(BeEquivalentTo(899276400))
				Expect(drawDowns[5].End).Should(BeEquivalentTo(901954800))
				Expect(drawDowns[5].Recovery).Should(BeEquivalentTo(909903600))
				Expect(drawDowns[5].LossPercent).Should(BeNumerically("~", -0.1537, 1e-2))

				Expect(drawDowns[6].Begin).Should(BeEquivalentTo(1272697200))
				Expect(drawDowns[6].End).Should(BeEquivalentTo(1275375600))
				Expect(drawDowns[6].Recovery).Should(BeEquivalentTo(1283324400))
				Expect(drawDowns[6].LossPercent).Should(BeNumerically("~", -0.1283, 1e-2))

				Expect(drawDowns[7].Begin).Should(BeEquivalentTo(1146466800))
				Expect(drawDowns[7].End).Should(BeEquivalentTo(1151737200))
				Expect(drawDowns[7].Recovery).Should(BeEquivalentTo(1162364400))
				Expect(drawDowns[7].LossPercent).Should(BeNumerically("~", -0.0995, 1e-2))

				Expect(drawDowns[8].Begin).Should(BeEquivalentTo(1433142000))
				Expect(drawDowns[8].End).Should(BeEquivalentTo(1441090800))
				Expect(drawDowns[8].Recovery).Should(BeEquivalentTo(1480575600))
				Expect(drawDowns[8].LossPercent).Should(BeNumerically("~", -0.0908, 1e-2))

				Expect(drawDowns[9].Begin).Should(BeEquivalentTo(762505200))
				Expect(drawDowns[9].End).Should(BeEquivalentTo(783673200))
				Expect(drawDowns[9].Recovery).Should(BeEquivalentTo(794041200))
				Expect(drawDowns[9].LossPercent).Should(BeNumerically("~", -0.0685, 1e-2))
			})

			It("should have a Net Profit", func() {
				Expect(perf.NetProfit()).Should(BeNumerically("~", 650998.5096, 1e-2))
			})

			It("should have a net profit percent", func() {
				Expect(perf.NetProfitPercent()).Should(BeNumerically("~", 65.0999, 1e-2))
			})

			It("should have a 1-yr CAGR", func() {
				Expect(perf.PeriodCagr(1)).Should(BeNumerically("~", 0.1336, 1e-3))
			})

			It("should have a 3-yr CAGR", func() {
				Expect(perf.PeriodCagr(3)).Should(BeNumerically("~", 0.0846, 1e-2))
			})

			It("should have a 5-yr CAGR", func() {
				Expect(perf.PeriodCagr(5)).Should(BeNumerically("~", 0.1393, 1e-2))
			})

			It("should have a standard deviation", func() {
				Expect(perf.StdDev()).Should(BeNumerically("~", .1483, 1e-3))
			})

			It("should have an ulcer index", func() {
				u := perf.UlcerIndex(14)
				Expect(u).To(HaveLen(len(perf.Measurements) - 14))
				Expect(u[0]).Should(BeNumerically("~", 20.2805, 1e-3))
				Expect(u[50]).Should(BeNumerically("~", 10.5812, 1e-3))
				Expect(u[len(u)-1]).Should(BeNumerically("~", 19.1695, 1e-3))
			})

			It("should have an avg ulcer index", func() {
				u := perf.AvgUlcerIndex(14)
				Expect(u).Should(BeNumerically("~", 12.1664, 1e-3))
			})

			It("should have a sharpe ratio", func() {
				Expect(perf2.SharpeRatio()).Should(BeNumerically("~", 1.1199, 1e-3))
			})

			It("should have a sortino ratio", func() {
				Expect(perf2.SortinoRatio()).Should(BeNumerically("~", 2.066, 1e-3))
			})
		})
	})

})
