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
})
