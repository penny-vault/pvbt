package fill_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/fill"
)

// stubBase always returns a fixed FillResult.
type stubBase struct {
	price float64
}

func (sb *stubBase) Fill(_ context.Context, order broker.Order, _ *data.DataFrame) (fill.FillResult, error) {
	return fill.FillResult{Price: sb.price, Quantity: order.Qty}, nil
}

// errBase always returns an error.
type errBase struct{}

func (eb *errBase) Fill(_ context.Context, _ broker.Order, _ *data.DataFrame) (fill.FillResult, error) {
	return fill.FillResult{}, fmt.Errorf("base model error")
}

// addAdjuster adds a fixed amount to the price.
type addAdjuster struct {
	amount float64
}

func (aa *addAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, current fill.FillResult) (fill.FillResult, error) {
	current.Price += aa.amount
	return current, nil
}

// errAdjuster always returns an error.
type errAdjuster struct{}

func (ea *errAdjuster) Adjust(_ context.Context, _ broker.Order, _ *data.DataFrame, _ fill.FillResult) (fill.FillResult, error) {
	return fill.FillResult{}, fmt.Errorf("adjuster error")
}

var _ = Describe("Pipeline", func() {
	It("returns the base model result when no adjusters are present", func() {
		pipe := fill.NewPipeline(&stubBase{price: 100.0}, nil)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(100.0))
		Expect(result.Quantity).To(Equal(50.0))
	})

	It("chains adjusters in order", func() {
		pipe := fill.NewPipeline(
			&stubBase{price: 100.0},
			[]fill.Adjuster{&addAdjuster{amount: 5.0}, &addAdjuster{amount: 3.0}},
		)
		order := broker.Order{Qty: 50}
		result, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Price).To(Equal(108.0))
	})

	It("propagates base model errors", func() {
		pipe := fill.NewPipeline(&errBase{}, nil)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("base model error"))
	})

	It("propagates adjuster errors and stops the chain", func() {
		pipe := fill.NewPipeline(
			&stubBase{price: 100.0},
			[]fill.Adjuster{&errAdjuster{}, &addAdjuster{amount: 5.0}},
		)
		order := broker.Order{Qty: 50}
		_, err := pipe.Fill(context.Background(), order, nil)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("adjuster error"))
	})
})
