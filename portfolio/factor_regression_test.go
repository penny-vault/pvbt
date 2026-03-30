package portfolio_test

import (
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/portfolio"
)

// buildFactorAccount constructs an Account whose portfolio excess returns
// are a linear combination of mktRF and smb:
//
//	excess_ret[i] = alpha + betaMkt*mktRF[i] + betaSMB*smb[i] + noise[i]
//
// Returns the account and 21 weekday timestamps (day 0 + 20 return days).
func buildFactorAccount(
	alpha, betaMkt, betaSMB float64,
	mktRF, smb, noise []float64,
) (*portfolio.Account, []time.Time) {
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
		excessRet := alpha + betaMkt*mktRF[ii-1] + betaSMB*smb[ii-1]
		if noise != nil {
			excessRet += noise[ii-1]
		}

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

	return acct, days
}

var _ = Describe("FactorRegression", func() {
	var (
		mktRF []float64
		smb   []float64
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
	})

	Describe("FactorAnalysis", func() {
		It("recovers known alpha and betas", func() {
			acct, days := buildFactorAccount(0.001, 0.8, 0.4, mktRF, smb, nil)

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

		It("works with a single factor", func() {
			acct, days := buildFactorAccount(0.002, 0.6, 0.0, mktRF, smb, nil)

			factorDF, err := data.NewDataFrame(
				days[1:],
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF")},
				data.Daily,
				[][]float64{mktRF},
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := acct.FactorAnalysis(factorDF)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Betas["MktRF"]).To(BeNumerically("~", 0.6, 0.05))
			Expect(result.RSquared).To(BeNumerically(">", 0.90))
		})

		It("handles NaN in excess returns without corruption", func() {
			acct, days := buildFactorAccount(0.001, 0.8, 0.4, mktRF, smb, nil)

			// Factor data includes days[0] (same as portfolio first observation),
			// so the NaN from Pct() at position 0 overlaps with factor data.
			// The method should skip it and still produce valid (non-NaN) results.
			factorDF, err := data.NewDataFrame(
				days[:20],
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF"), data.Metric("SMB")},
				data.Daily,
				[][]float64{mktRF, smb},
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := acct.FactorAnalysis(factorDF)
			Expect(err).NotTo(HaveOccurred())

			Expect(math.IsNaN(result.Alpha)).To(BeFalse())
			Expect(math.IsNaN(result.RSquared)).To(BeFalse())
			for _, beta := range result.Betas {
				Expect(math.IsNaN(beta)).To(BeFalse())
			}
		})
	})

	Describe("StepwiseFactorAnalysis", func() {
		It("selects factors that improve AIC and rejects noise", func() {
			idioNoise := []float64{
				0.0002, -0.0003, 0.0001, -0.0002, 0.0003,
				-0.0001, 0.0002, -0.0003, 0.0001, -0.0002,
				0.0003, -0.0001, 0.0002, -0.0003, 0.0001,
				-0.0002, 0.0003, -0.0001, 0.0002, -0.0003,
			}

			acct, days := buildFactorAccount(0.001, 0.8, 0.4, mktRF, smb, idioNoise)

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

			Expect(result.Best.Betas).To(HaveKey("MktRF"))
			Expect(result.Best.Betas).To(HaveKey("SMB"))
			Expect(result.Best.Betas).NotTo(HaveKey("Noise"))

			Expect(result.Steps).To(HaveLen(2))
			Expect(result.Steps[0].Betas).To(HaveLen(1))
			Expect(result.Steps[1].Betas).To(HaveLen(2))
		})

		It("stops after one factor when second does not improve AIC", func() {
			// Portfolio driven by MktRF only; noise factor is uncorrelated.
			// Stepwise should select MktRF and stop at 1 step.
			idioNoise := []float64{
				0.0002, -0.0003, 0.0001, -0.0002, 0.0003,
				-0.0001, 0.0002, -0.0003, 0.0001, -0.0002,
				0.0003, -0.0001, 0.0002, -0.0003, 0.0001,
				-0.0002, 0.0003, -0.0001, 0.0002, -0.0003,
			}
			acct, days := buildFactorAccount(0.001, 0.8, 0.0, mktRF, smb, idioNoise)

			noise := []float64{
				0.004779, 0.002363, -0.003715, 0.001151, -0.002484,
				-0.001272, 0.003574, 0.002335, -0.004899, 0.001148,
				-0.001273, 0.003572, -0.002485, -0.003689, 0.004779,
				0.001147, -0.002480, 0.002336, -0.001247, -0.003694,
			}

			factorDF, err := data.NewDataFrame(
				days[1:],
				[]asset.Asset{asset.Factor},
				[]data.Metric{data.Metric("MktRF"), data.Metric("Noise")},
				data.Daily,
				[][]float64{mktRF, noise},
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := acct.StepwiseFactorAnalysis(factorDF)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Steps).To(HaveLen(1))
			Expect(result.Best.Betas).To(HaveKey("MktRF"))
			Expect(result.Best.Betas).NotTo(HaveKey("Noise"))
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
				Date: days[0], Asset: spy, Type: asset.BuyTransaction,
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

		It("StepwiseFactorAnalysis returns error with no factors", func() {
			acct, _ := buildFactorAccount(0.001, 0.8, 0.4, mktRF, smb, nil)

			emptyDF, err := data.NewDataFrame(
				nil,
				[]asset.Asset{asset.Factor},
				[]data.Metric{},
				data.Daily,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = acct.StepwiseFactorAnalysis(emptyDF)
			Expect(err).To(MatchError(portfolio.ErrNoFactors))
		})
	})
})
