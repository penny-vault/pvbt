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
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/common"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(backtestCmd)
}

var backtestCmd = &cobra.Command{
	Use:        "backtest [flags] StrategyShortcode StrategyArguments",
	Short:      "Run a backtest of a strategy",
	Args:       cobra.MinimumNArgs(2),
	ArgAliases: []string{"StrategyShortcode", "StrategyArguments"},
	Run: func(cmd *cobra.Command, args []string) {
		// setup database
		err := database.Connect()
		if err != nil {
			log.Panic().Err(err).Msg("could not connect to database")
		}

		// Initialize data framework
		data.InitializeDataManager()
		log.Info().Msg("initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()
		dataManager := data.NewManager()

		strategies.LoadStrategyMetricsFromDb()
		strat := strategies.StrategyMap[args[0]]

		var arguments map[string]json.RawMessage
		err = json.Unmarshal([]byte(args[1]), &arguments)
		if err != nil {
			log.Error().Err(err).Msg("Could not unmarshal json")
			return
		}

		strategy, err := strat.Factory(arguments)
		if err != nil {
			log.Error().Err(err).Msg("Could not create strategy")
			return
		}

		target, predicted, err := strategy.Compute(context.Background(), dataManager)
		if err != nil {
			log.Fatal().Err(err).Msg("Could not compute strategy positions")
		}

		dateSeriesIdx, err := target.NameToColumn(common.DateIdx)
		if err != nil {
			log.Fatal().Err(err).Msg("could not get date index of target portfolio")
		}
		dateIdx := target.Series[dateSeriesIdx]
		startDate := dateIdx.Value(0).(time.Time)

		fmt.Println(target.Table())
		fmt.Printf("Start Date: %s\n", startDate.Format("2006-01-02"))
		fmt.Printf("Next Trade Date: %+v\n", predicted.TradeDate)
		fmt.Printf("Predicted Target: %+v\n", predicted.Target)
		fmt.Printf("Predicted Justification: %+v\n", predicted.Justification)

		pm := portfolio.NewPortfolio("Backtest Portfolio", startDate, 10_000, dataManager)
		fmt.Println("Building portfolio...")
		if err := pm.TargetPortfolio(context.Background(), target); err != nil {
			log.Fatal().Stack().Err(err).Msg("could not invest portfolio")
		}

		fmt.Println("Computing performance metrics...")
		perf := portfolio.NewPerformance(pm.Portfolio)
		if err := perf.CalculateThrough(context.Background(), pm, time.Now()); err != nil {
			log.Fatal().Err(err).Msg("could not calculate portfolio performance")
		}

		for _, meas := range perf.Measurements {
			fmt.Printf("%s\t%.2f\n", meas.Time.Format("2006-01-02"), meas.UlcerIndex)
		}

		fmt.Printf("Ulcer Index Average: %.2f\n", perf.PortfolioMetrics.UlcerIndexAvg)
		fmt.Printf("            P50    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP50)
		fmt.Printf("            P90    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP90)
		fmt.Printf("            P99    : %.2f\n", perf.PortfolioMetrics.UlcerIndexP99)
	},
}
