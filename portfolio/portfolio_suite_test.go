package portfolio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPortfolio(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Portfolio Suite")
}
