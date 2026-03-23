package tradier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Tradier API request/response types ---

type tradierOrderResponse struct {
	ID                int64   `json:"id"`
	Type              string  `json:"type"`
	Symbol            string  `json:"symbol"`
	Side              string  `json:"side"`
	Quantity          float64 `json:"quantity"`
	Status            string  `json:"status"`
	Duration          string  `json:"duration"`
	AvgFillPrice      float64 `json:"avg_fill_price"`
	ExecQuantity      float64 `json:"exec_quantity"`
	LastFillPrice     float64 `json:"last_fill_price"`
	LastFillQuantity  float64 `json:"last_fill_quantity"`
	RemainingQuantity float64 `json:"remaining_quantity"`
	CreateDate        string  `json:"create_date"`
	TransactionDate   string  `json:"transaction_date"`
	Class             string  `json:"class"`
	Price             float64 `json:"price"`
	Stop              float64 `json:"stop"`
	Tag               string  `json:"tag"`
}

type tradierOrdersWrapper struct {
	Orders struct {
		Order json.RawMessage `json:"order"`
	} `json:"orders"`
}

type tradierPositionResponse struct {
	ID           int64   `json:"id"`
	Symbol       string  `json:"symbol"`
	Quantity     float64 `json:"quantity"`
	CostBasis    float64 `json:"cost_basis"`
	DateAcquired string  `json:"date_acquired"`
}

type tradierPositionsWrapper struct {
	Positions struct {
		Position json.RawMessage `json:"position"`
	} `json:"positions"`
}

// tradierMarginBalance holds margin account buying-power data.
type tradierMarginBalance struct {
	StockBuyingPower   float64 `json:"stock_buying_power"`
	CurrentRequirement float64 `json:"current_requirement"`
}

// tradierCashBalance holds cash account available-funds data.
type tradierCashBalance struct {
	CashAvailable  float64 `json:"cash_available"`
	UnsettledFunds float64 `json:"unsettled_funds"`
}

type tradierBalanceResponse struct {
	AccountNumber string               `json:"account_number"`
	AccountType   string               `json:"account_type"`
	TotalEquity   float64              `json:"total_equity"`
	TotalCash     float64              `json:"total_cash"`
	MarketValue   float64              `json:"market_value"`
	Margin        tradierMarginBalance `json:"margin"`
	Cash          tradierCashBalance   `json:"cash"`
}

type tradierBalancesWrapper struct {
	Balances tradierBalanceResponse `json:"balances"`
}

type tradierQuoteResponse struct {
	Quotes struct {
		Quote struct {
			Last float64 `json:"last"`
		} `json:"quote"`
	} `json:"quotes"`
}

type tradierOrderSubmitResponse struct {
	Order struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	} `json:"order"`
}

type tradierSessionResponse struct {
	Stream struct {
		SessionID string `json:"sessionid"`
		URL       string `json:"url"`
	} `json:"stream"`
}

type tradierAccountEvent struct {
	ID                int64   `json:"id"`
	Event             string  `json:"event"`
	Status            string  `json:"status"`
	Type              string  `json:"type"`
	Price             float64 `json:"price"`
	StopPrice         float64 `json:"stop_price"`
	AvgFillPrice      float64 `json:"avg_fill_price"`
	ExecutedQuantity  float64 `json:"exec_quantity"`
	LastFillQuantity  float64 `json:"last_fill_quantity"`
	LastFillPrice     float64 `json:"last_fill_price"`
	RemainingQuantity float64 `json:"remaining_quantity"`
	TransactionDate   string  `json:"transaction_date"`
	CreateDate        string  `json:"create_date"`
	Account           string  `json:"account_id"`
}

// --- Generic JSON collection helper ---

// unmarshalFlexible handles Tradier's quirky JSON responses where a collection
// with one element is serialised as a plain object instead of a one-element
// array. It accepts a json.RawMessage that is either a JSON object or a JSON
// array and always returns []T.
func unmarshalFlexible[T any](raw json.RawMessage) ([]T, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return []T{}, nil
	}

	if trimmed[0] == '[' {
		var items []T
		if unmarshalErr := json.Unmarshal(trimmed, &items); unmarshalErr != nil {
			return nil, fmt.Errorf("tradier: unmarshal array: %w", unmarshalErr)
		}

		return items, nil
	}

	var single T
	if unmarshalErr := json.Unmarshal(trimmed, &single); unmarshalErr != nil {
		return nil, fmt.Errorf("tradier: unmarshal object: %w", unmarshalErr)
	}

	return []T{single}, nil
}

// --- Translation functions ---

// toTradierOrderParams converts a broker.Order to Tradier REST form parameters.
func toTradierOrderParams(order broker.Order) (url.Values, error) {
	duration, tifErr := mapTimeInForce(order.TimeInForce)
	if tifErr != nil {
		return nil, tifErr
	}

	params := url.Values{}
	params.Set("class", "equity")
	params.Set("symbol", order.Asset.Ticker)
	params.Set("side", mapSide(order.Side))
	params.Set("quantity", strconv.FormatFloat(order.Qty, 'f', -1, 64))
	params.Set("type", mapOrderType(order.OrderType))
	params.Set("duration", duration)

	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		params.Set("price", strconv.FormatFloat(order.LimitPrice, 'f', -1, 64))
	}

	if order.Justification != "" {
		params.Set("tag", order.Justification)
	}

	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		params.Set("stop", strconv.FormatFloat(order.StopPrice, 'f', -1, 64))
	}

	return params, nil
}

// toBrokerOrder maps a tradierOrderResponse to a broker.Order.
func toBrokerOrder(resp tradierOrderResponse) broker.Order {
	return broker.Order{
		ID:          fmt.Sprintf("%d", resp.ID),
		Asset:       assetFromSymbol(resp.Symbol),
		Side:        mapTradierSide(resp.Side),
		Qty:         resp.Quantity,
		Status:      mapTradierStatus(resp.Status),
		OrderType:   mapTradierOrderType(resp.Type),
		LimitPrice:  resp.Price,
		StopPrice:   resp.Stop,
		TimeInForce: mapTradierDuration(resp.Duration),
	}
}

// toBrokerPosition maps a tradierPositionResponse to a broker.Position.
// MarkPrice is set to 0 and populated later by Positions() via a quote fetch.
// RealizedDayPL is set to 0 as Tradier's positions endpoint does not supply it.
func toBrokerPosition(resp tradierPositionResponse) broker.Position {
	avgPrice := 0.0
	if resp.Quantity != 0 {
		avgPrice = math.Abs(resp.CostBasis) / math.Abs(resp.Quantity)
	}

	return broker.Position{
		Asset:         assetFromSymbol(resp.Symbol),
		Qty:           resp.Quantity,
		AvgOpenPrice:  avgPrice,
		MarkPrice:     0,
		RealizedDayPL: 0,
	}
}

// toBrokerBalance maps a tradierBalanceResponse to a broker.Balance.
// It reads from the margin or cash subsection depending on account type.
func toBrokerBalance(resp tradierBalanceResponse) broker.Balance {
	switch resp.AccountType {
	case "margin":
		return broker.Balance{
			CashBalance:         resp.TotalCash,
			NetLiquidatingValue: resp.TotalEquity,
			EquityBuyingPower:   resp.Margin.StockBuyingPower,
			MaintenanceReq:      resp.Margin.CurrentRequirement,
		}
	default:
		// cash account (and any other account types)
		return broker.Balance{
			CashBalance:         resp.Cash.CashAvailable,
			NetLiquidatingValue: resp.TotalEquity,
			EquityBuyingPower:   resp.Cash.CashAvailable,
			MaintenanceReq:      0,
		}
	}
}

// --- Mapping helpers ---

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "market"
	case broker.Limit:
		return "limit"
	case broker.Stop:
		return "stop"
	case broker.StopLimit:
		return "stop_limit"
	default:
		return "market"
	}
}

func mapTradierOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "market":
		return broker.Market
	case "limit":
		return broker.Limit
	case "stop":
		return broker.Stop
	case "stop_limit":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapSide(side broker.Side) string {
	switch side {
	case broker.Buy:
		return "buy"
	case broker.Sell:
		return "sell"
	default:
		return "buy"
	}
}

func mapTradierSide(side string) broker.Side {
	switch side {
	case "buy", "buy_to_cover":
		return broker.Buy
	case "sell", "sell_short":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapTimeInForce(tif broker.TimeInForce) (string, error) {
	switch tif {
	case broker.Day:
		return "day", nil
	case broker.GTC:
		return "gtc", nil
	case broker.IOC:
		return "", fmt.Errorf("tradier: IOC time-in-force is not supported")
	case broker.FOK:
		return "", fmt.Errorf("tradier: FOK time-in-force is not supported")
	case broker.GTD:
		return "", fmt.Errorf("tradier: GTD time-in-force is not supported")
	case broker.OnOpen:
		return "", fmt.Errorf("tradier: OnOpen time-in-force is not supported")
	case broker.OnClose:
		return "", fmt.Errorf("tradier: OnClose time-in-force is not supported")
	default:
		return "day", nil
	}
}

func mapTradierDuration(duration string) broker.TimeInForce {
	switch duration {
	case "day":
		return broker.Day
	case "gtc":
		return broker.GTC
	default:
		return broker.Day
	}
}

func mapTradierStatus(status string) broker.OrderStatus {
	switch status {
	case "pending":
		return broker.OrderSubmitted
	case "open":
		return broker.OrderOpen
	case "partially_filled":
		return broker.OrderPartiallyFilled
	case "filled":
		return broker.OrderFilled
	case "expired", "canceled", "rejected", "pending_cancel":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

// assetFromSymbol creates an asset.Asset from a ticker symbol.
func assetFromSymbol(symbol string) asset.Asset {
	return asset.Asset{Ticker: symbol}
}
