package router

import (
	"main/handler"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// SetupRoutes setup router api
func SetupRoutes(app *fiber.App) {
	// Middleware
	api := app.Group("/v1", logger.New())
	api.Get("/", handler.Hello)

	// Strategy
	strategy := api.Group("/strategy")
	strategy.Get("/:id", handler.GetStrategy)
	strategy.Get("/", handler.ListStrategies)

	// Portfolio
	portfolio := api.Group("/portfolio")
	portfolio.Get("/:id", handler.GetPortfolio)
	portfolio.Get("/", handler.ListPortfolios)
	portfolio.Post("/", handler.CreatePortfolio)
	portfolio.Patch("/:id", handler.UpdatePortfolio)
	portfolio.Delete("/:id", handler.DeletePortfolio)
}
