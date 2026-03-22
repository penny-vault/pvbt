package tastytrade_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker/tastytrade"
)

var _ = Describe("Errors", func() {
	Describe("Sentinel errors", func() {
		It("defines all expected sentinel errors", func() {
			Expect(tastytrade.ErrNotAuthenticated).To(MatchError("broker: not authenticated"))
			Expect(tastytrade.ErrMissingCredentials).To(MatchError("broker: missing credentials"))
			Expect(tastytrade.ErrAccountNotFound).To(MatchError("broker: account not found"))
			Expect(tastytrade.ErrOrderRejected).To(MatchError("broker: order rejected"))
			Expect(tastytrade.ErrStreamDisconnected).To(MatchError("broker: stream disconnected"))
			Expect(tastytrade.ErrEmptyOrderGroup).To(MatchError("broker: empty order group"))
			Expect(tastytrade.ErrNoEntryOrder).To(MatchError("broker: no entry order in group"))
			Expect(tastytrade.ErrMultipleEntryOrders).To(MatchError("broker: multiple entry orders in group"))
		})
	})

	Describe("IsTransient", Label("translation"), func() {
		It("returns true for network timeout errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			Expect(tastytrade.IsTransient(netErr)).To(BeTrue())
		})

		It("returns true for DNS errors", func() {
			dnsErr := &net.DNSError{Name: "api.tastyworks.com"}
			Expect(tastytrade.IsTransient(dnsErr)).To(BeTrue())
		})

		It("returns true for connection refused errors", func() {
			connErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
			Expect(tastytrade.IsTransient(connErr)).To(BeTrue())
		})

		It("returns true for URL errors wrapping net errors", func() {
			urlErr := &url.Error{Op: "Get", URL: "https://api.tastyworks.com", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
			Expect(tastytrade.IsTransient(urlErr)).To(BeTrue())
		})

		It("returns true for HTTP 500 errors", func() {
			httpErr := tastytrade.NewHTTPError(500, "internal server error")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns true for HTTP 502 errors", func() {
			httpErr := tastytrade.NewHTTPError(502, "bad gateway")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns true for HTTP 503 errors", func() {
			httpErr := tastytrade.NewHTTPError(503, "service unavailable")
			Expect(tastytrade.IsTransient(httpErr)).To(BeTrue())
		})

		It("returns false for HTTP 400 errors", func() {
			httpErr := tastytrade.NewHTTPError(400, "bad request")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for HTTP 401 errors", func() {
			httpErr := tastytrade.NewHTTPError(401, "unauthorized")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for HTTP 422 errors", func() {
			httpErr := tastytrade.NewHTTPError(422, "unprocessable entity")
			Expect(tastytrade.IsTransient(httpErr)).To(BeFalse())
		})

		It("returns false for order rejected errors", func() {
			Expect(tastytrade.IsTransient(tastytrade.ErrOrderRejected)).To(BeFalse())
		})

		It("returns false for auth errors", func() {
			Expect(tastytrade.IsTransient(tastytrade.ErrNotAuthenticated)).To(BeFalse())
		})

		It("returns false for generic errors", func() {
			Expect(tastytrade.IsTransient(errors.New("something went wrong"))).To(BeFalse())
		})

		It("returns true for wrapped transient errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			wrapped := fmt.Errorf("request failed: %w", netErr)
			Expect(tastytrade.IsTransient(wrapped)).To(BeTrue())
		})
	})
})
