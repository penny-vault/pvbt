package ibkr

import "github.com/penny-vault/pvbt/broker"

// Type aliases for test access.
type HTTPError = broker.HTTPError
type IBOrderRequest = ibOrderRequest
type IBOrderResponse = ibOrderResponse
type IBPositionEntry = ibPositionEntry
type IBAccountSummary = ibAccountSummary
type SummaryValue = summaryValue
type IBSecdefResult = ibSecdefResult
type IBOrderReply = ibOrderReply
type IBTradeEntry = ibTradeEntry

func ToIBOrder(order broker.Order, conid int64) (ibOrderRequest, error) {
	return toIBOrder(order, conid)
}

func ToBrokerOrder(resp ibOrderResponse) broker.Order {
	return toBrokerOrder(resp)
}

func ToBrokerPosition(pos ibPositionEntry) broker.Position {
	return toBrokerPosition(pos)
}

func ToBrokerBalance(summary ibAccountSummary) broker.Balance {
	return toBrokerBalance(summary)
}
