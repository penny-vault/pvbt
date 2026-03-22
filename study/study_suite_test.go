package study_test

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

func TestStudy(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Study Suite")
}
