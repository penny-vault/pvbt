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

package ibkr

import (
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// --- Request types ---

type ibOrderRequest struct {
	Conid      int64   `json:"conid"`
	OrderType  string  `json:"orderType"`
	Side       string  `json:"side"`
	Tif        string  `json:"tif"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price,omitempty"`
	AuxPrice   float64 `json:"auxPrice,omitempty"`
	COID       string  `json:"cOID,omitempty"`
	ParentId   string  `json:"parentId,omitempty"`
	OcaGroup   string  `json:"ocaGroup,omitempty"`
	OcaType    int     `json:"ocaType,omitempty"`
	OutsideRTH bool    `json:"outsideRTH,omitempty"`
}

// --- Response types ---

type ibOrderResponse struct {
	OrderID           string  `json:"orderId"`
	Status            string  `json:"status"`
	Side              string  `json:"side"`
	OrderType         string  `json:"orderType"`
	Ticker            string  `json:"ticker"`
	Conid             int64   `json:"conid"`
	FilledQuantity    float64 `json:"filledQuantity"`
	RemainingQuantity float64 `json:"remainingQuantity"`
	TotalQuantity     float64 `json:"totalQuantity"`
}

type ibPositionEntry struct {
	ContractId int64   `json:"contractId"`
	Position   float64 `json:"position"`
	AvgCost    float64 `json:"avgCost"`
	MktPrice   float64 `json:"mktPrice"`
	Ticker     string  `json:"ticker"`
	Currency   string  `json:"currency"`
}

type summaryValue struct {
	Amount float64 `json:"amount"`
}

type ibAccountSummary struct {
	CashBalance    summaryValue `json:"cashbalance"`
	NetLiquidation summaryValue `json:"netliquidation"`
	BuyingPower    summaryValue `json:"buyingpower"`
	MaintMarginReq summaryValue `json:"maintmarginreq"`
}

type ibSecdefResult struct {
	Conid       int64  `json:"conid"`
	CompanyName string `json:"companyName"`
	Ticker      string `json:"ticker"`
}

type ibOrderReply struct {
	OrderID     string   `json:"order_id"`
	OrderStatus string   `json:"order_status"`
	ReplyID     string   `json:"id,omitempty"`
	Message     []string `json:"message,omitempty"`
}

type ibTradeEntry struct {
	OrderID       string  `json:"order_ref"`
	Price         float64 `json:"price"`
	Quantity      float64 `json:"size"`
	ExecutionTime string  `json:"execution_id"`
}

// --- Mapping functions ---

func toIBOrder(order broker.Order, conid int64) (ibOrderRequest, error) {
	tifStr, err := mapTimeInForce(order.TimeInForce)
	if err != nil {
		return ibOrderRequest{}, err
	}

	req := ibOrderRequest{
		Conid:     conid,
		OrderType: mapOrderType(order.OrderType),
		Side:      mapSide(order.Side),
		Tif:       tifStr,
		Quantity:  order.Qty,
	}

	if order.OrderType == broker.Limit || order.OrderType == broker.StopLimit {
		req.Price = order.LimitPrice
	}

	if order.OrderType == broker.Stop || order.OrderType == broker.StopLimit {
		req.AuxPrice = order.StopPrice
	}

	return req, nil
}

func toBrokerOrder(resp ibOrderResponse) broker.Order {
	return broker.Order{
		ID:        resp.OrderID,
		Asset:     asset.Asset{Ticker: resp.Ticker},
		Side:      mapIBSide(resp.Side),
		Status:    mapIBStatus(resp.Status),
		Qty:       resp.TotalQuantity,
		OrderType: mapIBOrderType(resp.OrderType),
	}
}

func toBrokerPosition(pos ibPositionEntry) broker.Position {
	return broker.Position{
		Asset:        asset.Asset{Ticker: pos.Ticker},
		Qty:          pos.Position,
		AvgOpenPrice: pos.AvgCost,
		MarkPrice:    pos.MktPrice,
	}
}

func toBrokerBalance(summary ibAccountSummary) broker.Balance {
	return broker.Balance{
		CashBalance:         summary.CashBalance.Amount,
		NetLiquidatingValue: summary.NetLiquidation.Amount,
		EquityBuyingPower:   summary.BuyingPower.Amount,
		MaintenanceReq:      summary.MaintMarginReq.Amount,
	}
}

// --- Outbound mapping helpers ---

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
		return "MKT"
	case broker.Limit:
		return "LMT"
	case broker.Stop:
		return "STP"
	case broker.StopLimit:
		return "STP_LIMIT"
	default:
		return "MKT"
	}
}

func mapTimeInForce(tif broker.TimeInForce) (string, error) {
	switch tif {
	case broker.Day:
		return "DAY", nil
	case broker.GTC:
		return "GTC", nil
	case broker.IOC:
		return "IOC", nil
	case broker.OnOpen:
		return "OPG", nil
	case broker.OnClose:
		return "MOC", nil
	case broker.GTD:
		return "", fmt.Errorf("unsupported time-in-force %d: %w", tif, broker.ErrOrderRejected)
	case broker.FOK:
		return "", fmt.Errorf("unsupported time-in-force %d: %w", tif, broker.ErrOrderRejected)
	default:
		return "", fmt.Errorf("unsupported time-in-force %d: %w", tif, broker.ErrOrderRejected)
	}
}

// --- Inbound mapping helpers ---

func mapIBStatus(status string) broker.OrderStatus {
	switch status {
	case "PreSubmitted":
		return broker.OrderSubmitted
	case "Submitted":
		return broker.OrderOpen
	case "Filled":
		return broker.OrderFilled
	case "PartiallyFilled":
		return broker.OrderPartiallyFilled
	case "Cancelled", "Inactive":
		return broker.OrderCancelled
	default:
		return broker.OrderOpen
	}
}

func mapIBOrderType(orderType string) broker.OrderType {
	switch orderType {
	case "MKT":
		return broker.Market
	case "LMT":
		return broker.Limit
	case "STP":
		return broker.Stop
	case "STP_LIMIT":
		return broker.StopLimit
	default:
		return broker.Market
	}
}

func mapIBSide(side string) broker.Side {
	switch side {
	case "BUY":
		return broker.Buy
	case "SELL":
		return broker.Sell
	default:
		return broker.Buy
	}
}
