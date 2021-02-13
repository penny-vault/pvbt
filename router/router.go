package router

import (
	"main/handler"
	"main/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// SetupRoutes setup router api
func SetupRoutes(app *fiber.App, jwks map[string]interface{}) {
	// Middleware
	api := app.Group("/v1", logger.New())
	api.Get("/", handler.Ping)

	// Strategy
	strategy := api.Group("/strategy")
	strategy.Get("/:id", middleware.JWTAuth(jwks), handler.GetStrategy)
	strategy.Get("/", middleware.JWTAuth(jwks), handler.ListStrategies)
	strategy.Post("/:id", middleware.JWTAuth(jwks), handler.RunStrategy)

	// Portfolio
	portfolio := api.Group("/portfolio")
	portfolio.Get("/:id", middleware.JWTAuth(jwks), handler.GetPortfolio)
	portfolio.Get("/", middleware.JWTAuth(jwks), handler.ListPortfolios)
	portfolio.Post("/", middleware.JWTAuth(jwks), handler.CreatePortfolio)
	portfolio.Patch("/:id", middleware.JWTAuth(jwks), handler.UpdatePortfolio)
	portfolio.Delete("/:id", middleware.JWTAuth(jwks), handler.DeletePortfolio)
}
