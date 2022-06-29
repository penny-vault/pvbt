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
	"github.com/penny-vault/pv-api/pkginfo"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Profile bool
var Trace bool

func init() {
	// PV secret key
	if err := viper.BindEnv("secret_key", "PV_SECRET"); err != nil {
		log.Panic().Err(err).Msg("could not bind PV_SECRET")
	}
	rootCmd.PersistentFlags().String("secret-key", "", "Secret encryption key")
	if err := viper.BindPFlag("secret_key", serveCmd.Flags().Lookup("secret-key")); err != nil {
		log.Panic().Err(err).Msg("could not bind secret-key")
	}

	// AUTH0
	if err := viper.BindEnv("auth0.secret", "AUTH0_SECRET"); err != nil {
		log.Panic().Err(err).Msg("could not bind PV_SECRET")
	}
	rootCmd.PersistentFlags().String("auth0-secret", "", "Auth0 secret")
	if err := viper.BindPFlag("auth0.secret", serveCmd.Flags().Lookup("auth0-secret")); err != nil {
		log.Panic().Err(err).Msg("could not bind auth0-secret")
	}

	if err := viper.BindEnv("auth0.client_id", "AUTH0_CLIENT_ID"); err != nil {
		log.Panic().Err(err).Msg("could not bind AUTH0_CLIENT_ID")
	}
	rootCmd.PersistentFlags().String("auth0-client-id", "", "Auth0 client id")
	if err := viper.BindPFlag("auth0.client_id", serveCmd.Flags().Lookup("auth0-client-id")); err != nil {
		log.Panic().Err(err).Msg("could not bind auth0-client-id")
	}

	if err := viper.BindEnv("auth0.domain", "AUTH0_DOMAIN"); err != nil {
		log.Panic().Err(err).Msg("could not bind AUTH0_DOMAIN")
	}
	rootCmd.PersistentFlags().String("auth0-domain", "", "Auth0 domain")
	if err := viper.BindPFlag("auth0.domain", serveCmd.Flags().Lookup("auth0-domain")); err != nil {
		log.Panic().Err(err).Msg("could not bind auth0-domain")
	}

	// Database
	if err := viper.BindEnv("database.url", "DATABASE_URL"); err != nil {
		log.Panic().Err(err).Msg("could not bind DATABASE_URL")
	}
	rootCmd.PersistentFlags().String("database-url", "", "PostgreSQL connection string")
	if err := viper.BindPFlag("database.url", serveCmd.Flags().Lookup("database-url")); err != nil {
		log.Panic().Err(err).Msg("could not bind database-url")
	}

	// Logging configuration
	if err := viper.BindEnv("log.level", "PV_LOG_LEVEL"); err != nil {
		log.Panic().Err(err).Msg("could not bind PV_LOG_LEVEL")
	}
	rootCmd.PersistentFlags().String("log-level", "warning", "Logging level")
	if err := viper.BindPFlag("log.level", serveCmd.Flags().Lookup("log-level")); err != nil {
		log.Panic().Err(err).Msg("could not bind log-level")
	}

	if err := viper.BindEnv("log.report_caller", "PV_LOG_REPORT_CALLER"); err != nil {
		log.Panic().Err(err).Msg("could not bind PV_LOG_REPORT_CALLER")
	}
	rootCmd.PersistentFlags().Bool("log-report-caller", false, "Log function name that called log statement")
	if err := viper.BindPFlag("log.report_caller", serveCmd.Flags().Lookup("log-report-caller")); err != nil {
		log.Panic().Err(err).Msg("could not bind log-report-caller")
	}

	if err := viper.BindEnv("log.output", "PV_LOG_OUTPUT"); err != nil {
		log.Panic().Err(err).Msg("could not bind PV_LOG_OUTPUT")
	}
	rootCmd.PersistentFlags().String("log-output", "stdout", "Write logs to specified output one of: file path, `stdout`, or `stderr`")
	if err := viper.BindPFlag("log.output", serveCmd.Flags().Lookup("log-output")); err != nil {
		log.Panic().Err(err).Msg("could not bind log-output")
	}

	if err := viper.BindEnv("log.loki_url", "LOKI_URL"); err != nil {
		log.Panic().Err(err).Msg("could not bind LOKI_URL")
	}
	rootCmd.PersistentFlags().String("log-loki-url", "", "Loki server to send log messages to, if blank don't send to Loki")
	if err := viper.BindPFlag("log.loki_url", serveCmd.Flags().Lookup("log-loki-url")); err != nil {
		log.Panic().Err(err).Msg("could not bind log-loki-url")
	}

	if err := viper.BindEnv("log.otlp_url", "OTLP_URL"); err != nil {
		log.Panic().Err(err).Msg("could not bind OTLP_URL")
	}
	rootCmd.PersistentFlags().String("log-otlp-url", "", "OTLP server to send traces to, if blank don't send traces")
	if err := viper.BindPFlag("log.otlp_url", serveCmd.Flags().Lookup("log-otlp-url")); err != nil {
		log.Panic().Err(err).Msg("could not bind log-otlp-url")
	}

	rootCmd.PersistentFlags().BoolVar(&Profile, "cpu-profile", false, "Run pprof and save in profile.out")
	rootCmd.PersistentFlags().BoolVar(&Trace, "trace", false, "Trace program execution and save in trace.out")
}

var rootCmd = &cobra.Command{
	Use:     "pvapi",
	Version: pkginfo.Version,
	Short:   "Penny Vault is a quantitative investment package",
	Long:    `A fast and flexible quantitative investment platform that can backtest and execute investment strategies.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Panic().Err(err).Msg("rootCmd.Execute returned an error")
	}
}
