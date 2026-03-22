package schwab_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSchwab(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Schwab Suite")
}
