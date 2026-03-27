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

package webull

import (
	"strconv"

	"github.com/google/uuid"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type orderRequest struct {
	Symbol      string `json:"instrument_id,omitempty"`
	Side        string `json:"side"`
	OrderType   string `json:"order_type"`
	TimeInForce string `json:"time_in_force"`
	Qty         string `json:"qty,omitempty"`
	Notional    string `json:"notional,omitempty"`
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
	ClientID    string `json:"client_order_id"`
}

type replaceRequest struct {
	Qty        string `json:"qty,omitempty"`
	LimitPrice string `json:"limit_price,omitempty"`
	StopPrice  string `json:"stop_price,omitempty"`
}

// --- Response types ---

type orderResponse struct {
	ID          string `json:"order_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Status      string `json:"order_status"`
	OrderType   string `json:"order_type"`
	Qty         string `json:"qty"`
	FilledQty   string `json:"filled_qty"`
	FilledPrice string `json:"filled_avg_price"`
	LimitPrice  string `json:"limit_price"`
	StopPrice   string `json:"stop_price"`
}

type positionResponse struct {
	Symbol       string `json:"symbol"`
	Qty          string `json:"qty"`
	AvgCost      string `json:"avg_cost"`
	MarketValue  string `json:"market_value"`
	UnrealizedPL string `json:"unrealized_pl"`
}

type accountResponse struct {
	AccountID      string `json:"account_id"`
	NetLiquidation string `json:"net_liquidation"`
	CashBalance    string `json:"cash_balance"`
	BuyingPower    string `json:"buying_power"`
	MaintenanceReq string `json:"maintenance_req"`
}

type accountListResponse struct {
	Accounts []accountEntry `json:"accounts"`
}

type accountEntry struct {
	AccountID string `json:"account_id"`
	Status    string `json:"status"`
}

// --- Helper functions ---

func parseFloat(value string) float64 {
	result, parseErr := strconv.ParseFloat(value, 64)
	if parseErr != nil {
		return 0
	}

	return result
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// --- Outbound mapping: broker.* -> Webull ---

func toWebullOrder(order broker.Order, fractional bool) orderRequest {
	req := orderRequest{
		Symbol:      order.Asset.Ticker,
		Side:        mapSide(order.Side),
		OrderType:   mapOrderType(order.OrderType),
		TimeInForce: mapTimeInForce(order.TimeInForce),
		ClientID:    uuid.New().String(),
	}

	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		req.LimitPrice = formatFloat(order.LimitPrice)
	}

	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		req.StopPrice = formatFloat(order.StopPrice)
	}

	if fractional && order.Qty == 0 && order.Amount > 0 {
		req.Notional = formatFloat(order.Amount)
	} else {
		req.Qty = formatFloat(order.Qty)
	}

	return req
}

// --- Inbound mapping: Webull -> broker.* ---

func toBrokerOrder(resp orderResponse) broker.Order {
	return broker.Order{
		ID:         resp.ID,
		Asset:      asset.Asset{Ticker: resp.Symbol},
		Side:       mapWebullSide(resp.Side),
		Status:     mapWebullStatus(resp.Status),
		Qty:        parseFloat(resp.Qty),
		OrderType:  mapWebullOrderType(resp.OrderType),
		LimitPrice: parseFloat(resp.LimitPrice),
		StopPrice:  parseFloat(resp.StopPrice),
	}
}

func toBrokerPosition(resp positionResponse) broker.Position {
	return broker.Position{
		Asset:        asset.Asset{Ticker: resp.Symbol},
		Qty:          parseFloat(resp.Qty),
		AvgOpenPrice: parseFloat(resp.AvgCost),
		MarkPrice:    parseFloat(resp.MarketValue),
	}
}

func toBrokerBalance(resp accountResponse) broker.Balance {
	return broker.Balance{
		NetLiquidatingValue: parseFloat(resp.NetLiquidation),
		CashBalance:         parseFloat(resp.CashBalance),
		EquityBuyingPower:   parseFloat(resp.BuyingPower),
		MaintenanceReq:      parseFloat(resp.MaintenanceReq),
	}
}

// --- Outbound enum mappers ---

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

func mapOrderType(orderType broker.OrderType) string {
	switch orderType {
	case broker.Market:
		return "MARKET"
	case broker.Limit:
		return "LIMIT"
	case broker.Stop:
		return "STOP_LOSS"
	case broker.StopLimit:
		return "STOP_LOSS_LIMIT"
	default:
		return "MARKET"
	}
}

func mapTimeInForce(tif broker.TimeInForce) string {
	switch tif {
	case broker.Day:
		return "DAY"
	case broker.GTC:
		return "GTC"
	default:
		return "DAY"
	}
}

// --- Inbound enum mappers ---

func mapWebullSide(side string) broker.Side {
	switch side {
	case "BUY":
		return broker.Buy
	case "SELL":
		return broker.Sell
	default:
		return broker.Buy
	}
}

func mapWebullOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "MARKET":
		return broker.Market
	case "LIMIT":
		return broker.Limit
	case "STOP_LOSS":
		return broker.Stop
	case "STOP_LOSS_LIMIT":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapWebullStatus(status string) broker.OrderStatus {
	switch status {
	case "PENDING", "NEW":
		return broker.OrderSubmitted
	case "PARTIALLY_FILLED":
		return broker.OrderPartiallyFilled
	case "FILLED":
		return broker.OrderFilled
	case "CANCELLED", "EXPIRED", "REJECTED":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}
