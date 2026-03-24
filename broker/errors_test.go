package broker_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/broker"
)

var _ = Describe("Errors", func() {
	Describe("Sentinel errors", func() {
		It("defines all expected sentinel errors with correct messages", func() {
			Expect(broker.ErrMissingCredentials).To(MatchError("broker: missing credentials"))
			Expect(broker.ErrNotAuthenticated).To(MatchError("broker: not authenticated"))
			Expect(broker.ErrAccountNotFound).To(MatchError("broker: account not found"))
			Expect(broker.ErrAccountNotActive).To(MatchError("broker: account not active"))
			Expect(broker.ErrStreamDisconnected).To(MatchError("broker: stream disconnected"))
			Expect(broker.ErrEmptyOrderGroup).To(MatchError("broker: empty order group"))
			Expect(broker.ErrNoEntryOrder).To(MatchError("broker: no entry order in group"))
			Expect(broker.ErrMultipleEntryOrders).To(MatchError("broker: multiple entry orders in group"))
			Expect(broker.ErrOrderRejected).To(MatchError("broker: order rejected"))
		})
	})

	Describe("HTTPError", func() {
		It("formats the error message with status code and message", func() {
			httpErr := broker.NewHTTPError(404, "not found")
			Expect(httpErr.Error()).To(Equal("broker: HTTP 404: not found"))
		})

		It("stores StatusCode and Message fields", func() {
			httpErr := broker.NewHTTPError(500, "internal server error")
			Expect(httpErr.StatusCode).To(Equal(500))
			Expect(httpErr.Message).To(Equal("internal server error"))
		})
	})

	Describe("IsRetryableError", func() {
		It("returns false for nil", func() {
			Expect(broker.IsRetryableError(nil)).To(BeFalse())
		})

		It("returns true for HTTP 5xx errors", func() {
			Expect(broker.IsRetryableError(broker.NewHTTPError(500, "internal server error"))).To(BeTrue())
			Expect(broker.IsRetryableError(broker.NewHTTPError(502, "bad gateway"))).To(BeTrue())
			Expect(broker.IsRetryableError(broker.NewHTTPError(503, "service unavailable"))).To(BeTrue())
		})

		It("returns true for HTTP 429 errors", func() {
			Expect(broker.IsRetryableError(broker.NewHTTPError(429, "too many requests"))).To(BeTrue())
		})

		It("returns false for HTTP 4xx errors (non-429)", func() {
			Expect(broker.IsRetryableError(broker.NewHTTPError(400, "bad request"))).To(BeFalse())
			Expect(broker.IsRetryableError(broker.NewHTTPError(401, "unauthorized"))).To(BeFalse())
			Expect(broker.IsRetryableError(broker.NewHTTPError(404, "not found"))).To(BeFalse())
			Expect(broker.IsRetryableError(broker.NewHTTPError(422, "unprocessable entity"))).To(BeFalse())
		})

		It("returns true for net.OpError", func() {
			opErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
			Expect(broker.IsRetryableError(opErr)).To(BeTrue())
		})

		It("returns true for net.DNSError", func() {
			dnsErr := &net.DNSError{Name: "api.example.com"}
			Expect(broker.IsRetryableError(dnsErr)).To(BeTrue())
		})

		It("returns true for url.Error wrapping a transient error", func() {
			urlErr := &url.Error{
				Op:  "Get",
				URL: "https://api.example.com",
				Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			}
			Expect(broker.IsRetryableError(urlErr)).To(BeTrue())
		})

		It("returns false for url.Error wrapping a non-transient error", func() {
			urlErr := &url.Error{
				Op:  "Get",
				URL: "https://api.example.com",
				Err: errors.New("some permanent error"),
			}
			Expect(broker.IsRetryableError(urlErr)).To(BeFalse())
		})

		It("returns false for generic errors", func() {
			Expect(broker.IsRetryableError(errors.New("something went wrong"))).To(BeFalse())
		})

		It("returns true for wrapped transient errors", func() {
			netErr := &net.OpError{Op: "read", Err: &net.DNSError{IsTimeout: true}}
			wrapped := fmt.Errorf("request failed: %w", netErr)
			Expect(broker.IsRetryableError(wrapped)).To(BeTrue())
		})

		It("returns false for sentinel errors", func() {
			Expect(broker.IsRetryableError(broker.ErrOrderRejected)).To(BeFalse())
			Expect(broker.IsRetryableError(broker.ErrNotAuthenticated)).To(BeFalse())
		})

		It("returns true for ErrRateLimited", func() {
			Expect(broker.IsRetryableError(broker.ErrRateLimited)).To(BeTrue())
		})

		It("returns true for wrapped ErrRateLimited", func() {
			wrapped := fmt.Errorf("ibkr: %w", broker.ErrRateLimited)
			Expect(broker.IsRetryableError(wrapped)).To(BeTrue())
		})
	})
})
