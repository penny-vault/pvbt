package ibkr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/ibkr"
)

var _ = Describe("Errors", func() {
	It("defines ErrConidNotFound with a descriptive message", func() {
		Expect(ibkr.ErrConidNotFound).To(MatchError("ibkr: contract ID not found for symbol"))
	})
})
