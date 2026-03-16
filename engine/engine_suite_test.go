package engine_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/rs/zerolog/log"
)

var _ = BeforeSuite(func() {
	// Initialize an empty holiday calendar so tradecron does not panic.
	tradecron.SetMarketHolidays(nil)
})

func TestEngine(t *testing.T) {
	RegisterFailHandler(Fail)

	// Send all zerolog output to GinkgoWriter so it only appears on test failure.
	log.Logger = log.Output(GinkgoWriter)

	RunSpecs(t, "Engine Suite")
}
