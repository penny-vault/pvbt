package library_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLibrary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Library Suite")
}
