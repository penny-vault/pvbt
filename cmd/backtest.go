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
	"encoding/json"
	"fmt"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/penny-vault/pv-api/tradecron"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
			log.Fatal(err)
		}

		tradecron.InitializeTradeCron()

		// Initialize data framework
		data.InitializeDataManager()
		log.Info("Initialized data framework")

		// initialize strategies
		strategies.InitializeStrategyMap()

		credentials := make(map[string]string)
		credentials["tiingo"] = viper.GetString("tiingo.system_token")
		dataManager := data.NewManager(credentials)

		strategies.LoadStrategyMetricsFromDb()
		strat := strategies.StrategyMap[args[0]]

		var arguments map[string]json.RawMessage
		err = json.Unmarshal([]byte(args[1]), &arguments)
		if err != nil {
			log.Error("Could not unmarshal json")
			log.Error(err)
			return
		}

		strategy, err := strat.Factory(arguments)
		if err != nil {
			log.Error("Could not create strategy")
			log.Error(err)
			return
		}

		target, predicted, err := strategy.Compute(context.Background(), &dataManager)
		if err != nil {
			log.Error("Could not compute strategy positions")
			log.Error(err)
			return
		}

		fmt.Println(target.Table())
		fmt.Printf("Next Trade Date: %+v\n", predicted.TradeDate)
		fmt.Printf("Predicted Target: %+v\n", predicted.Target)
		fmt.Printf("Predicted Justification: %+v\n", predicted.Justification)
	},
}
