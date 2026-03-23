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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const defaultTickleInterval = 55 * time.Second

// Compile-time interface check.
var _ Authenticator = (*gatewayAuthenticator)(nil)

// Authenticator decorates HTTP requests with IB credentials. Two
// implementations are planned: OAuth (signed requests) and Gateway
// (cookie-based Client Portal sessions).
type Authenticator interface {
	// Init performs any one-time setup such as token exchange.
	Init(ctx context.Context) error
	// Decorate adds authentication headers/cookies to the request.
	Decorate(req *http.Request) error
	// Keepalive keeps the session alive (e.g. periodic tickle).
	Keepalive(ctx context.Context)
	// Close releases resources held by the authenticator.
	Close() error
}

// gatewayAuthenticator authenticates via the IB Client Portal Gateway.
// The user must already be logged in through the gateway web UI; this
// authenticator verifies the session and keeps it alive with periodic
// tickle requests.
type gatewayAuthenticator struct {
	httpClient      *resty.Client
	tickleInterval  time.Duration
	cancelKeepalive context.CancelFunc
}

// authStatusResponse represents the JSON response from /iserver/auth/status.
type authStatusResponse struct {
	Authenticated bool `json:"authenticated"`
	Connected     bool `json:"connected"`
}

// newGatewayAuthenticator creates a gateway authenticator pointed at the
// given base URL.
func newGatewayAuthenticator(baseURL string) *gatewayAuthenticator {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(clientTimeout)

	return &gatewayAuthenticator{
		httpClient:     client,
		tickleInterval: defaultTickleInterval,
	}
}

// Init checks whether the gateway session is authenticated. If not, it
// triggers reauthentication and returns ErrNotAuthenticated so the caller
// knows the user must log in via the gateway web UI.
func (ga *gatewayAuthenticator) Init(ctx context.Context) error {
	resp, err := ga.httpClient.R().
		SetContext(ctx).
		Post("/iserver/auth/status")
	if err != nil {
		return fmt.Errorf("gateway auth status check: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("gateway auth status returned HTTP %d", resp.StatusCode())
	}

	var status authStatusResponse
	if decodeErr := json.Unmarshal(resp.Body(), &status); decodeErr != nil {
		return fmt.Errorf("gateway auth status decode: %w", decodeErr)
	}

	if status.Authenticated {
		return nil
	}

	// Session is not authenticated -- trigger reauthentication.
	reauthResp, reauthErr := ga.httpClient.R().
		SetContext(ctx).
		Post("/iserver/reauthenticate")
	if reauthErr != nil {
		return fmt.Errorf("gateway reauthenticate: %w", reauthErr)
	}

	if reauthResp.IsError() {
		return fmt.Errorf("gateway reauthenticate returned HTTP %d", reauthResp.StatusCode())
	}

	return broker.ErrNotAuthenticated
}

// Decorate is a no-op for gateway authentication because the gateway
// uses cookies managed by the HTTP client.
func (ga *gatewayAuthenticator) Decorate(_ *http.Request) error {
	return nil
}

// Keepalive sends POST /tickle at the configured interval to keep the
// gateway session alive. It stops when ctx is cancelled.
func (ga *gatewayAuthenticator) Keepalive(ctx context.Context) {
	ticker := time.NewTicker(ga.tickleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := ga.httpClient.R().
				SetContext(ctx).
				Post("/tickle")
			if err != nil {
				log.Warn().Err(err).Msg("gateway tickle failed")
			}
		}
	}
}

// Close cancels the keepalive goroutine if running and releases resources.
func (ga *gatewayAuthenticator) Close() error {
	if ga.cancelKeepalive != nil {
		ga.cancelKeepalive()
	}

	return nil
}
