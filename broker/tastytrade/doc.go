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

// Package tastytrade implements broker.Broker for the tastytrade brokerage.
//
// This is the first live broker integration for pvbt. It supports equities
// only. Strategies that work with the SimulatedBroker require no changes
// to run live -- swap the broker via engine.WithBroker(tastytrade.New()).
//
// # Authentication
//
// The broker reads credentials from environment variables:
//
//   - TASTYTRADE_USERNAME: tastytrade account username
//   - TASTYTRADE_PASSWORD: tastytrade account password
//
// Authentication happens during Connect(). The session token is managed
// internally and refreshed automatically on 401 responses.
//
// # Sandbox
//
// Use WithSandbox() to target the tastytrade sandbox environment for
// testing without risking real money:
//
//	broker := tastytrade.New(tastytrade.WithSandbox())
//
// The sandbox API mirrors production but uses paper money.
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to tastytrade's account
// streamer. On disconnect, the broker reconnects with exponential backoff
// and polls for any fills missed during the outage. Duplicate fills are
// suppressed automatically.
//
// # Order Types
//
// All four broker.OrderType values are supported: Market, Limit, Stop,
// and StopLimit. Dollar-amount orders (Qty=0, Amount>0) are converted
// to share quantities by fetching a real-time quote.
//
// # Order Groups
//
// TastytradeBroker implements broker.GroupSubmitter for native OCO and
// bracket (OTOCO) order support. OCO pairs are submitted atomically
// via tastytrade's complex-orders endpoint. Bracket orders map the
// entry to a trigger order and the stop-loss/take-profit legs to
// contingent orders.
package tastytrade
