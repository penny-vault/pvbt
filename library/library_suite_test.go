package library_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestLibrary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Library Suite")
}

var _ = BeforeSuite(func() {
	log.Logger = log.Output(GinkgoWriter)
})
