// 🚀 Fiber is an Express inspired web framework written in Go with 💖
// 📌 API Documentation: https://fiber.wiki
// 📝 Github Repository: https://github.com/gofiber/fiber

// Install and configure heroku: https://devcenter.heroku.com/articles/getting-started-with-go#set-up
// You need to read the PORT env from heroku and you need to define the Procfile

// Deploy the app: https://devcenter.heroku.com/articles/getting-started-with-go#deploy-the-app

package main

import (
	"log"
	"main/handler"
	"main/jwks"
	"main/router"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// Create new Fiber instance
	app := fiber.New()

	// Configure CORS
	// cors.Config{
	// 	 AllowOrigins: "http://localhost, https://www.pennyvault.com",
	//	 AllowHeaders: "Origin, Content-Type, Accept",
	// }
	app.Use(cors.New())

	// Configure authentication
	signingKeys := jwks.LoadJWKS()

	// Setup routes
	router.SetupRoutes(app, signingKeys)

	// initialize strategies
	handler.IntializeStrategyMap()

	// Get the PORT from heroku env
	port := os.Getenv("PORT")

	// Verify if heroku provided the port or not
	if os.Getenv("PORT") == "" {
		port = "3000"
	}

	// Start server on http://${heroku-url}:${port}
	log.Fatal(app.Listen(":" + port))
}
