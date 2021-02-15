package handler

import (
	"main/newrelicapi"

	"github.com/gofiber/fiber/v2"
)

func Ping(c *fiber.Ctx) error {
	txn := newrelicapi.StartTransaction(c)
	defer txn.End()

	return c.JSON(fiber.Map{"status": "success", "message": "API is alive"})
}
