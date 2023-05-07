// Copyright 2021-2023
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

package middleware

import (
	"encoding/base64"

	"github.com/penny-vault/pv-api/common"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/jdfergason/jwt/v2"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/rs/zerolog/log"
)

type apiToken struct {
	UserID string `json:"sub"`
}

// JWTAuth instantiate JWT auth middleware
func PVAuth(jwks *jwk.AutoRefresh, jwksURL string) fiber.Handler {
	jwtMiddleware := jwtware.New(jwtware.Config{
		Jwks:         jwks,
		JwksUrl:      jwksURL,
		ErrorHandler: jwtError,
		SuccessHandler: func(c *fiber.Ctx) error {
			return nil
		},
	})

	apiKey := func(c *fiber.Ctx, token string) error {
		if token == "" {
			return c.Status(fiber.StatusBadRequest).SendString("apikey may not be empty")
		}

		tokenBytes, err := base64.URLEncoding.DecodeString(token)
		if err != nil {
			log.Warn().Stack().Err(err).Msg("could not base64 decode apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("could not base64 decode apikey")
		}

		jsonBytes, err := common.Decrypt(tokenBytes)
		if err != nil {
			log.Warn().Stack().Err(err).Msg("could not unencrypt apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}

		var v apiToken
		if err := json.Unmarshal(jsonBytes, &v); err != nil {
			log.Warn().Stack().Err(err).Msg("could not unmarshal json from apikey - maybe apikey is corrupt?")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}
		c.Locals("userID", v.UserID)
		return c.Next()
	}

	return func(c *fiber.Ctx) error {
		token := c.Query("apikey")
		if token != "" {
			return apiKey(c, token)
		}

		if token, ok := c.GetReqHeaders()["X-Pv-Api"]; ok {
			return apiKey(c, token)
		}

		res := jwtMiddleware(c)

		if res != nil {
			return c.SendString(res.Error())
		}

		// store user ID and token in c.Locals
		jwtToken, ok := c.Locals("user").(jwt.Token)
		if !ok {
			return fiber.ErrUnauthorized
		}
		c.Locals("userID", jwtToken.Subject())
		return c.Next()
	}
}

func jwtError(c *fiber.Ctx, err error) error {
	log.Warn().Stack().Err(err).Msg("jwt authentication error")

	if err.Error() == "Missing or malformed JWT" {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"status": "error", "message": "Missing or malformed JWT", "data": nil})
	}
	return c.Status(fiber.StatusUnauthorized).
		JSON(fiber.Map{"status": "error", "message": "Invalid or expired JWT", "data": nil})
}
