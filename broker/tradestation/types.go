package tradestation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- TradeStation API request/response types ---

type tsOrderRequest struct {
	AccountID   string        `json:"AccountID"`
	Symbol      string        `json:"Symbol"`
	Quantity    string        `json:"Quantity"`
	OrderType   string        `json:"OrderType"`
	TradeAction string        `json:"TradeAction"`
	TimeInForce tsTimeInForce `json:"TimeInForce"`
	Route       string        `json:"Route"`
	LimitPrice  string        `json:"LimitPrice,omitempty"`
	StopPrice   string        `json:"StopPrice,omitempty"`
}

type tsTimeInForce struct {
	Duration   string `json:"Duration"`
	Expiration string `json:"Expiration,omitempty"`
}

type tsGroupOrderRequest struct {
	Type   string           `json:"Type"`
	Orders []tsOrderRequest `json:"Orders"`
}

type tsOrderResponse struct {
	OrderID     string       `json:"OrderID"`
	Status      string       `json:"Status"`
	StatusDesc  string       `json:"StatusDescription"`
	OrderType   string       `json:"OrderType"`
	LimitPrice  string       `json:"LimitPrice"`
	StopPrice   string       `json:"StopPrice"`
	Duration    string       `json:"Duration"`
	Legs        []tsOrderLeg `json:"Legs"`
	FilledQty   string       `json:"FilledQuantity"`
	FilledPrice string       `json:"FilledPrice"`
}

type tsOrderLeg struct {
	BuySellSideCode string      `json:"BuyOrSell"`
	Symbol          string      `json:"Symbol"`
	QuantityOrdered string      `json:"QuantityOrdered"`
	ExecQuantity    string      `json:"ExecQuantity"`
	ExecPrice       string      `json:"ExecPrice"`
	Fills           []tsLegFill `json:"Fills"`
}

type tsLegFill struct {
	ExecID    string `json:"ExecId"`
	Quantity  string `json:"Quantity"`
	Price     string `json:"Price"`
	Timestamp string `json:"Timestamp"`
}

type tsAccountEntry struct {
	AccountID   string `json:"AccountID"`
	AccountType string `json:"AccountType"`
	Status      string `json:"Status"`
}

type tsPositionEntry struct {
	Symbol           string `json:"Symbol"`
	Quantity         string `json:"Quantity"`
	AveragePrice     string `json:"AveragePrice"`
	MarketValue      string `json:"MarketValue"`
	TodaysProfitLoss string `json:"TodaysProfitLoss"`
	Last             string `json:"Last"`
}

type tsBalanceResponse struct {
	CashBalance string `json:"CashBalance"`
	Equity      string `json:"Equity"`
	BuyingPower string `json:"BuyingPower"`
	MarketValue string `json:"MarketValue"`
}

type tsQuoteResponse struct {
	Quotes []tsQuote `json:"Quotes"`
}

type tsQuote struct {
	Symbol string  `json:"Symbol"`
	Last   float64 `json:"Last"`
}

type tsTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// tsStreamOrderEvent represents an order update from the HTTP streaming endpoint.
type tsStreamOrderEvent struct {
	OrderID     string       `json:"OrderID"`
	Status      string       `json:"Status"`
	OrderType   string       `json:"OrderType"`
	FilledQty   string       `json:"FilledQuantity"`
	FilledPrice string       `json:"FilledPrice"`
	Legs        []tsOrderLeg `json:"Legs"`
	// Heartbeat and snapshot signals
	Heartbeat   int    `json:"Heartbeat,omitempty"`
	EndSnapshot bool   `json:"EndSnapshot,omitempty"`
	GoAway      bool   `json:"GoAway,omitempty"`
	Error       string `json:"Error,omitempty"`
}

// --- Translation functions ---

func toTSOrder(order broker.Order, accountID string) (tsOrderRequest, error) {
	tif := mapTimeInForce(order.TimeInForce)

	tsOrder := tsOrderRequest{
		AccountID:   accountID,
		Symbol:      order.Asset.Ticker,
		Quantity:    formatQty(order.Qty),
		OrderType:   mapOrderType(order.OrderType),
		TradeAction: mapSide(order.Side),
		TimeInForce: tsTimeInForce{Duration: tif},
		Route:       "Intelligent",
	}

	if order.LimitPrice != 0 {
		tsOrder.LimitPrice = fmt.Sprintf("%.2f", order.LimitPrice)
	}

	if order.StopPrice != 0 {
		tsOrder.StopPrice = fmt.Sprintf("%.2f", order.StopPrice)
	}

	if order.TimeInForce == broker.GTD && !order.GTDDate.IsZero() {
		tsOrder.TimeInForce.Expiration = order.GTDDate.Format("2006-01-02")
	}

	return tsOrder, nil
}

func toBrokerOrder(resp tsOrderResponse) broker.Order {
	order := broker.Order{
		ID:        resp.OrderID,
		Status:    mapTSStatus(resp.Status),
		OrderType: mapTSOrderType(resp.OrderType),
	}

	order.LimitPrice = parseFloat(resp.LimitPrice)
	order.StopPrice = parseFloat(resp.StopPrice)

	if len(resp.Legs) > 0 {
		leg := resp.Legs[0]
		order.Asset = asset.Asset{Ticker: leg.Symbol}
		order.Qty = parseFloat(leg.QuantityOrdered)
		order.Side = mapTSSide(leg.BuySellSideCode)
	}

	return order
}

func toBrokerPosition(resp tsPositionEntry) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Symbol},
		Qty:           parseFloat(resp.Quantity),
		AvgOpenPrice:  parseFloat(resp.AveragePrice),
		MarkPrice:     parseFloat(resp.Last),
		RealizedDayPL: parseFloat(resp.TodaysProfitLoss),
	}
}

func toBrokerBalance(resp tsBalanceResponse) broker.Balance {
	return broker.Balance{
		CashBalance:         parseFloat(resp.CashBalance),
		NetLiquidatingValue: parseFloat(resp.Equity),
		EquityBuyingPower:   parseFloat(resp.BuyingPower),
	}
}

// --- Mapping helpers ---

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "Market"
	case broker.Limit:
		return "Limit"
	case broker.Stop:
		return "StopMarket"
	case broker.StopLimit:
		return "StopLimit"
	default:
		return "Market"
	}
}

func mapTSOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "Market":
		return broker.Market
	case "Limit":
		return broker.Limit
	case "StopMarket":
		return broker.Stop
	case "StopLimit":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "BUY"
	case broker.Sell:
		return "SELL"
	default:
		return "BUY"
	}
}

// mapTSSide maps TradeStation BuySellSideCode to broker.Side.
// 1=Buy, 2=Sell, 3=SellShort, 4=BuyToCover.
func mapTSSide(code string) broker.Side {
	switch code {
	case "1", "4":
		return broker.Buy
	case "2", "3":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day:
		return "DAY"
	case broker.GTC:
		return "GTC"
	case broker.GTD:
		return "GTD"
	case broker.IOC:
		return "IOC"
	case broker.FOK:
		return "FOK"
	case broker.OnOpen:
		return "OPG"
	case broker.OnClose:
		return "CLO"
	default:
		return "DAY"
	}
}

// mapTSStatus maps TradeStation order status codes to broker.OrderStatus.
// Reference: ACK=acknowledged, DON=condition pending, OPN=open, FLL=filled,
// FLP=partially filled, OUT=cancelled/completed, CAN=cancelled, EXP=expired,
// REJ=rejected, UCN=unable to cancel, BRO=broken.
func mapTSStatus(status string) broker.OrderStatus {
	switch status {
	case "ACK", "DON", "CND", "QUE", "REC":
		return broker.OrderSubmitted
	case "OPN":
		return broker.OrderOpen
	case "FLL":
		return broker.OrderFilled
	case "FLP":
		return broker.OrderPartiallyFilled
	case "OUT", "CAN", "EXP", "REJ", "UCN", "BRO", "TSC":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func stripDashes(orderID string) string {
	return strings.ReplaceAll(orderID, "-", "")
}

func parseFloat(str string) float64 {
	if str == "" {
		return 0
	}

	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return 0
	}

	return val
}

// --- Group order builders ---

func buildGroupOrder(orders []broker.Order, groupType broker.GroupType, accountID string) (tsGroupOrderRequest, error) {
	if len(orders) == 0 {
		return tsGroupOrderRequest{}, broker.ErrEmptyOrderGroup
	}

	var groupTypeStr string

	switch groupType {
	case broker.GroupOCO:
		groupTypeStr = "OCO"
	case broker.GroupBracket:
		groupTypeStr = "BRK"
	default:
		return tsGroupOrderRequest{}, fmt.Errorf("tradestation: unsupported group type %d", groupType)
	}

	tsOrders := make([]tsOrderRequest, 0, len(orders))

	if groupType == broker.GroupBracket {
		// Entry order must be first in the array.
		var entryOrder *tsOrderRequest

		var contingentOrders []tsOrderRequest

		for _, order := range orders {
			tsOrd, err := toTSOrder(order, accountID)
			if err != nil {
				return tsGroupOrderRequest{}, err
			}

			if order.GroupRole == broker.RoleEntry {
				if entryOrder != nil {
					return tsGroupOrderRequest{}, broker.ErrMultipleEntryOrders
				}

				entryOrder = &tsOrd
			} else {
				contingentOrders = append(contingentOrders, tsOrd)
			}
		}

		if entryOrder == nil {
			return tsGroupOrderRequest{}, broker.ErrNoEntryOrder
		}

		tsOrders = append(tsOrders, *entryOrder)
		tsOrders = append(tsOrders, contingentOrders...)
	} else {
		for _, order := range orders {
			tsOrd, err := toTSOrder(order, accountID)
			if err != nil {
				return tsGroupOrderRequest{}, err
			}

			tsOrders = append(tsOrders, tsOrd)
		}
	}

	return tsGroupOrderRequest{
		Type:   groupTypeStr,
		Orders: tsOrders,
	}, nil
}
