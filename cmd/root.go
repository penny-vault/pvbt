// Copyright 2021 JD Fergason
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

	"github.com/penny-vault/pv-api/common"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Profile bool
var Trace bool

func init() {
	// PV secret key
	viper.BindEnv("secret_key", "PV_SECRET")
	rootCmd.PersistentFlags().String("secret-key", "", "Secret encryption key")
	viper.BindPFlag("secret_key", serveCmd.Flags().Lookup("secret-key"))

	// AUTH0
	viper.BindEnv("auth0.secret", "AUTH0_SECRET")
	rootCmd.PersistentFlags().String("auth0-secret", "", "Auth0 secret")
	viper.BindPFlag("auth0.secret", serveCmd.Flags().Lookup("auth0-secret"))

	viper.BindEnv("auth0.client_id", "AUTH0_CLIENT_ID")
	rootCmd.PersistentFlags().String("auth0-client-id", "", "Auth0 client id")
	viper.BindPFlag("auth0.client_id", serveCmd.Flags().Lookup("auth0-client-id"))

	viper.BindEnv("auth0.domain", "AUTH0_DOMAIN")
	rootCmd.PersistentFlags().String("auth0-domain", "", "Auth0 domain")
	viper.BindPFlag("auth0.domain", serveCmd.Flags().Lookup("auth0-domain"))

	// Database
	viper.BindEnv("database.url", "DATABASE_URL")
	rootCmd.PersistentFlags().String("database-url", "", "PostgreSQL connection string")
	viper.BindPFlag("database.url", serveCmd.Flags().Lookup("database-url"))

	// Logging configuration
	viper.BindEnv("log.level", "PV_LOG_LEVEL")
	rootCmd.PersistentFlags().String("log-level", "warning", "Logging level")
	viper.BindPFlag("log.level", serveCmd.Flags().Lookup("log-level"))

	viper.BindEnv("log.report_caller", "PV_LOG_REPORT_CALLER")
	rootCmd.PersistentFlags().Bool("log-report-caller", false, "Log function name that called log statement")
	viper.BindPFlag("log.report_caller", serveCmd.Flags().Lookup("log-report-caller"))

	viper.BindEnv("log.output", "PV_LOG_OUTPUT")
	rootCmd.PersistentFlags().String("log-output", "stdout", "Write logs to specified output one of: file path, `stdout`, or `stderr`")
	viper.BindPFlag("log.output", serveCmd.Flags().Lookup("log-output"))

	viper.BindEnv("log.loki_url", "LOKI_URL")
	rootCmd.PersistentFlags().String("log-loki-url", "", "Loki server to send log messages to, if blank don't send to Loki")
	viper.BindPFlag("log.loki_url", serveCmd.Flags().Lookup("log-loki-url"))

	rootCmd.PersistentFlags().BoolVar(&Profile, "cpu-profile", false, "Run pprof and save in profile.out")
	rootCmd.PersistentFlags().BoolVar(&Trace, "trace", false, "Trace program execution and save in trace.out")
}

var rootCmd = &cobra.Command{
	Use:     "pvapi",
	Version: common.CurrentVersion.String(),
	Short:   "Penny Vault is a quantitaive investment package",
	Long:    `A fast and flexible quantitative investment platform that can backtest and execute investment strategies.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
