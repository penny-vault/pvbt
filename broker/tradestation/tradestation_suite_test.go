package tradestation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestTradeStation(t *testing.T) {
	// Send all zerolog output to GinkgoWriter so it only appears on test failure.
	log.Logger = log.Output(GinkgoWriter)

	RegisterFailHandler(Fail)
	RunSpecs(t, "TradeStation Suite")
}
