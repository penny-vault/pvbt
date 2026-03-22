package schwab_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/schwab"
)

var _ = Describe("Errors", func() {
	Describe("Sentinel errors", func() {
		It("defines ErrTokenExpired with a descriptive message", func() {
			Expect(schwab.ErrTokenExpired).To(MatchError("schwab: refresh token expired, re-authorization required"))
		})

		It("defines ErrAuthorizationRequired with a descriptive message", func() {
			Expect(schwab.ErrAuthorizationRequired).To(MatchError("schwab: user must authorize via browser"))
		})

		It("defines ErrAccountNotFound with a descriptive message", func() {
			Expect(schwab.ErrAccountNotFound).To(MatchError("schwab: no accounts found"))
		})

		It("defines ErrLoginDenied with a descriptive message", func() {
			Expect(schwab.ErrLoginDenied).To(MatchError("schwab: streamer LOGIN denied"))
		})

		It("defines ErrStreamDisconnected with a descriptive message", func() {
			Expect(schwab.ErrStreamDisconnected).To(MatchError("schwab: WebSocket disconnected"))
		})
	})
})
