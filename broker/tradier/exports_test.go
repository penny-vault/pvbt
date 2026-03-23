package tradier

import (
	"encoding/json"
	"net/url"

	"github.com/penny-vault/pvbt/broker"
)

// Type aliases for test access.
type TradierOrderResponse = tradierOrderResponse
type TradierPositionResponse = tradierPositionResponse
type TradierBalanceResponse = tradierBalanceResponse
type TradierMarginBalance = tradierMarginBalance
type TradierCashBalance = tradierCashBalance

// UnmarshalFlexible exposes unmarshalFlexible for testing.
func UnmarshalFlexible[T any](raw json.RawMessage) ([]T, error) {
	return unmarshalFlexible[T](raw)
}

// ToTradierOrderParams exposes toTradierOrderParams for testing.
func ToTradierOrderParams(order broker.Order) (url.Values, error) {
	return toTradierOrderParams(order)
}

// ToBrokerOrder exposes toBrokerOrder for testing.
func ToBrokerOrder(resp tradierOrderResponse) broker.Order {
	return toBrokerOrder(resp)
}

// ToBrokerPosition exposes toBrokerPosition for testing.
func ToBrokerPosition(resp tradierPositionResponse) broker.Position {
	return toBrokerPosition(resp)
}

// ToBrokerBalance exposes toBrokerBalance for testing.
func ToBrokerBalance(resp tradierBalanceResponse) broker.Balance {
	return toBrokerBalance(resp)
}

// MapTradierSide exposes mapTradierSide for testing.
func MapTradierSide(side string) broker.Side {
	return mapTradierSide(side)
}
