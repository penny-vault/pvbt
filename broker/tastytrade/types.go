package tastytrade

import (
	"encoding/json"
	"strconv"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type sessionRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Data struct {
		SessionToken string       `json:"session-token"`
		User         userResponse `json:"user"`
	} `json:"data"`
}

type userResponse struct {
	ExternalID string `json:"external-id"`
}

type accountsResponse struct {
	Data struct {
		Items []accountItem `json:"items"`
	} `json:"data"`
}

type accountItem struct {
	Account struct {
		AccountNumber string `json:"account-number"`
	} `json:"account"`
}

type orderRequest struct {
	TimeInForce     string     `json:"time-in-force"`
	OrderType       string     `json:"order-type"`
	Price           float64    `json:"price,omitempty"`
	PriceEffect     string     `json:"price-effect,omitempty"`
	StopTrigger     float64    `json:"stop-trigger,omitempty"`
	AutomatedSource bool       `json:"automated-source"`
	Legs            []orderLeg `json:"legs"`
}

type orderLeg struct {
	InstrumentType string  `json:"instrument-type"`
	Symbol         string  `json:"symbol"`
	Action         string  `json:"action"`
	Quantity       float64 `json:"quantity"`
}

// --- Response types ---

type orderSubmitResponse struct {
	Data struct {
		Order orderResponse `json:"order"`
	} `json:"data"`
}

type ordersListResponse struct {
	Data struct {
		Items []orderResponse `json:"items"`
	} `json:"data"`
}

type complexOrderRequest struct {
	Type         string         `json:"type"`
	TriggerOrder *orderRequest  `json:"trigger-order,omitempty"`
	Orders       []orderRequest `json:"orders"`
}

type complexOrderSubmitResponse struct {
	Data struct {
		ComplexOrder struct {
			ID     string          `json:"id"`
			Orders []orderResponse `json:"orders"`
		} `json:"complex-order"`
	} `json:"data"`
}

type orderResponse struct {
	ID             string             `json:"id"`
	Status         string             `json:"status"`
	OrderType      string             `json:"order-type"`
	TimeInForce    string             `json:"time-in-force"`
	Price          float64            `json:"price"`
	StopTrigger    float64            `json:"stop-trigger"`
	ComplexOrderID string             `json:"complex-order-id"`
	Legs           []orderLegResponse `json:"legs"`
}

type orderLegResponse struct {
	Symbol         string            `json:"symbol"`
	InstrumentType string            `json:"instrument-type"`
	Action         string            `json:"action"`
	Quantity       float64           `json:"quantity"`
	Fills          []legFillResponse `json:"fills"`
}

type legFillResponse struct {
	FillID   string  `json:"fill-id"`
	Price    float64 `json:"fill-price"`
	Quantity string  `json:"quantity"`
	FilledAt string  `json:"filled-at"`
}

type streamerMessage struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type positionsListResponse struct {
	Data struct {
		Items []positionResponse `json:"items"`
	} `json:"data"`
}

type positionResponse struct {
	Symbol        string  `json:"symbol"`
	Quantity      float64 `json:"quantity"`
	AveragePrice  float64 `json:"average-open-price"`
	MarkPrice     float64 `json:"mark-price"`
	RealizedDayPL float64 `json:"realized-day-gain-effect"`
}

// balanceResponse contains the inner balance fields only.
// The client method unwraps the tastytrade JSON envelope before populating this.
type balanceResponse struct {
	CashBalance         float64 `json:"cash-balance"`
	NetLiquidatingValue float64 `json:"net-liquidating-value"`
	EquityBuyingPower   float64 `json:"equity-buying-power"`
	MaintenanceReq      float64 `json:"maintenance-requirement"`
}

type quoteResponse struct {
	Data struct {
		Items []quoteItem `json:"items"`
	} `json:"data"`
}

type quoteItem struct {
	Symbol    string  `json:"symbol"`
	LastPrice float64 `json:"last"`
}

// --- Translation functions ---

func toTastytradeOrder(order broker.Order) orderRequest {
	return orderRequest{
		TimeInForce:     mapTimeInForce(order.TimeInForce),
		OrderType:       mapOrderType(order.OrderType),
		Price:           order.LimitPrice,
		PriceEffect:     mapPriceEffect(order.Side, order.OrderType),
		StopTrigger:     order.StopPrice,
		AutomatedSource: true,
		Legs: []orderLeg{
			{
				InstrumentType: "Equity",
				Symbol:         order.Asset.Ticker,
				Action:         mapSide(order.Side),
				Quantity:       order.Qty,
			},
		},
	}
}

func toBrokerOrder(resp orderResponse) broker.Order {
	order := broker.Order{
		ID:         resp.ID,
		Status:     mapTTStatus(resp.Status),
		OrderType:  mapTTOrderType(resp.OrderType),
		LimitPrice: resp.Price,
		StopPrice:  resp.StopTrigger,
	}

	if len(resp.Legs) > 0 {
		leg := resp.Legs[0]
		order.Asset = asset.Asset{Ticker: leg.Symbol}
		order.Qty = leg.Quantity
		order.Side = mapTTSide(leg.Action)
	}

	return order
}

func toBrokerPosition(resp positionResponse) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Symbol},
		Qty:           resp.Quantity,
		AvgOpenPrice:  resp.AveragePrice,
		MarkPrice:     resp.MarkPrice,
		RealizedDayPL: resp.RealizedDayPL,
	}
}

func toBrokerBalance(resp balanceResponse) broker.Balance {
	return broker.Balance{
		CashBalance:         resp.CashBalance,
		NetLiquidatingValue: resp.NetLiquidatingValue,
		EquityBuyingPower:   resp.EquityBuyingPower,
		MaintenanceReq:      resp.MaintenanceReq,
	}
}

func parseLegFillQuantity(quantity string) float64 {
	value, err := strconv.ParseFloat(quantity, 64)
	if err != nil {
		return 0
	}
	return value
}

// --- Mapping helpers ---

func mapPriceEffect(side broker.Side, orderType broker.OrderType) string {
	if orderType == broker.Market {
		return ""
	}

	switch side {
	case broker.Buy:
		return "Debit"
	case broker.Sell:
		return "Credit"
	default:
		return "Debit"
	}
}

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "Buy to Open"
	case broker.Sell:
		return "Sell to Close"
	default:
		return "Buy to Open"
	}
}

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "Market"
	case broker.Limit:
		return "Limit"
	case broker.Stop:
		return "Stop"
	case broker.StopLimit:
		return "Stop Limit"
	default:
		return "Market"
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day, broker.OnOpen, broker.OnClose:
		return "Day"
	case broker.GTC:
		return "GTC"
	case broker.GTD:
		return "GTD"
	case broker.IOC:
		return "IOC"
	case broker.FOK:
		return "FOK"
	default:
		return "Day"
	}
}

func mapTTStatus(status string) broker.OrderStatus {
	switch status {
	case "Received", "Routed", "In Flight", "Contingent":
		return broker.OrderSubmitted
	case "Live":
		return broker.OrderOpen
	case "Filled":
		return broker.OrderFilled
	case "Cancelled", "Expired", "Rejected":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func mapTTOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "Market":
		return broker.Market
	case "Limit":
		return broker.Limit
	case "Stop":
		return broker.Stop
	case "Stop Limit":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapTTSide(action string) broker.Side {
	switch action {
	case "Buy to Open", "Buy to Close":
		return broker.Buy
	case "Sell to Open", "Sell to Close":
		return broker.Sell
	default:
		return broker.Buy
	}
}
