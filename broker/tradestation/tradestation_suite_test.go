package tradestation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTradeStation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TradeStation Suite")
}
