package handler

import (
	"github.com/gofiber/fiber/v2"
)

// GetPortfolio get a portfolio
func GetPortfolio(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "Product found"})
}

// ListPortfolios list all portfolios for logged in user
func ListPortfolios(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "Product found"})
}

// CreatePortfolio new portfolio
func CreatePortfolio(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "Product found"})
}

// UpdatePortfolio update portfolio
func UpdatePortfolio(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "Product found"})
}

// DeletePortfolio delete portfolio
func DeletePortfolio(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success", "message": "Product found"})
}
