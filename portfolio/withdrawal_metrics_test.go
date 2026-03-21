package portfolio_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Withdrawal Metrics", func() {
	var spy asset.Asset

	BeforeEach(func() {
		spy = asset.Asset{CompositeFigi: "SPY", Ticker: "SPY"}
	})

	// daySeq returns a date that is dayIndex calendar days after start.
	daySeq := func(start time.Time, dayIndex int) time.Time {
		return start.AddDate(0, 0, dayIndex)
	}

	// buildModerateAccount creates an Account with 400 daily data points
	// (~13 months) at 0.02% daily growth.
	buildModerateAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
		price := 100_000.0
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for idx := range 400 {
			date := daySeq(start, idx)
			if idx > 0 {
				growth := price * 0.0002
				a.Record(portfolio.Transaction{
					Date:   date,
					Type:   portfolio.DepositTransaction,
					Amount: growth,
				})
				price += growth
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450 + float64(idx)}, []float64{448 + float64(idx)})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildFlatAccount creates an Account with 400 daily data points
	// and zero growth -- the equity curve is constant at 100,000.
	buildFlatAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

		for idx := range 400 {
			date := daySeq(start, idx)
			df := buildDF(date, []asset.Asset{spy}, []float64{450}, []float64{448})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildShortAccount creates an Account with 200 daily data points
	// (~7 months), which is less than one year.
	buildShortAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

		for idx := range 200 {
			date := daySeq(start, idx)
			if idx > 0 {
				growth := 10_000.0 * 0.0002
				a.Record(portfolio.Transaction{
					Date:   date,
					Type:   portfolio.DepositTransaction,
					Amount: growth,
				})
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450 + float64(idx)}, []float64{448 + float64(idx)})
			a.UpdatePrices(df)
		}

		return a
	}

	// buildDecliningAccount creates an Account with 1100 daily data points
	// (~3 years) losing 0.5% per day. Over 3 year boundaries, even small
	// withdrawal rates become unsustainable.
	buildDecliningAccount := func() *portfolio.Account {
		a := portfolio.New(portfolio.WithCash(100_000, time.Time{}))
		start := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
		price := 100_000.0

		for idx := range 1100 {
			date := daySeq(start, idx)
			if idx > 0 {
				loss := price * 0.005
				a.Record(portfolio.Transaction{
					Date:   date,
					Type:   portfolio.WithdrawalTransaction,
					Amount: -loss,
				})
				price -= loss
			}
			df := buildDF(date, []asset.Asset{spy}, []float64{450 - float64(idx)*0.5}, []float64{448 - float64(idx)*0.5})
			a.UpdatePrices(df)
		}

		return a
	}

	Describe("SafeWithdrawalRate", func() {
		It("returns 0 for backtests shorter than 1 year", func() {
			a := buildShortAccount()
			val, err := portfolio.SafeWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns a positive rate for moderate growth", func() {
			a := buildModerateAccount()
			val, err := portfolio.SafeWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically(">", 0.0))
		})

		It("returns a lower rate for declining curve than for moderate growth", func() {
			declining := buildDecliningAccount()
			moderate := buildModerateAccount()

			declSWR, err := portfolio.SafeWithdrawalRate.Compute(context.Background(), declining, nil)
			Expect(err).NotTo(HaveOccurred())
			modSWR, err := portfolio.SafeWithdrawalRate.Compute(context.Background(), moderate, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(declSWR).To(BeNumerically("<", modSWR))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			series, err := portfolio.SafeWithdrawalRate.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(series).To(BeNil())
		})
	})

	Describe("PerpetualWithdrawalRate", func() {
		It("returns 0 for backtests shorter than 1 year", func() {
			a := buildShortAccount()
			val, err := portfolio.PerpetualWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns 0 for a flat equity curve", func() {
			// With 0% returns over ~13 months, any withdrawal means the
			// ending balance cannot preserve inflation-adjusted purchasing power.
			a := buildFlatAccount()
			val, err := portfolio.PerpetualWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns 0 for a steeply declining curve", func() {
			a := buildDecliningAccount()
			val, err := portfolio.PerpetualWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			series, err := portfolio.PerpetualWithdrawalRate.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(series).To(BeNil())
		})
	})

	Describe("DynamicWithdrawalRate", func() {
		It("returns 0 for backtests shorter than 1 year", func() {
			a := buildShortAccount()
			val, err := portfolio.DynamicWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(0.0))
		})

		It("returns 0.200 for a flat equity curve (dynamic never depletes)", func() {
			// Dynamic withdrawal adapts downward, so with 0% returns and
			// only 1 year boundary the portfolio never fully depletes.
			a := buildFlatAccount()
			val, err := portfolio.DynamicWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(BeNumerically("~", 0.200, 0.001))
		})

		It("returns nil from ComputeSeries", func() {
			a := buildModerateAccount()
			series, err := portfolio.DynamicWithdrawalRate.ComputeSeries(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(series).To(BeNil())
		})
	})

	Describe("ordering invariant", func() {
		It("PerpetualWithdrawalRate <= SafeWithdrawalRate <= DynamicWithdrawalRate", func() {
			a := buildModerateAccount()
			pwr, err := portfolio.PerpetualWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			swr, err := portfolio.SafeWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())
			dwr, err := portfolio.DynamicWithdrawalRate.Compute(context.Background(), a, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(pwr).To(BeNumerically("<=", swr))
			Expect(swr).To(BeNumerically("<=", dwr))
		})
	})
})
