package handler

import (
	"main/newrelicapi"
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func Ping(c *fiber.Ctx) error {
	txn := newrelicapi.NewRelicApp.StartTransaction("/Ping")
	reqURL, _ := url.Parse(string(c.Request().URI().FullURI()))
	txn.SetWebRequest(newrelic.WebRequest{
		URL:    reqURL,
		Method: string(c.Request().Header.Method()),
		Host:   string(c.Request().URI().Host()),
	})
	defer txn.End()

	return c.JSON(fiber.Map{"status": "success", "message": "API is alive"})
}
