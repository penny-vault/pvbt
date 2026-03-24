package tradier_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTradier(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tradier Suite")
}
