package portfolio_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("Annotations", func() {
	It("records and returns annotations in order", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		ts := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		acct.Annotate(ts, "SPY/Momentum", "0.87")
		acct.Annotate(ts, "bond_fraction", "0.3")

		annotations := acct.Annotations()
		Expect(annotations).To(HaveLen(2))
		Expect(annotations[0].Timestamp).To(Equal(ts))
		Expect(annotations[0].Key).To(Equal("SPY/Momentum"))
		Expect(annotations[0].Value).To(Equal("0.87"))
		Expect(annotations[1].Key).To(Equal("bond_fraction"))
		Expect(annotations[1].Value).To(Equal("0.3"))
	})

	It("returns nil when no annotations have been recorded", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))
		Expect(acct.Annotations()).To(BeNil())
	})

	It("overwrites value when same timestamp and key are annotated again", func() {
		acct := portfolio.New(portfolio.WithCash(10_000, time.Time{}))

		ts := time.Date(2024, 1, 15, 16, 0, 0, 0, time.UTC)
		acct.Annotate(ts, "SPY/Momentum", "0.87")
		acct.Annotate(ts, "bond_fraction", "0.3")
		acct.Annotate(ts, "SPY/Momentum", "0.92") // overwrite

		annotations := acct.Annotations()
		Expect(annotations).To(HaveLen(2))
		Expect(annotations[0].Key).To(Equal("SPY/Momentum"))
		Expect(annotations[0].Value).To(Equal("0.92"))
		Expect(annotations[1].Key).To(Equal("bond_fraction"))
		Expect(annotations[1].Value).To(Equal("0.3"))
	})
})
