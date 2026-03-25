package broker_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
)

// stubBase always returns a fixed FillResult.
type stubBase struct {
	price float64
}

func (sb *stubBase) Fill(_ context.Context, order broker.Order, _ *data.DataFrame) (broker.FillResult, error) {
	return broker.FillResult{Price: sb.price, Quantity: order.Qty}, nil
}

// errBase always returns an error.
type errBase struct{}

func (eb *errBase) Fill(_ context.Context, _ broker.Order, _ *data.DataFrame) (broker.FillResult, error) {
	return broker.FillResult{}, fmt.Errorf("base model error")
}

// addAdjuster adds a fixed amount to the price.
type addAdjuster struct {
	amount float64
}

func (aa *addAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, current broker.FillResult) (broker.FillResult, error) {
	current.Price += aa.amount
	return current, nil
}

// errAdjuster always returns an error.
type errAdjuster struct{}

func (ea *errAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, _ broker.FillResult) (broker.FillResult, error) {
	return broker.FillResult{}, fmt.Errorf("adjuster error")
}

var _ = Describe("Pipeline", func() {
	It("returns the base model result when no adjusters are present", func() {
		pipe := broker.NewPipeline(&stubBase{price: 100.0}, nil)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(100.0))
		Expect(result.Quantity).To(Equal(50.0))
	})

	It("chains adjusters in order", func() {
		pipe := broker.NewPipeline(
			&stubBase{price: 100.0},
			[]broker.Adjuster{&addAdjuster{amount: 5.0}, &addAdjuster{amount: 3.0}},
		)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(108.0))
	})

	It("propagates base model errors", func() {
		pipe := broker.NewPipeline(&errBase{}, nil)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("base model error"))
	})

	It("propagates adjuster errors and stops the chain", func() {
		pipe := broker.NewPipeline(
			&stubBase{price: 100.0},
			[]broker.Adjuster{&errAdjuster{}, &addAdjuster{amount: 5.0}},
		)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("adjuster error"))
	})
})
