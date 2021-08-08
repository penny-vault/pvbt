package router

import (
	"main/handler"
	"main/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/lestrrat-go/jwx/jwk"
)

// SetupRoutes setup router api
func SetupRoutes(app *fiber.App, jwks *jwk.AutoRefresh, jwksUrl string) {
	// API
	api := app.Group("/v1", logger.New())
	api.Get("/", handler.Ping)
	api.Get("/apikey", middleware.PVAuth(jwks, jwksUrl), handler.ApiKey)

	// Benchmark
	api.Get("/benchmark/:ticker", middleware.PVAuth(jwks, jwksUrl), handler.Benchmark)

	// Portfolio
	portfolio := api.Group("/portfolio")
	portfolio.Get("/", middleware.PVAuth(jwks, jwksUrl), handler.ListPortfolios)
	portfolio.Post("/", middleware.PVAuth(jwks, jwksUrl), handler.CreatePortfolio)
	portfolio.Get("/:id", middleware.PVAuth(jwks, jwksUrl), handler.GetPortfolio)
	portfolio.Patch("/:id", middleware.PVAuth(jwks, jwksUrl), handler.UpdatePortfolio)
	portfolio.Delete("/:id", middleware.PVAuth(jwks, jwksUrl), handler.DeletePortfolio)
	portfolio.Get("/:id/performance", middleware.PVAuth(jwks, jwksUrl), handler.GetPortfolioPerformance)
	portfolio.Get("/:id/measurements", middleware.PVAuth(jwks, jwksUrl), handler.GetPortfolioMeasurements)
	portfolio.Get("/:id/transactions", middleware.PVAuth(jwks, jwksUrl), handler.GetPortfolioTransactions)

	// Strategy
	strategy := api.Group("/strategy")
	strategy.Get("/", middleware.PVAuth(jwks, jwksUrl), handler.ListStrategies)
	strategy.Get("/:shortcode", middleware.PVAuth(jwks, jwksUrl), handler.GetStrategy)
	strategy.Get("/:shortcode/execute", middleware.PVAuth(jwks, jwksUrl), handler.RunStrategy)
}
