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

// Package broker defines the interface between the portfolio and a brokerage.
//
// The package is intentionally decoupled from the rest of the system: broker
// imports only the asset package, and the portfolio package imports broker.
// This creates a one-way dependency chain. Strategy code never interacts with
// a broker directly; the engine mediates all communication.
//
// # Broker Interface
//
// The Broker interface is organized into three groups:
//
//   - Lifecycle: Connect establishes a session with the brokerage, handling
//     authentication and setup. Close tears down the session and releases
//     resources.
//   - Trading: Submit sends an order and returns fill reports. Cancel
//     requests cancellation of an open order by ID. Replace performs an
//     atomic cancel-replace, cancelling an existing order and submitting a
//     replacement in one step.
//   - Queries: Orders returns all orders for the current trading day.
//     Positions returns current holdings. Balance returns the account's
//     financial state.
//
// All methods accept a context.Context as their first parameter.
//
// # Order
//
// An Order describes a trade to be executed. Key fields:
//
//   - Side: Buy or Sell.
//   - Qty and Amount: when Qty is zero and Amount is positive the broker
//     treats the order as a dollar-amount order and computes the share
//     quantity from the current market price.
//   - OrderType: the price behavior (see Order Types below).
//   - TimeInForce: how long the order remains active (see Time in Force
//     below).
//   - LimitPrice, StopPrice: trigger and limit prices for Limit, Stop,
//     and StopLimit orders.
//   - GTDDate: expiry date for GTD time-in-force orders.
//
// OrderStatus tracks an order through its lifecycle:
//
//   - OrderOpen: created but not yet sent.
//   - OrderSubmitted: sent to the brokerage.
//   - OrderFilled: fully executed.
//   - OrderPartiallyFilled: partially executed, remainder still active.
//   - OrderCancelled: cancelled before full execution.
//
// # Order Types
//
//   - Market: execute immediately at the best available price.
//   - Limit: execute at the specified LimitPrice or better.
//   - Stop: becomes a market order when the price reaches StopPrice.
//   - StopLimit: becomes a limit order at LimitPrice when the price reaches
//     StopPrice.
//
// # Time in Force
//
//   - Day: valid for the current trading session only.
//   - GTC (good until cancelled): remains active across sessions.
//   - GTD (good until date): remains active until the date specified by
//     GTDDate.
//   - IOC (immediate or cancel): fill what is possible immediately,
//     cancel the remainder.
//   - FOK (fill or kill): fill the entire order immediately or cancel it
//     entirely.
//   - OnOpen: execute during the opening auction.
//   - OnClose: execute during the closing auction.
//
// # Fill
//
// A Fill reports how an order was executed. It contains the OrderID, the
// execution Price, the filled Qty, and the timestamp FilledAt. A single
// order may produce multiple fills when executed in lots at different
// prices.
//
// # Order Groups
//
// Orders can be linked into groups for coordinated execution:
//
//   - OCO (one-cancels-other): two orders where filling one automatically
//     cancels the other. Used for attaching stop-loss and take-profit exits
//     to an existing position.
//   - Bracket: an entry order plus an OCO pair (stop loss and take profit)
//     that activates when the entry fills.
//
// GroupType identifies the group kind (GroupOCO, GroupBracket). GroupRole
// identifies an order's role within its group (RoleEntry, RoleStopLoss,
// RoleTakeProfit). The Order struct carries GroupID and GroupRole fields.
//
// OrderGroup links related orders by their IDs. The portfolio account layer
// tracks groups and orchestrates fill-triggered cancellation.
//
// # GroupSubmitter
//
// Brokers that natively support OCO/bracket groups can implement the
// GroupSubmitter interface. Its single method, SubmitGroup, submits a
// slice of linked orders atomically. When a broker does not implement
// GroupSubmitter, the account layer submits orders individually and
// manages cancellation on fill.
//
// # Position
//
// A Position represents a current holding in the account. It carries the
// Asset, the share Qty, the average open price (AvgOpenPrice), the current
// mark price (MarkPrice), and the realized profit or loss for the day
// (RealizedDayPL).
//
// # Balance
//
// Balance captures the account's financial state: CashBalance, the net
// liquidating value (NetLiquidatingValue), equity buying power
// (EquityBuyingPower), and the maintenance margin requirement
// (MaintenanceReq).
//
// # PriceProvider
//
// The PriceProvider interface supplies current market prices. The engine
// implements this interface. The SimulatedBroker uses it to determine fill
// prices and to convert dollar-amount orders into share quantities.
//
// # SimulatedBroker
//
// SimulatedBroker lives in the engine package and fills all orders at the
// closing price for backtesting. It supports dollar-amount orders by
// dividing the requested amount by the close price. Cancel is supported for
// managing pending bracket/OCO orders. Replace is not supported and returns
// an error if called.
//
// # Implementing a Broker
//
// Future broker implementations should live in sub-packages under broker/
// (for example broker/alpaca or broker/ibkr). Each sub-package provides a
// concrete type that satisfies the Broker interface and handles the
// brokerage-specific wire protocol, authentication, and order routing.
package broker
