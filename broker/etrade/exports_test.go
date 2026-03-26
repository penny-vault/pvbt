package etrade

import (
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
