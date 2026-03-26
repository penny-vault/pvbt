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

// Package tradestation implements broker.Broker for the TradeStation brokerage.
//
// This broker integrates with TradeStation's v3 REST API for order management
// and HTTP chunked streaming for real-time fill delivery via order status
// updates.
//
// # Authentication
//
// TradeStation uses OAuth 2.0 with the authorization_code grant type via Auth0.
// The broker reads credentials from environment variables:
//
//   - TRADESTATION_CLIENT_ID: OAuth app key
//   - TRADESTATION_CLIENT_SECRET: OAuth app secret
//   - TRADESTATION_CALLBACK_URL: Registered callback URL (default: https://127.0.0.1:5174)
//   - TRADESTATION_TOKEN_FILE: Path to persist tokens (default: ~/.config/pvbt/tradestation-tokens.json)
//   - TRADESTATION_ACCOUNT_ID: Account ID to trade (optional; uses first account if unset)
//
// On first run, Connect() prints an authorization URL. Open it in a browser,
// log in to TradeStation, and authorize the app. The callback server captures
// the tokens automatically. Subsequent runs reuse the stored refresh token.
//
// # Fill Delivery
//
// Fills are delivered via an HTTP chunked stream to the order status endpoint.
// On disconnect, the broker reconnects with exponential backoff and polls for
// any fills missed during the outage. Duplicate fills are suppressed
// automatically.
//
// # Order Types
//
// All four broker.OrderType values are supported: Market, Limit, Stop, and
// StopLimit. All seven broker.TimeInForce values are supported. Dollar-amount
// orders (Qty=0, Amount>0) are converted to share quantities by fetching a
// real-time quote.
//
// # Order Groups
//
// TradeStationBroker implements broker.GroupSubmitter for native bracket (BRK)
// and OCO order group support using TradeStation's order groups endpoint.
//
// # Usage
//
//	import "github.com/penny-vault/pvbt/broker/tradestation"
//
//	tsBroker := tradestation.New()
//	eng := engine.New(&MyStrategy{},
//	    engine.WithBroker(tsBroker),
//	)
package tradestation
