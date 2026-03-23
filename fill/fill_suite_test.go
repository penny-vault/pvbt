package fill_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
)

func TestFill(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = log.Output(GinkgoWriter)
	RunSpecs(t, "Fill Suite")
}
