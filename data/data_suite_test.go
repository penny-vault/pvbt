package data_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestData(t *testing.T) {
	log.Logger = log.Output(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Suite")
}
