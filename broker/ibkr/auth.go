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
	"net/http"
)

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
