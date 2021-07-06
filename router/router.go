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
	// Middleware
	api := app.Group("/v1", logger.New())
	api.Get("/", handler.Ping)
	api.Get("/benchmark/:ticker", middleware.JWTAuth(jwks, jwksUrl), handler.Benchmark)

	// Portfolio
	portfolio := api.Group("/portfolio")
	portfolio.Get("/", middleware.JWTAuth(jwks, jwksUrl), handler.ListPortfolios)
	portfolio.Post("/", middleware.JWTAuth(jwks, jwksUrl), handler.CreatePortfolio)
	portfolio.Get("/:id", middleware.JWTAuth(jwks, jwksUrl), handler.GetPortfolio)
	portfolio.Patch("/:id", middleware.JWTAuth(jwks, jwksUrl), handler.UpdatePortfolio)
	portfolio.Delete("/:id", middleware.JWTAuth(jwks, jwksUrl), handler.DeletePortfolio)
	portfolio.Get("/:id/performance", middleware.JWTAuth(jwks, jwksUrl), handler.GetPortfolioPerformance)

	// Strategy
	strategy := api.Group("/strategy")
	strategy.Get("/", middleware.JWTAuth(jwks, jwksUrl), handler.ListStrategies)
	strategy.Get("/:shortcode", middleware.JWTAuth(jwks, jwksUrl), handler.GetStrategy)
	strategy.Get("/:shortcode/execute", middleware.JWTAuth(jwks, jwksUrl), handler.RunStrategy)
}
