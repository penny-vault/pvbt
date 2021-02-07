// 🚀 Fiber is an Express inspired web framework written in Go with 💖
// 📌 API Documentation: https://fiber.wiki
// 📝 Github Repository: https://github.com/gofiber/fiber

// Install and configure heroku: https://devcenter.heroku.com/articles/getting-started-with-go#set-up
// You need to read the PORT env from heroku and you need to define the Procfile

// Deploy the app: https://devcenter.heroku.com/articles/getting-started-with-go#deploy-the-app

package main

import (
	"main/database"
	"main/jwks"
	"main/router"
	"main/strategies"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/github"

	log "github.com/sirupsen/logrus"
)

// @title Penny Vault Investment API
// @version 1.0
// @description Execute investment strategies
// @license.name Commercial
// @BasePath /
func main() {
	// setup database
	err := database.SetupDatabaseMigrations()
	if err != nil {
		log.Fatal(err)
	}
	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Create new Fiber instance
	app := fiber.New()

	// Configure CORS
	corsConfig := cors.Config{
		AllowOrigins: "http://localhost:8080, https://www.pennyvault.com",
		AllowHeaders: "*",
		AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH",
	}
	app.Use(cors.New(corsConfig))

	// Configure authentication
	signingKeys := jwks.LoadJWKS()

	// Setup routes
	router.SetupRoutes(app, signingKeys)

	// initialize strategies
	strategies.InitializeStrategyMap()

	// Get the PORT from heroku env
	port := os.Getenv("PORT")

	// Verify if heroku provided the port or not
	if os.Getenv("PORT") == "" {
		port = "3000"
	}

	// Start server on http://${heroku-url}:${port}
	log.Fatal(app.Listen(":" + port))
}
