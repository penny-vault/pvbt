package tastytrade_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTastytrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tastytrade Suite")
}
