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

// Package webull implements broker.Broker for the Webull brokerage using the
// official Webull OpenAPI platform.
//
// # Authentication
//
// The broker supports two authentication models selected automatically by
// environment variables:
//
// Direct API (HMAC-SHA1): set WEBULL_APP_KEY and WEBULL_APP_SECRET. Every
// request is signed with HMAC-SHA1. No token refresh or browser auth needed.
//
// Connect API (OAuth 2.0): set WEBULL_CLIENT_ID and WEBULL_CLIENT_SECRET.
// Uses authorization code flow with browser-based consent. Access tokens are
// refreshed automatically. Optionally set WEBULL_CALLBACK_URL (default
// https://127.0.0.1:5174) and WEBULL_TOKEN_FILE (default
// ~/.pvbt/webull_token.json).
//
// If both are set, Direct API takes priority. Set WEBULL_ACCOUNT_ID to select
// an account; otherwise the first account is used.
//
// # UAT Environment
//
// Use WithUAT() to target Webull's UAT/test environment:
//
//	webullBroker := webull.New(webull.WithUAT())
//
// # Fractional Shares
//
// Use WithFractionalShares() to enable dollar-amount orders:
//
//	webullBroker := webull.New(webull.WithFractionalShares())
//
// # Fill Delivery
//
// Fills are delivered by periodically polling the Webull REST API for order
// status updates. The poller runs with exponential backoff and deduplicates
// fills already delivered, so each fill is reported exactly once. Webull's
// gRPC streaming API is not used.
package webull
