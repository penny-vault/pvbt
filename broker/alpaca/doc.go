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

// Package alpaca implements broker.Broker for the Alpaca brokerage.
//
// # Authentication
//
// The broker reads credentials from environment variables:
//
//   - ALPACA_API_KEY: Alpaca API key ID
//   - ALPACA_API_SECRET: Alpaca API secret key
//
// # Paper Trading
//
// Use WithPaper() to target Alpaca's paper trading environment:
//
//	alpacaBroker := alpaca.New(alpaca.WithPaper())
//
// # Fractional Shares
//
// Use WithFractionalShares() to enable dollar-amount orders using
// Alpaca's notional field instead of computing whole-share quantities:
//
//	alpacaBroker := alpaca.New(alpaca.WithFractionalShares())
//
// # Fill Delivery
//
// Fills are delivered via a WebSocket connection to Alpaca's trade
// updates stream. On disconnect, the broker reconnects with exponential
// backoff and polls for any fills missed during the outage.
//
// # Order Groups
//
// AlpacaBroker implements broker.GroupSubmitter for native bracket and
// OCO order support via Alpaca's order_class parameter.
package alpaca
