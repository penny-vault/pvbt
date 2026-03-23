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

// Package tradier implements broker.Broker for the Tradier brokerage.
//
// Tradier offers a clean REST API popular with algo traders. This broker
// supports equities only. Strategies that work with the SimulatedBroker
// require no changes to run live -- swap the broker via
// engine.WithBroker(tradier.New()).
//
// # Authentication
//
// The broker supports two authentication modes:
//
//   - API access token: Set TRADIER_ACCESS_TOKEN. Individual tokens never
//     expire. This is the simplest mode for personal use.
//   - OAuth 2.0: Set TRADIER_CLIENT_ID and TRADIER_CLIENT_SECRET. On first
//     run, Connect() prints an authorization URL. Access tokens expire in
//     24 hours; refresh tokens (partner accounts only) are used automatically.
//
// Both modes require TRADIER_ACCOUNT_ID.
//
// # Sandbox
//
// Use WithSandbox() to target the Tradier sandbox environment:
//
//	broker := tradier.New(tradier.WithSandbox())
//
// The sandbox uses paper money. Note: account streaming is not available
// in sandbox mode; the broker falls back to polling for fills.
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to Tradier's account
// events streamer. On disconnect, the broker reconnects with exponential
// backoff and polls for any fills missed during the outage. Duplicate
// fills are suppressed automatically.
//
// # Order Types
//
// Market, Limit, Stop, and StopLimit are supported. Duration supports
// Day and GTC. Dollar-amount orders (Qty=0, Amount>0) are converted to
// share quantities by fetching a real-time quote.
//
// # Order Groups
//
// TradierBroker implements broker.GroupSubmitter for native OCO and
// bracket (OTOCO) order support.
package tradier
