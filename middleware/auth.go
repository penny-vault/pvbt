// Copyright 2021 JD Fergason
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

package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"

	"github.com/penny-vault/pv-api/common"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	jwtware "github.com/jdfergason/jwt/v2"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"

	log "github.com/sirupsen/logrus"
)

type apiToken struct {
	userID string
	tiingo string
}

// JWTAuth instantiate JWT auth middleware
func PVAuth(jwks *jwk.AutoRefresh, jwksUrl string) fiber.Handler {
	jwtMiddleware := jwtware.New(jwtware.Config{
		Jwks:         jwks,
		JwksUrl:      jwksUrl,
		ErrorHandler: jwtError,
		SuccessHandler: func(c *fiber.Ctx) error {
			return nil
		},
	})

	apiKey := func(c *fiber.Ctx) error {
		token := c.Query("apikey")
		if token == "" {
			return c.Status(fiber.StatusBadRequest).SendString("apikey may not be empty")
		}

		tokenBytes, err := hex.DecodeString(token)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("could not hex decode apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("could not hex decode apikey")
		}

		unencryptedBytes, err := common.Decrypt(tokenBytes)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("could not unencrypt apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}

		buf := bytes.NewBuffer(unencryptedBytes)
		zr, err := gzip.NewReader(buf)
		if err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("could not ungzip apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}

		jsonBytes := make([]byte, 0, 100)
		if _, err := zr.Read(jsonBytes); err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("could not ungzip apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}

		if err := zr.Close(); err != nil {
			log.WithFields(log.Fields{
				"Error": err,
			}).Warn("could not ungzip apiKey")
			return c.Status(fiber.StatusBadRequest).SendString("invalid apikey")
		}

		var v apiToken
		json.Unmarshal(jsonBytes, &v)
		c.Locals("userID", v.userID)
		c.Locals("tiingoToken", v.tiingo)
		return c.Next()
	}

	return func(c *fiber.Ctx) error {
		token := c.Query("apikey")
		if token != "" {
			return apiKey(c)
		}

		res := jwtMiddleware(c)

		if res != nil {
			return c.SendString(res.Error())
		}

		// store user ID and token in c.Locals
		jwtToken := c.Locals("user").(jwt.Token)
		c.Locals("userID", jwtToken.Subject())

		if tiingoToken, ok := jwtToken.Get(`https://pennyvault.com/tiingo_token`); ok {
			c.Locals("tiingoToken", tiingoToken.(string))
		} else {
			log.WithFields(log.Fields{
				"jwtToken": tiingoToken,
			}).Warn("jwt token does not have expected claim: https://pennyvault.com/tiingo_token")
			return c.Status(fiber.StatusBadRequest).SendString("invalid jwt token")
		}

		return c.Next()
	}
}

func jwtError(c *fiber.Ctx, err error) error {
	log.WithFields(log.Fields{
		"Error": err,
	}).Warn("jwt authentication error")

	if err.Error() == "Missing or malformed JWT" {
		c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"status": "error", "message": "Missing or malformed JWT", "data": nil})
	} else {
		c.Status(fiber.StatusUnauthorized).
			JSON(fiber.Map{"status": "error", "message": "Invalid or expired JWT", "data": nil})
	}

	return err
}
