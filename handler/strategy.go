package handler

import "github.com/gofiber/fiber/v2"

// ListStrategies get a list of all strategies
func ListStrategies(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "API is alive", "data": nil})
}

// GetStrategy get configuration for a specific strategy
func GetStrategy(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "API is alive", "data": nil})
}
