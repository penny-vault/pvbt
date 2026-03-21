package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("ExcursionRecord", func() {
	var (
		acme asset.Asset
	)

	BeforeEach(func() {
		acme = asset.Asset{CompositeFigi: "ACME", Ticker: "ACME"}
	})

	Describe("initialization on buy", func() {
		It("creates an excursion record when a position is opened", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			exc := acct.Excursions()
			Expect(exc).To(HaveKey(acme))
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
			Expect(exc[acme].HighPrice).To(Equal(100.0))
			Expect(exc[acme].LowPrice).To(Equal(100.0))
		})
	})

	Describe("position adds", func() {
		It("keeps existing record when adding to a position", func() {
			acct := portfolio.New(portfolio.WithCash(50_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    5,
				Price:  110.0,
				Amount: -550.0,
			})

			exc := acct.Excursions()
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
		})
	})

	Describe("position close", func() {
		It("removes excursion record when position is fully closed", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    10,
				Price:  100.0,
				Amount: -1_000.0,
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			exc := acct.Excursions()
			Expect(exc).NotTo(HaveKey(acme))
		})

		It("keeps excursion record on partial close", func() {
			acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.BuyTransaction,
				Qty:    20,
				Price:  100.0,
				Amount: -2_000.0,
			})

			acct.Record(portfolio.Transaction{
				Date:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
				Asset:  acme,
				Type:   portfolio.SellTransaction,
				Qty:    10,
				Price:  120.0,
				Amount: 1_200.0,
			})

			exc := acct.Excursions()
			Expect(exc).To(HaveKey(acme))
			Expect(exc[acme].EntryPrice).To(Equal(100.0))
		})
	})
})
