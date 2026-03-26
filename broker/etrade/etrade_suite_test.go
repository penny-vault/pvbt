package etrade_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestEtrade(t *testing.T) {
	t.Parallel()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etrade Suite")
}

var _ = BeforeSuite(func() {
	log.Logger = zerolog.New(GinkgoWriter)
})
