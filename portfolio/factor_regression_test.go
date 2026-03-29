package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("FactorRegression", func() {
	// Known regression: y = 0.02 + 0.8*x1 + 0.4*x2 + noise
	// Using a simple deterministic dataset where we know the answer.
	//
	// Factor returns (20 observations):
	//   MktRF: [0.01, -0.02, 0.03, -0.01, 0.02, 0.01, -0.03, 0.04, -0.02, 0.01,
	//           0.02, -0.01, 0.03, -0.02, 0.01, 0.02, -0.01, 0.03, -0.03, 0.02]
	//   SMB:   [0.005, -0.01, 0.02, -0.005, 0.01, 0.005, -0.015, 0.025, -0.01, 0.005,
	//           0.01, -0.005, 0.015, -0.01, 0.005, 0.01, -0.005, 0.02, -0.02, 0.01]
	//
	// Portfolio excess returns = 0.02 + 0.8*MktRF + 0.4*SMB (no noise, perfect fit)

	var (
		factorDF   *data.DataFrame
		excessRets []float64
		times      []time.Time
		mktRF      []float64
		smb        []float64
	)

	BeforeEach(func() {
		mktRF = []float64{
			0.01, -0.02, 0.03, -0.01, 0.02, 0.01, -0.03, 0.04, -0.02, 0.01,
			0.02, -0.01, 0.03, -0.02, 0.01, 0.02, -0.01, 0.03, -0.03, 0.02,
		}
		smb = []float64{
			0.005, -0.01, 0.02, -0.005, 0.01, 0.005, -0.015, 0.025, -0.01, 0.005,
			0.01, -0.005, 0.015, -0.01, 0.005, 0.01, -0.005, 0.02, -0.02, 0.01,
		}

		// y = 0.02 + 0.8*MktRF + 0.4*SMB
		excessRets = make([]float64, 20)
		for ii := range 20 {
			excessRets[ii] = 0.02 + 0.8*mktRF[ii] + 0.4*smb[ii]
		}

		times = daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 20)

		var err error
		factorDF, err = data.NewDataFrame(
			times,
			[]asset.Asset{asset.Factor},
			[]data.Metric{data.Metric("MktRF"), data.Metric("SMB")},
			data.Daily,
			[][]float64{mktRF, smb},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("olsRegress", func() {
		It("recovers known alpha and betas with perfect fit", func() {
			result, err := portfolio.OLSRegress(excessRets, factorDF)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Alpha).To(BeNumerically("~", 0.02, 1e-10))
			Expect(result.Betas["MktRF"]).To(BeNumerically("~", 0.8, 1e-10))
			Expect(result.Betas["SMB"]).To(BeNumerically("~", 0.4, 1e-10))
			Expect(result.RSquared).To(BeNumerically("~", 1.0, 1e-10))
		})

		It("returns error when fewer than 12 observations", func() {
			shortTimes := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)
			shortDF, err := data.NewDataFrame(
				shortTimes,
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF")},
				data.Daily,
				[][]float64{mktRF[:5]},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = portfolio.OLSRegress(excessRets[:5], shortDF)
			Expect(err).To(MatchError(portfolio.ErrTooFewObservations))
		})

		It("returns error when factor DataFrame has no metrics", func() {
			emptyDF, err := data.NewDataFrame(
				nil,
				[]asset.Asset{asset.Factor},
				[]data.Metric{},
				data.Daily,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = portfolio.OLSRegress(excessRets, emptyDF)
			Expect(err).To(MatchError(portfolio.ErrNoFactors))
		})
	})

	Describe("FactorAnalysis", func() {
		It("regresses portfolio excess returns against factors", func() {
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
			days := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 21)

			acct := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(spy),
			)

			rfDaily := 0.0001
			equity := make([]float64, 21)
			equity[0] = 10_000.0

			rfEquity := make([]float64, 21)
			rfEquity[0] = 100.0

			benchEquity := make([]float64, 21)
			benchEquity[0] = 100.0

			for ii := 1; ii <= 20; ii++ {
				excessRet := 0.001 + 0.8*mktRF[ii-1] + 0.4*smb[ii-1]
				portRet := excessRet + rfDaily
				equity[ii] = equity[ii-1] * (1 + portRet)
				rfEquity[ii] = rfEquity[ii-1] * (1 + rfDaily)
				benchEquity[ii] = benchEquity[ii-1] * (1 + 0.005)
			}

			acct.Record(portfolio.Transaction{
				Date:   days[0],
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			for ii, dd := range days {
				spyPrice := equity[ii] / 100.0
				cols := [][]float64{
					{spyPrice}, {spyPrice},
					{benchEquity[ii]}, {benchEquity[ii]},
				}

				df, err := data.NewDataFrame(
					[]time.Time{dd},
					[]asset.Asset{spy, bil},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					cols,
				)
				Expect(err).NotTo(HaveOccurred())

				acct.SetRiskFreeValue(rfEquity[ii])
				acct.UpdatePrices(df)
			}

			factorDF, err := data.NewDataFrame(
				days[1:],
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF"), data.Metric("SMB")},
				data.Daily,
				[][]float64{mktRF, smb},
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := acct.FactorAnalysis(factorDF)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Alpha).To(BeNumerically("~", 0.001, 0.01))
			Expect(result.Betas["MktRF"]).To(BeNumerically("~", 0.8, 0.05))
			Expect(result.Betas["SMB"]).To(BeNumerically("~", 0.4, 0.05))
			Expect(result.RSquared).To(BeNumerically(">", 0.95))
		})
	})

	Describe("StepwiseFactorAnalysis", func() {
		It("selects factors that improve AIC and rejects noise", func() {
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			bil := asset.Asset{CompositeFigi: "BIL", Ticker: "BIL"}
			days := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 21)

			acct := portfolio.New(
				portfolio.WithCash(10_000, time.Time{}),
				portfolio.WithBenchmark(spy),
			)

			rfDaily := 0.0001
			equity := make([]float64, 21)
			equity[0] = 10_000.0

			rfEquity := make([]float64, 21)
			rfEquity[0] = 100.0

			benchEquity := make([]float64, 21)
			benchEquity[0] = 100.0

			// Add small idiosyncratic noise uncorrelated with the factors so
			// the model is not a perfect fit (prevents floating-point artifacts
			// in AIC from dominating selection).
			idioNoise := []float64{
				0.0002, -0.0003, 0.0001, -0.0002, 0.0003,
				-0.0001, 0.0002, -0.0003, 0.0001, -0.0002,
				0.0003, -0.0001, 0.0002, -0.0003, 0.0001,
				-0.0002, 0.0003, -0.0001, 0.0002, -0.0003,
			}

			for ii := 1; ii <= 20; ii++ {
				excessRet := 0.001 + 0.8*mktRF[ii-1] + 0.4*smb[ii-1] + idioNoise[ii-1]
				portRet := excessRet + rfDaily
				equity[ii] = equity[ii-1] * (1 + portRet)
				rfEquity[ii] = rfEquity[ii-1] * (1 + rfDaily)
				benchEquity[ii] = benchEquity[ii-1] * (1 + 0.005)
			}

			acct.Record(portfolio.Transaction{
				Date:   days[0],
				Asset:  spy,
				Type:   asset.BuyTransaction,
				Qty:    100,
				Price:  100.0,
				Amount: -10_000.0,
			})

			for ii, dd := range days {
				spyPrice := equity[ii] / 100.0
				cols := [][]float64{
					{spyPrice}, {spyPrice},
					{benchEquity[ii]}, {benchEquity[ii]},
				}

				df, err := data.NewDataFrame(
					[]time.Time{dd},
					[]asset.Asset{spy, bil},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					cols,
				)
				Expect(err).NotTo(HaveOccurred())

				acct.SetRiskFreeValue(rfEquity[ii])
				acct.UpdatePrices(df)
			}

			// Noise factor constructed to be orthogonal to MktRF and SMB
			// (via Gram-Schmidt projection), ensuring zero explanatory power.
			noise := []float64{
				0.004779, 0.002363, -0.003715, 0.001151, -0.002484,
				-0.001272, 0.003574, 0.002335, -0.004899, 0.001148,
				-0.001273, 0.003572, -0.002485, -0.003689, 0.004779,
				0.001147, -0.002480, 0.002336, -0.001247, -0.003694,
			}

			factorDF, err := data.NewDataFrame(
				days[1:],
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF"), data.Metric("SMB"), data.Metric("Noise")},
				data.Daily,
				[][]float64{mktRF, smb, noise},
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := acct.StepwiseFactorAnalysis(factorDF)
			Expect(err).NotTo(HaveOccurred())

			// Best model should include MktRF and SMB but not Noise.
			Expect(result.Best.Betas).To(HaveKey("MktRF"))
			Expect(result.Best.Betas).To(HaveKey("SMB"))
			Expect(result.Best.Betas).NotTo(HaveKey("Noise"))

			// Should have 2 steps (one per selected factor).
			Expect(result.Steps).To(HaveLen(2))

			// First step picks one factor (single-factor model).
			Expect(result.Steps[0].Betas).To(HaveLen(1))

			// Second step adds the other real factor.
			Expect(result.Steps[1].Betas).To(HaveLen(2))
		})
	})

	Describe("edge cases", func() {
		It("FactorAnalysis returns error when portfolio has no price history", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			factorDF, err := data.NewDataFrame(
				daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 20),
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF")},
				data.Daily,
				[][]float64{mktRF},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = acct.FactorAnalysis(factorDF)
			Expect(err).To(HaveOccurred())
		})

		It("FactorAnalysis returns error when date overlap is too short", func() {
			spy := asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
			days := daySeq(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), 5)

			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
			acct.Record(portfolio.Transaction{
				Date:  days[0], Asset: spy, Type: asset.BuyTransaction,
				Qty: 100, Price: 100.0, Amount: -10_000.0,
			})

			for ii, dd := range days {
				price := 100.0 + float64(ii)
				df, err := data.NewDataFrame(
					[]time.Time{dd},
					[]asset.Asset{spy},
					[]data.Metric{data.MetricClose, data.AdjClose},
					data.Daily,
					[][]float64{{price}, {price}},
				)
				Expect(err).NotTo(HaveOccurred())
				acct.SetRiskFreeValue(50.0)
				acct.UpdatePrices(df)
			}

			// Factor data on completely different dates.
			farDays := daySeq(time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC), 20)
			factorDF, err := data.NewDataFrame(
				farDays,
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF")},
				data.Daily,
				[][]float64{mktRF},
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = acct.FactorAnalysis(factorDF)
			Expect(err).To(MatchError(portfolio.ErrTooFewObservations))
		})
	})
})
