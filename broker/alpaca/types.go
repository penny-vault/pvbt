// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alpaca

import (
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type orderRequest struct {
	Symbol        string             `json:"symbol"`
	Qty           string             `json:"qty,omitempty"`
	Notional      string             `json:"notional,omitempty"`
	Side          string             `json:"side"`
	Type          string             `json:"type"`
	TimeInForce   string             `json:"time_in_force"`
	LimitPrice    string             `json:"limit_price,omitempty"`
	StopPrice     string             `json:"stop_price,omitempty"`
	ExpireTime    string             `json:"expire_time,omitempty"`
	ClientOrderID string             `json:"client_order_id"`
	OrderClass    string             `json:"order_class,omitempty"`
	TakeProfit    *takeProfitRequest `json:"take_profit,omitempty"`
	StopLoss      *stopLossRequest   `json:"stop_loss,omitempty"`
}

type takeProfitRequest struct {
	LimitPrice string `json:"limit_price"`
}

type stopLossRequest struct {
	StopPrice string `json:"stop_price"`
}

type replaceRequest struct {
	Qty         string `json:"qty,omitempty"`
	TimeInForce string `json:"time_in_force,omitempty"`
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
}

// --- Response types ---

type orderResponse struct {
	ID             string          `json:"id"`
	ClientOrderID  string          `json:"client_order_id"`
	Status         string          `json:"status"`
	Type           string          `json:"type"`
	Side           string          `json:"side"`
	Symbol         string          `json:"symbol"`
	Qty            string          `json:"qty"`
	FilledQty      string          `json:"filled_qty"`
	FilledAvgPrice string          `json:"filled_avg_price"`
	LimitPrice     string          `json:"limit_price"`
	StopPrice      string          `json:"stop_price"`
	TimeInForce    string          `json:"time_in_force"`
	OrderClass     string          `json:"order_class"`
	Legs           []orderResponse `json:"legs"`
}

type positionResponse struct {
	Symbol               string `json:"symbol"`
	Qty                  string `json:"qty"`
	AvgEntryPrice        string `json:"avg_entry_price"`
	CurrentPrice         string `json:"current_price"`
	UnrealizedIntradayPL string `json:"unrealized_intraday_pl"`
}

type accountResponse struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	Cash              string `json:"cash"`
	Equity            string `json:"equity"`
	BuyingPower       string `json:"buying_power"`
	MaintenanceMargin string `json:"maintenance_margin"`
}

type latestTradeResponse struct {
	Trade struct {
		Price string `json:"p"`
	} `json:"trade"`
}

// --- WebSocket message types ---

type wsAuthMessage struct {
	Action string `json:"action"`
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

type wsListenMessage struct {
	Action string `json:"action"`
	Data   struct {
		Streams []string `json:"streams"`
	} `json:"data"`
}

type wsStreamMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type wsTradeUpdate struct {
	Event       string      `json:"event"`
	ExecutionID string      `json:"execution_id"`
	Price       string      `json:"price"`
	Qty         string      `json:"qty"`
	Timestamp   string      `json:"timestamp"`
	Order       wsOrderData `json:"order"`
}

type wsOrderData struct {
	ID string `json:"id"`
}

type wsAuthResponse struct {
	Stream string `json:"stream"`
	Data   struct {
		Status string `json:"status"`
		Action string `json:"action"`
	} `json:"data"`
}

// --- Helper functions ---

func parseFloat(value string) float64 {
	result, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	return result
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// --- Mapping functions ---

func toAlpacaOrder(order broker.Order, fractional bool) orderRequest {
	request := orderRequest{
		Symbol:        order.Asset.Ticker,
		Side:          mapSide(order.Side),
		Type:          mapOrderType(order.OrderType),
		TimeInForce:   mapTimeInForce(order.TimeInForce),
		ClientOrderID: uuid.New().String(),
	}

	// Set limit price for Limit and StopLimit orders.
	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		request.LimitPrice = formatFloat(order.LimitPrice)
	}

	// Set stop price for Stop and StopLimit orders.
	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		request.StopPrice = formatFloat(order.StopPrice)
	}

	// Set expire time for GTD orders.
	if order.TimeInForce == broker.GTD {
		request.ExpireTime = order.GTDDate.Format("2006-01-02T15:04:05Z")
	}

	// For fractional dollar-amount orders, use notional instead of qty.
	if fractional && order.Qty == 0 && order.Amount > 0 {
		request.Notional = formatFloat(order.Amount)
	} else {
		request.Qty = formatFloat(order.Qty)
	}

	return request
}

func toBrokerOrder(resp orderResponse) broker.Order {
	return broker.Order{
		ID:         resp.ID,
		Asset:      asset.Asset{Ticker: resp.Symbol},
		Side:       mapAlpacaSide(resp.Side),
		Status:     mapAlpacaStatus(resp.Status),
		Qty:        parseFloat(resp.Qty),
		OrderType:  mapAlpacaOrderType(resp.Type),
		LimitPrice: parseFloat(resp.LimitPrice),
		StopPrice:  parseFloat(resp.StopPrice),
	}
}

func toBrokerPosition(resp positionResponse) broker.Position {
	return broker.Position{
		Asset:         asset.Asset{Ticker: resp.Symbol},
		Qty:           parseFloat(resp.Qty),
		AvgOpenPrice:  parseFloat(resp.AvgEntryPrice),
		MarkPrice:     parseFloat(resp.CurrentPrice),
		RealizedDayPL: parseFloat(resp.UnrealizedIntradayPL),
	}
}

func toBrokerBalance(resp accountResponse) broker.Balance {
	return broker.Balance{
		CashBalance:         parseFloat(resp.Cash),
		NetLiquidatingValue: parseFloat(resp.Equity),
		EquityBuyingPower:   parseFloat(resp.BuyingPower),
		MaintenanceReq:      parseFloat(resp.MaintenanceMargin),
	}
}

// --- Outbound mapping helpers ---

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

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day:
		return "day"
	case broker.GTC:
		return "gtc"
	case broker.GTD:
		return "gtd"
	case broker.IOC:
		return "ioc"
	case broker.FOK:
		return "fok"
	case broker.OnOpen:
		return "opg"
	case broker.OnClose:
		return "cls"
	default:
		return "day"
	}
}

// --- Inbound mapping helpers ---

func mapAlpacaStatus(status string) broker.OrderStatus {
	switch status {
	case "new", "accepted", "pending_new":
		return broker.OrderSubmitted
	case "partially_filled":
		return broker.OrderPartiallyFilled
	case "filled":
		return broker.OrderFilled
	case "canceled", "expired", "rejected", "suspended":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func mapAlpacaOrderType(orderType string) broker.OrderType {
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

func mapAlpacaSide(side string) broker.Side {
	switch side {
	case "buy":
		return broker.Buy
	case "sell":
		return broker.Sell
	default:
		return broker.Buy
	}
}
