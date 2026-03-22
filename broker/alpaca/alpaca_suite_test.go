package alpaca_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAlpaca(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alpaca Suite")
}
