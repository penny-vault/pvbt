package portfolio_test

import (
	"testing"

	"github.com/jarcoal/httpmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
	//	log.SetLevel(log.DebugLevel)

	// block all HTTP requests
	httpmock.Activate()
})

var _ = BeforeEach(func() {
	// remove any mocks
	httpmock.Reset()
})

var _ = AfterSuite(func() {
	httpmock.DeactivateAndReset()
})

func TestPortfolio(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Portfolio Suite")
}
