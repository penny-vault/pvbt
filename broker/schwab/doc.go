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

// Package schwab implements broker.Broker for the Charles Schwab brokerage.
//
// This broker integrates with Schwab's Trader API v1 for order management
// and the WebSocket streamer for real-time fill delivery via ACCT_ACTIVITY
// subscriptions.
//
// # Authentication
//
// Schwab uses OAuth 2.0 with the authorization_code grant type. The broker
// reads credentials from environment variables:
//
//   - SCHWAB_CLIENT_ID: OAuth app client ID (consumer key)
//   - SCHWAB_CLIENT_SECRET: OAuth app client secret
//   - SCHWAB_CALLBACK_URL: Registered callback URL (default: https://127.0.0.1:5174)
//   - SCHWAB_TOKEN_FILE: Path to persist tokens (default: ~/.config/pvbt/schwab-tokens.json)
//   - SCHWAB_ACCOUNT_NUMBER: Plain-text account number to trade (optional; uses first linked account if unset)
//
// On first run, Connect() prints an authorization URL. Open it in a browser,
// log in to Schwab, and authorize the app. The callback server captures the
// tokens automatically. Subsequent runs reuse the stored refresh token until
// it expires (7 days).
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to Schwab's account activity
// streamer. On disconnect, the broker reconnects with exponential backoff and
// polls for any fills missed during the outage. Duplicate fills are suppressed
// automatically.
//
// # Order Types
//
// All four broker.OrderType values are supported: Market, Limit, Stop, and
// StopLimit. Dollar-amount orders (Qty=0, Amount>0) are converted to share
// quantities by fetching a real-time quote.
//
// # Order Groups
//
// SchwabBroker implements broker.GroupSubmitter for native bracket (1st
// Triggers OCO) and OCO order support using Schwab's nested
// childOrderStrategies.
//
// # Usage
//
//	import "github.com/penny-vault/pvbt/broker/schwab"
//
//	schwabBroker := schwab.New()
//	eng := engine.New(&MyStrategy{},
//	    engine.WithBroker(schwabBroker),
//	)
package schwab
