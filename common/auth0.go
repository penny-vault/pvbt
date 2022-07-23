// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/goccy/go-json"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

var (
	ErrAuth0               = errors.New("cannot get Auth0 Management API access token request")
	ErrAuth0AccountRequest = errors.New("User account request failed")
)

type Token struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type Auth0User struct {
	UserID        string                 `json:"user_id"`
	Name          string                 `json:"name"`
	Email         string                 `json:"email"`
	EmailVerified bool                   `json:"email_verified"`
	UserMetaData  map[string]interface{} `json:"user_metadata"`
}

var userMap = make(map[string]Auth0User)

func getToken() (string, error) {
	domain := viper.GetString("auth0.domain")
	clientID := viper.GetString("auth0.client_id")
	secret := viper.GetString("auth0.secret")

	subLog := log.With().Str("ClientID", clientID).Str("Domain", domain).Logger()

	url := fmt.Sprintf("https://%s/oauth/token", domain)
	bodyStr := fmt.Sprintf(`grant_type=client_credentials&client_id=%s&client_secret=%s&audience=https://%s/api/v2/`, clientID, secret, domain)
	body := strings.NewReader(bodyStr)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		subLog.Error().Err(err).Msg("cannot build Auth0 Management API access token request")
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		subLog.Error().Err(err).Msg("Cannot get Auth0 Management API access token request")
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// NOTE: It's save to ignore the err return here
		// as we are just formatting an error message for the
		// already errored out HTTP response
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			subLog.Error().Err(err).Int("StatusCode", resp.StatusCode).Msg("error reading response body")
		}
		subLog.Error().Err(err).Int("StatusCode", resp.StatusCode).Bytes("Body", respBody).Msg("Cannot get Auth0 Management API access token request")
		return "", ErrAuth0
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		subLog.Error().Err(err).Int("StatusCode", resp.StatusCode).Bytes("Body", respBody).Msg("Failed to read body response when retrieving Auth0 Management API access token")
		return "", ErrAuth0
	}

	managementToken := &Token{}
	err = json.Unmarshal(respBody, managementToken)
	if err != nil {
		subLog.Error().Err(err).Int("StatusCode", resp.StatusCode).Bytes("Body", respBody).Msg("Failed to convert json Auth0 Management API access token")
		return "", ErrAuth0
	}

	return managementToken.AccessToken, nil
}

func GetAuth0User(userID string) (*Auth0User, error) {
	// Check if user is already in cache
	if u, ok := userMap[userID]; ok {
		return &u, nil
	}

	// User hasn't been loaded yet, request from identity provider
	domain := viper.GetString("auth0.domain")
	token, err := getToken()
	if err != nil {
		return nil, err
	}

	subLog := log.With().Str("UserID", userID).Str("Domain", domain).Logger()
	subLog.Info().Msg("Requesting user from auth0")

	encodedUserID := url.QueryEscape(userID)
	url := fmt.Sprintf("https://%s/api/v2/users/%s", domain, encodedUserID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		subLog.Error().Err(err).Msg("Could not create Auth0 user request")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		subLog.Error().Err(err).Msg("User account request failed")
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		subLog.Error().Int("StatusCode", resp.StatusCode).Str("Body", string(respBody)).Msg("User account request failed")
		return nil, ErrAuth0AccountRequest
	}

	respBody, _ := ioutil.ReadAll(resp.Body)
	auth0User := Auth0User{}
	err = json.Unmarshal(respBody, &auth0User)
	if err != nil {
		subLog.Error().Err(err).Str("Body", string(respBody)).Msg("Could not decode user response")
	}

	userMap[userID] = auth0User
	return &auth0User, nil
}
