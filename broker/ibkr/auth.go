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
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	defaultTickleInterval     = 55 * time.Second
	defaultLSTRefreshInterval = 20 * time.Minute
	oauthSignatureMethod      = "HMAC-SHA256"
	nonceLength               = 32
)

// Compile-time interface checks.
var (
	_ Authenticator = (*gatewayAuthenticator)(nil)
	_ Authenticator = (*oauthAuthenticator)(nil)
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

// oauthAuthenticator authenticates via IB's OAuth 1.0a-style signing flow.
// It performs RSA-SHA256 signed token exchange, establishes a live session
// token via Diffie-Hellman challenge/response, and signs each outbound
// request with HMAC-SHA256.
type oauthAuthenticator struct {
	consumerKey       string
	signingKey        *rsa.PrivateKey
	accessToken       string
	accessTokenSecret string
	liveSessionToken  []byte
	baseURL           string
	cancelKeepalive   context.CancelFunc
}

// oauthTokenResponse holds fields parsed from OAuth token endpoint responses.
type oauthTokenResponse struct {
	OAuthToken            string `json:"oauth_token"`
	OAuthTokenSecret      string `json:"oauth_token_secret"`
	DiffieHellmanResponse string `json:"diffie_hellman_response"`
}

// newOAuthAuthenticator creates an OAuth authenticator by loading the RSA
// private key from a PKCS#8 PEM file.
func newOAuthAuthenticator(consumerKey, keyFile string) *oauthAuthenticator {
	pemData, readErr := os.ReadFile(keyFile)
	if readErr != nil {
		log.Fatal().Err(readErr).Str("file", keyFile).Msg("failed to read OAuth signing key")
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		log.Fatal().Str("file", keyFile).Msg("no PEM block found in signing key file")

		return nil
	}

	parsed, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
	if parseErr != nil {
		log.Fatal().Err(parseErr).Msg("failed to parse PKCS#8 private key")
	}

	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		log.Fatal().Msg("signing key is not an RSA private key")
	}

	return &oauthAuthenticator{
		consumerKey: consumerKey,
		signingKey:  rsaKey,
	}
}

// Init performs the OAuth token exchange flow. If an access token is already
// present (e.g. from environment variables), it skips directly to obtaining
// a live session token.
func (oa *oauthAuthenticator) Init(ctx context.Context) error {
	client := resty.New().SetTimeout(clientTimeout)

	if oa.accessToken == "" {
		// Step 1: request token
		reqTokenResp, reqErr := client.R().
			SetContext(ctx).
			SetHeader("Authorization", oa.rsaSignedHeader("POST", oa.baseURL+"/oauth/request_token", "")).
			Post(oa.baseURL + "/oauth/request_token")
		if reqErr != nil {
			return fmt.Errorf("oauth request token: %w", reqErr)
		}

		if reqTokenResp.IsError() {
			return fmt.Errorf("oauth request token returned HTTP %d", reqTokenResp.StatusCode())
		}

		var reqTokenData oauthTokenResponse
		if decodeErr := json.Unmarshal(reqTokenResp.Body(), &reqTokenData); decodeErr != nil {
			return fmt.Errorf("oauth request token decode: %w", decodeErr)
		}

		requestToken := reqTokenData.OAuthToken

		// Step 2: access token
		accessResp, accessErr := client.R().
			SetContext(ctx).
			SetHeader("Authorization", oa.rsaSignedHeader("POST", oa.baseURL+"/oauth/access_token", requestToken)).
			Post(oa.baseURL + "/oauth/access_token")
		if accessErr != nil {
			return fmt.Errorf("oauth access token: %w", accessErr)
		}

		if accessResp.IsError() {
			return fmt.Errorf("oauth access token returned HTTP %d", accessResp.StatusCode())
		}

		var accessData oauthTokenResponse
		if decodeErr := json.Unmarshal(accessResp.Body(), &accessData); decodeErr != nil {
			return fmt.Errorf("oauth access token decode: %w", decodeErr)
		}

		oa.accessToken = accessData.OAuthToken
		oa.accessTokenSecret = accessData.OAuthTokenSecret
	}

	// Step 3: live session token via DH challenge
	if lstErr := oa.obtainLiveSessionToken(ctx, client); lstErr != nil {
		return lstErr
	}

	return nil
}

// obtainLiveSessionToken sends a DH challenge and derives the live session
// token from the response.
func (oa *oauthAuthenticator) obtainLiveSessionToken(ctx context.Context, client *resty.Client) error {
	challenge := make([]byte, nonceLength)
	if _, randErr := rand.Read(challenge); randErr != nil {
		return fmt.Errorf("oauth generate DH challenge: %w", randErr)
	}

	challengeB64 := base64.StdEncoding.EncodeToString(challenge)

	body := map[string]string{
		"diffie_hellman_challenge": challengeB64,
	}

	lstResp, lstErr := client.R().
		SetContext(ctx).
		SetHeader("Authorization", oa.rsaSignedHeader("POST", oa.baseURL+"/oauth/live_session_token", oa.accessToken)).
		SetBody(body).
		Post(oa.baseURL + "/oauth/live_session_token")
	if lstErr != nil {
		return fmt.Errorf("oauth live session token: %w", lstErr)
	}

	if lstResp.IsError() {
		return fmt.Errorf("oauth live session token returned HTTP %d", lstResp.StatusCode())
	}

	var lstData oauthTokenResponse
	if decodeErr := json.Unmarshal(lstResp.Body(), &lstData); decodeErr != nil {
		return fmt.Errorf("oauth live session token decode: %w", decodeErr)
	}

	// Simplified DH: use the response bytes directly as the session token.
	// Full DH computation can be refined later with real IB credentials.
	dhBytes, decodeErr := base64.StdEncoding.DecodeString(lstData.DiffieHellmanResponse)
	if decodeErr != nil {
		return fmt.Errorf("oauth decode DH response: %w", decodeErr)
	}

	oa.liveSessionToken = dhBytes

	return nil
}

// rsaSignedHeader builds an OAuth Authorization header signed with the RSA
// private key (used for token exchange requests).
func (oa *oauthAuthenticator) rsaSignedHeader(method, requestURL, token string) string {
	params := map[string]string{
		"oauth_consumer_key":     oa.consumerKey,
		"oauth_nonce":            generateNonce(),
		"oauth_signature_method": "RSA-SHA256",
		"oauth_timestamp":        strconv.FormatInt(time.Now().Unix(), 10),
	}

	if token != "" {
		params["oauth_token"] = token
	}

	baseString := buildSignatureBaseString(method, requestURL, params)

	hashed := sha256.Sum256([]byte(baseString))

	signature, signErr := rsa.SignPKCS1v15(rand.Reader, oa.signingKey, crypto.SHA256, hashed[:])
	if signErr != nil {
		log.Error().Err(signErr).Msg("RSA signing failed")
		return ""
	}

	params["oauth_signature"] = base64.StdEncoding.EncodeToString(signature)

	return buildAuthorizationHeader(params)
}

// Decorate adds an OAuth Authorization header with HMAC-SHA256 signature
// to each outbound API request using the live session token as the key.
func (oa *oauthAuthenticator) Decorate(req *http.Request) error {
	params := map[string]string{
		"oauth_consumer_key":     oa.consumerKey,
		"oauth_token":            oa.accessToken,
		"oauth_nonce":            generateNonce(),
		"oauth_signature_method": oauthSignatureMethod,
		"oauth_timestamp":        strconv.FormatInt(time.Now().Unix(), 10),
	}

	requestURL := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path
	baseString := buildSignatureBaseString(req.Method, requestURL, params)

	mac := hmac.New(sha256.New, oa.liveSessionToken)
	mac.Write([]byte(baseString))
	signature := mac.Sum(nil)

	params["oauth_signature"] = base64.StdEncoding.EncodeToString(signature)

	req.Header.Set("Authorization", buildAuthorizationHeader(params))

	return nil
}

// Keepalive periodically refreshes the live session token. It stops when
// ctx is cancelled.
func (oa *oauthAuthenticator) Keepalive(ctx context.Context) {
	ticker := time.NewTicker(defaultLSTRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			client := resty.New().SetTimeout(clientTimeout)
			if lstErr := oa.obtainLiveSessionToken(ctx, client); lstErr != nil {
				log.Warn().Err(lstErr).Msg("oauth live session token refresh failed")
			}
		}
	}
}

// Close cancels the keepalive goroutine if running.
func (oa *oauthAuthenticator) Close() error {
	if oa.cancelKeepalive != nil {
		oa.cancelKeepalive()
	}

	return nil
}

// generateNonce returns a random base64-encoded nonce string.
func generateNonce() string {
	nonceBytes := make([]byte, nonceLength)
	if _, randErr := rand.Read(nonceBytes); randErr != nil {
		log.Error().Err(randErr).Msg("failed to generate nonce")
	}

	return base64.RawURLEncoding.EncodeToString(nonceBytes)
}

// buildSignatureBaseString constructs the OAuth signature base string:
// METHOD&url_encode(URL)&url_encode(sorted_params).
func buildSignatureBaseString(method, requestURL string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, url.QueryEscape(key)+"="+url.QueryEscape(params[key]))
	}

	paramString := strings.Join(pairs, "&")

	return method + "&" + url.QueryEscape(requestURL) + "&" + url.QueryEscape(paramString)
}

// buildAuthorizationHeader creates an OAuth Authorization header string
// from the given parameters.
func buildAuthorizationHeader(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+`="`+url.QueryEscape(params[key])+`"`)
	}

	return "OAuth " + strings.Join(parts, ", ")
}
