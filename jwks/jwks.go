// Copyright 2021-2022
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

package jwks

import (
	"context"
	"fmt"

	"github.com/lestrrat-go/jwx/jwk"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// LoadJWKS retrieves JWKS from auth0 domain
func SetupJWKS() (*jwk.AutoRefresh, string) {
	// read remote JWKS
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks.json", viper.GetString("auth0.domain"))

	log.Debug().Str("Url", jwksURL).Msg("reading JWKS")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ar := jwk.NewAutoRefresh(ctx)
	ar.Configure(jwksURL)
	if _, err := ar.Fetch(ctx, jwksURL); err != nil {
		log.Panic().Err(err).Msg("could not fetch jwks from auth0")
	}

	return ar, jwksURL
}
