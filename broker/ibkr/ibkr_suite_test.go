package ibkr_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIBKR(tt *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(tt, "IBKR Suite")
}
