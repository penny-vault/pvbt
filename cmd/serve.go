// Copyright 2021-2022
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/jwks"
	"github.com/penny-vault/pv-api/loki"
	"github.com/penny-vault/pv-api/middleware"
	"github.com/penny-vault/pv-api/router"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/tradecron"

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
		if Profile {
			f, err := os.Create("profile.out")
			if err != nil {
				log.Fatal(err)
			}
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}

		if Trace {
			f, err := os.Create("trace.out")
			if err != nil {
				log.Fatalf("failed to create trace output file: %v", err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Fatalf("failed to close trace file: %v", err)
				}
			}()

			if err := trace.Start(f); err != nil {
				log.Fatalf("failed to start trace: %v", err)
			}
			defer trace.Stop()
		}

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

		tradecron.InitializeTradeCron()

		// Initialize data framework
		data.InitializeDataManager()
		log.Info("Initialized data framework")

		// Create new Fiber instance
		app := fiber.New()

		// shutdown cleanly on interrupt
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			sig := <-c // block until signal is read
			fmt.Printf("Received signal: '%s'; shutting down...\n", sig.String())
			err = app.Shutdown()
			if err != nil {
				log.Fatal(err)
			}
		}()

		// Configure CORS
		corsConfig := cors.Config{
			AllowOrigins: "http://localhost:8080, https://www.pennyvault.com, https://beta.pennyvault.com",
			AllowHeaders: "Accept, Accept-CH, Accept-Charset, Accept-Datetime, Accept-Encoding, Accept-Ext, Accept-Features, Accept-Language, Accept-Params, Accept-Ranges, Access-Control-Allow-Credentials, Access-Control-Allow-Headers, Access-Control-Allow-Methods, Access-Control-Allow-Origin, Access-Control-Expose-Headers, Access-Control-Max-Age, Access-Control-Request-Headers, Access-Control-Request-Method, Age, Allow, Alternates, Authentication-Info, Authorization, C-Ext, C-Man, C-Opt, C-PEP, C-PEP-Info, CONNECT, Cache-Control, Compliance, Connection, Content-Base, Content-Disposition, Content-Encoding, Content-ID, Content-Language, Content-Length, Content-Location, Content-MD5, Content-Range, Content-Script-Type, Content-Security-Policy, Content-Style-Type, Content-Transfer-Encoding, Content-Type, Content-Version, Cookie, Cost, DAV, DELETE, DNT, DPR, Date, Default-Style, Delta-Base, Depth, Derived-From, Destination, Differential-ID, Digest, ETag, Expect, Expires, Ext, From, GET, GetProfile, HEAD, HTTP-date, Host, IM, If, If-Match, If-Modified-Since, If-None-Match, If-Range, If-Unmodified-Since, Keep-Alive, Label, Last-Event-ID, Last-Modified, Link, Location, Lock-Token, MIME-Version, Man, Max-Forwards, Media-Range, Message-ID, Meter, Negotiate, Non-Compliance, OPTION, OPTIONS, OWS, Opt, Optional, Ordering-Type, Origin, Overwrite, P3P, PEP, PICS-Label, POST, PUT, Pep-Info, Permanent, Position, Pragma, ProfileObject, Protocol, Protocol-Query, Protocol-Request, Proxy-Authenticate, Proxy-Authentication-Info, Proxy-Authorization, Proxy-Features, Proxy-Instruction, Public, RWS, Range, Referer, Refresh, Resolution-Hint, Resolver-Location, Retry-After, Safe, Sec-Websocket-Extensions, Sec-Websocket-Key, Sec-Websocket-Origin, Sec-Websocket-Protocol, Sec-Websocket-Version, Security-Scheme, Server, Set-Cookie, Set-Cookie2, SetProfile, SoapAction, Status, Status-URI, Strict-Transport-Security, SubOK, Subst, Surrogate-Capability, Surrogate-Control, TCN, TE, TRACE, Timeout, Title, Trailer, Transfer-Encoding, UA-Color, UA-Media, UA-Pixels, UA-Resolution, UA-Windowpixels, URI, Upgrade, User-Agent, Variant-Vary, Vary, Version, Via, Viewport-Width, WWW-Authenticate, Want-Digest, Warning, Width, X-Content-Duration, X-Content-Security-Policy, X-Content-Type-Options, X-CustomHeader, X-DNSPrefetch-Control, X-Forwarded-For, X-Forwarded-Port, X-Forwarded-Proto, X-Frame-Options, X-Modified, X-OTHER, X-PING, X-PINGOTHER, X-Powered-By, X-Requested-With",
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
		err = app.Listen(":" + viper.GetString("server.port"))
		if err != nil {
			log.Fatal(err)
		}
	},
}
