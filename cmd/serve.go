package cmd

import (
	"main/common"
	"main/data"
	"main/database"
	"main/jwks"
	"main/loki"
	"main/middleware"
	"main/router"
	"main/strategies"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
)

func init() {
	viper.BindEnv("server.port", "PORT")
	serveCmd.Flags().IntP("port", "p", 3000, "Port to run application server on")
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))

	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the pv-api server",
	Long:  `Run HTTP server that implements the Penny Vault API`,
	Run: func(cmd *cobra.Command, args []string) {
		common.SetupLogging()
		common.SetupCache()
		loki_url := viper.GetString("log.loki_url")
		if loki_url != "" {
			loki.Init()
		}
		log.Info("Initialized logging")

		// setup database
		err := database.Connect()
		if err != nil {
			log.Fatal(err)
		}

		// Initialize data framework
		data.InitializeDataManager()
		log.Info("Initialized data framework")

		// Create new Fiber instance
		app := fiber.New()

		// Configure CORS
		corsConfig := cors.Config{
			AllowOrigins: "http://localhost:8080, https://www.pennyvault.com, https://beta.pennyvault.com",
			AllowHeaders: "*",
			AllowMethods: "GET,POST,HEAD,PUT,DELETE,PATCH",
		}
		app.Use(cors.New(corsConfig))

		// Setup logging middleware
		app.Use(middleware.NewLogger())

		// Configure authentication
		jwksAutoRefresh, jwksUrl := jwks.SetupJWKS()

		// Setup routes
		router.SetupRoutes(app, jwksAutoRefresh, jwksUrl)

		// initialize strategies
		strategies.InitializeStrategyMap()

		// Get strategy metrics
		tz, _ := time.LoadLocation("America/New_York") // New York is the reference time
		scheduler := gocron.NewScheduler(tz)
		scheduler.Every(1).Hours().Do(strategies.LoadStrategyMetricsFromDb)
		scheduler.StartAsync()

		// Start server on http://${heroku-url}:${port}
		log.Fatal(app.Listen(":" + viper.GetString("server.port")))
	},
}
