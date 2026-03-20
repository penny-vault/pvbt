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
type descriptorTestStrategy struct {
	RiskOn  string `pvbt:"riskOn"  desc:"ETFs to invest in"   default:"VOO,SCZ" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
	RiskOff string `pvbt:"riskOff" desc:"Out-of-market asset" default:"TLT"     suggest:"Classic=VUSTX|Modern=TLT"`
}

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
		Schedule:    "@monthend",
		Benchmark:   "SPY",
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
		It("prints human-readable output by default", func() {
			strategy := &descriptorTestStrategy{}
			cmd := newDescribeCmd(strategy)

			outputBuf := new(bytes.Buffer)
			cmd.SetOut(outputBuf)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{})

			Expect(cmd.Execute()).To(Succeed())

			output := outputBuf.String()
			Expect(output).To(ContainSubstring("DescriptorCLITest"))
			Expect(output).To(ContainSubstring("dct"))
			Expect(output).To(ContainSubstring("@monthend"))
			Expect(output).To(ContainSubstring("SPY"))
			Expect(output).To(ContainSubstring("riskOn"))
			Expect(output).To(ContainSubstring("Classic"))
			Expect(output).NotTo(HavePrefix("{"))
		})

		It("prints JSON with --json flag", func() {
			strategy := &descriptorTestStrategy{}
			cmd := newDescribeCmd(strategy)

			outputBuf := new(bytes.Buffer)
			cmd.SetOut(outputBuf)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{"--json"})

			Expect(cmd.Execute()).To(Succeed())

			var parsed engine.StrategyInfo
			Expect(json.Unmarshal(outputBuf.Bytes(), &parsed)).To(Succeed())
			Expect(parsed.ShortCode).To(Equal("dct"))
		})
	})

	Context("with a strategy that does NOT implement Descriptor", func() {
		It("outputs with empty optional fields", func() {
			strategy := &nonDescriptorTestStrategy{}
			cmd := newDescribeCmd(strategy)

			outputBuf := new(bytes.Buffer)
			cmd.SetOut(outputBuf)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{})

			Expect(cmd.Execute()).To(Succeed())

			output := outputBuf.String()
			Expect(output).To(ContainSubstring("NonDescriptorCLITest"))
			Expect(output).NotTo(ContainSubstring("Schedule"))
		})
	})
})
