package dfextras_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDfextras(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dfextras Suite")
}
