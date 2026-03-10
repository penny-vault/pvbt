package portfolio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestPortfolio(t *testing.T) {
	RegisterFailHandler(Fail)

	// Send all zerolog output to GinkgoWriter so it only appears on test failure.
	log.Logger = log.Output(GinkgoWriter)

	RunSpecs(t, "Portfolio Suite")
}
