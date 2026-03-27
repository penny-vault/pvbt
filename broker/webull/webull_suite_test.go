package webull_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestWebull(t *testing.T) {
	RegisterFailHandler(Fail)
	log.Logger = zerolog.New(GinkgoWriter)
	RunSpecs(t, "Webull Suite")
}
