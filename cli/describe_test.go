package cli

import (
	"bytes"
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

// descriptorTestStrategy implements both Strategy and Descriptor.
type descriptorTestStrategy struct{}

func (s *descriptorTestStrategy) Name() string           { return "DescriptorCLITest" }
func (s *descriptorTestStrategy) Setup(_ *engine.Engine)  {}
func (s *descriptorTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}
func (s *descriptorTestStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode:   "dct",
		Description: "A CLI test strategy",
		Source:      "unit test",
		Version:     "1.2.3",
	}
}

// nonDescriptorTestStrategy implements only Strategy (no Descriptor).
type nonDescriptorTestStrategy struct{}

func (s *nonDescriptorTestStrategy) Name() string           { return "NonDescriptorCLITest" }
func (s *nonDescriptorTestStrategy) Setup(_ *engine.Engine)  {}
func (s *nonDescriptorTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return nil
}

var _ = Describe("newDescribeCmd", func() {
	Context("with a strategy that implements Descriptor", func() {
		It("outputs JSON containing the expected shortcode", func() {
			strategy := &descriptorTestStrategy{}
			cmd := newDescribeCmd(strategy)

			outputBuf := new(bytes.Buffer)
			cmd.SetOut(outputBuf)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{})

			Expect(cmd.Execute()).To(Succeed())

			var parsed engine.StrategyDescription
			Expect(json.Unmarshal(outputBuf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed.ShortCode).To(Equal("dct"))
			Expect(parsed.Description).To(Equal("A CLI test strategy"))
			Expect(parsed.Version).To(Equal("1.2.3"))
		})
	})

	Context("with a strategy that does NOT implement Descriptor", func() {
		It("returns an error", func() {
			strategy := &nonDescriptorTestStrategy{}
			cmd := newDescribeCmd(strategy)

			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{})

			err := cmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not implement Descriptor"))
		})
	})
})
