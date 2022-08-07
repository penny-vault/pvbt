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

package router

import (
	"github.com/penny-vault/pv-api/handler"
	"github.com/penny-vault/pv-api/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/lestrrat-go/jwx/jwk"
)

// SetupRoutes setup router api
func SetupRoutes(app *fiber.App, jwks *jwk.AutoRefresh, jwksURL string) {
	// API
	api := app.Group("/v1", logger.New())
	api.Get("/", handler.Ping)
	api.Get("/apikey", middleware.PVAuth(jwks, jwksURL), handler.GetAPIKey)
	api.Get("/announcements", middleware.PVAuth(jwks, jwksURL), handler.GetAnnouncements)
	api.Get("/activity", middleware.PVAuth(jwks, jwksURL), handler.GetAllActivity)

	// Benchmark
	api.Get("/benchmark/:ticker", middleware.PVAuth(jwks, jwksURL), handler.Benchmark)

	// Portfolio
	portfolio := api.Group("/portfolio")
	portfolio.Get("/", middleware.PVAuth(jwks, jwksURL), handler.ListPortfolios)
	portfolio.Post("/", middleware.PVAuth(jwks, jwksURL), handler.CreatePortfolio)
	portfolio.Get("/:id", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolio)
	portfolio.Patch("/:id", middleware.PVAuth(jwks, jwksURL), handler.UpdatePortfolio)
	portfolio.Delete("/:id", middleware.PVAuth(jwks, jwksURL), handler.DeletePortfolio)
	portfolio.Get("/:id/activity", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolioActivity)
	portfolio.Get("/:id/holdings", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolioHoldings)
	portfolio.Get("/:id/measurements", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolioMeasurements)
	portfolio.Get("/:id/performance", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolioPerformance)
	portfolio.Get("/:id/transactions", middleware.PVAuth(jwks, jwksURL), handler.GetPortfolioTransactions)

	// Strategy
	strategy := api.Group("/strategy")
	strategy.Get("/", middleware.PVAuth(jwks, jwksURL), handler.ListStrategies)
	strategy.Get("/:shortcode", middleware.PVAuth(jwks, jwksURL), handler.GetStrategy)
	strategy.Post("/:shortcode/execute", middleware.PVAuth(jwks, jwksURL), handler.RunStrategy)
}
