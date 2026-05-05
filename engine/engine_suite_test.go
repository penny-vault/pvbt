package engine_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/tradecron"
	"github.com/rs/zerolog/log"
)

// init runs before any test (including Example tests, which Ginkgo's
// BeforeSuite does not cover) so the tradecron market calendar is populated
// before any code path touches it.
func init() {
	tradecron.SetMarketHolidays(nil)
}

func TestEngine(t *testing.T) {
	RegisterFailHandler(Fail)

	// Send all zerolog output to GinkgoWriter so it only appears on test failure.
	log.Logger = log.Output(GinkgoWriter)

	RunSpecs(t, "Engine Suite")
}
