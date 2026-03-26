package etrade

import (
	"net/url"
	"time"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access to unexported response types.
type EtradeOrderDetail = etradeOrderDetail
type EtradeOrderLeg = etradeOrderLeg
type EtradeInstrument = etradeInstrument
type EtradePosition = etradePosition
type EtradeBalanceResponse = etradeBalanceResponse
type EtradeTransaction = etradeTransaction
type EtradePreviewRequest = etradePreviewRequest

// ToBrokerOrder exposes toBrokerOrder for testing.
func ToBrokerOrder(detail etradeOrderDetail) broker.Order {
	return toBrokerOrder(detail)
}

// ToBrokerPosition exposes toBrokerPosition for testing.
func ToBrokerPosition(pos etradePosition) broker.Position {
	return toBrokerPosition(pos)
}

// ToBrokerBalance exposes toBrokerBalance for testing.
func ToBrokerBalance(resp etradeBalanceResponse) broker.Balance {
	return toBrokerBalance(resp)
}

// ToBrokerTransaction exposes toBrokerTransaction for testing.
func ToBrokerTransaction(txn etradeTransaction) broker.Transaction {
	return toBrokerTransaction(txn)
}

// ToEtradeOrderRequest exposes toEtradeOrderRequest for testing.
func ToEtradeOrderRequest(order broker.Order) (etradePreviewRequest, error) {
	return toEtradeOrderRequest(order)
}

// MapPriceType exposes mapPriceType for testing.
func MapPriceType(orderType broker.OrderType) string {
	return mapPriceType(orderType)
}

// MapOrderTerm exposes mapOrderTerm for testing.
func MapOrderTerm(tif broker.TimeInForce) (string, error) {
	return mapOrderTerm(tif)
}

// MapOrderAction exposes mapOrderAction for testing.
func MapOrderAction(side broker.Side) string {
	return mapOrderAction(side)
}

// MapOrderStatus exposes mapOrderStatus for testing.
func MapOrderStatus(status string) broker.OrderStatus {
	return mapOrderStatus(status)
}

// UnmapPriceType exposes unmapPriceType for testing.
func UnmapPriceType(priceType string) broker.OrderType {
	return unmapPriceType(priceType)
}

// UnmapOrderAction exposes unmapOrderAction for testing.
func UnmapOrderAction(action string) broker.Side {
	return unmapOrderAction(action)
}

// FormatDate exposes formatDate for testing.
func FormatDate(tt time.Time) string {
	return formatDate(tt)
}

// ParseDate exposes parseDate for testing.
func ParseDate(ss string) (time.Time, error) {
	return parseDate(ss)
}

// Auth test exports

// BuildAuthHeader exposes buildAuthHeader for testing.
func BuildAuthHeader(method, rawURL, consumerKey, consumerSecret, token, tokenSecret, nonce, timestamp string, extraParams url.Values) string {
	return buildAuthHeader(method, rawURL, consumerKey, consumerSecret, token, tokenSecret, nonce, timestamp, extraParams)
}

// PercentEncode exposes percentEncode for testing.
func PercentEncode(ss string) string {
	return percentEncode(ss)
}

// NewTokenManagerForTest exposes newTokenManager for testing.
func NewTokenManagerForTest(consumerKey, consumerSecret, callbackURL, tokenFile string) *tokenManager {
	return newTokenManager(consumerKey, consumerSecret, callbackURL, tokenFile)
}

// SetAuthBaseURL sets the authBaseURL field on a tokenManager for testing.
func SetAuthBaseURL(tm *tokenManager, baseURL string) {
	tm.authBaseURL = baseURL
}

// TokenManagerForTest is a type alias for tokenManager for test access.
type TokenManagerForTest = tokenManager

// OAuthCredentials is a type alias for oauthCredentials for test access.
type OAuthCredentials = oauthCredentials

// RequestToken exposes requestToken for testing.
func (tm *tokenManager) RequestToken() (string, string, error) {
	return tm.requestToken()
}

// ExchangeAccessToken exposes exchangeAccessToken for testing.
func (tm *tokenManager) ExchangeAccessToken(requestToken, requestSecret, verifier string) error {
	return tm.exchangeAccessToken(requestToken, requestSecret, verifier)
}

// RenewAccessToken exposes renewAccessToken for testing.
func (tm *tokenManager) RenewAccessToken() error {
	return tm.renewAccessToken()
}

// Creds returns the current oauthCredentials for testing.
func (tm *tokenManager) Creds() oauthCredentials {
	return tm.creds
}

// SaveTokens exposes saveTokens for testing.
func SaveTokens(path string, creds *oauthCredentials) error {
	return saveTokens(path, creds)
}

// LoadTokens exposes loadTokens for testing.
func LoadTokens(path string) (*oauthCredentials, error) {
	return loadTokens(path)
}

// ExpandHome exposes expandHome for testing.
func ExpandHome(path string) string {
	return expandHome(path)
}
