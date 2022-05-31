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

package strategies

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/ioutil"
	"main/database"
	"main/strategies/adm"
	"main/strategies/daa"
	"main/strategies/mdep"
	"main/strategies/paa"
	"main/strategies/strategy"
	"math"

	"github.com/pelletier/go-toml/v2"
	log "github.com/sirupsen/logrus"
)

//go:embed **/*.md **/*.toml
var resources embed.FS

// StrategyList List of all strategies
var StrategyList = []*strategy.StrategyInfo{}

// StrategyMap Map of strategies
var StrategyMap = make(map[string]*strategy.StrategyInfo)

// StrategyMetrics map of updated metrics for each strategy - this is used by the StrategyInfo constrcutors and the GetStrategies endpoint
var StrategyMetricsMap = make(map[string]strategy.StrategyMetrics)

// InitializeStrategyMap configure the strategy map
func InitializeStrategyMap() {
	Register("adm", adm.New)
	Register("daa", daa.New)
	Register("paa", paa.New)
	Register("mdep", mdep.New)
}

func Register(strategyPkg string, factory strategy.StrategyFactory) {
	// read description
	fn := fmt.Sprintf("%s/description.md", strategyPkg)
	file, err := resources.Open(fn)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to open file")
	}
	defer file.Close()
	doc, err := ioutil.ReadAll(file)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to read file")
	}
	longDescription := string(doc)

	// load config file
	fn = fmt.Sprintf("%s/strategy.toml", strategyPkg)
	file, err = resources.Open(fn)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to open file")
	}
	defer file.Close()
	doc, err = ioutil.ReadAll(file)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to read file")
	}

	var strat strategy.StrategyInfo
	err = toml.Unmarshal(doc, &strat)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
			"File":  fn,
		}).Error("failed to parse toml file")
	}

	strat.LongDescription = longDescription
	strat.Factory = factory

	StrategyList = append(StrategyList, &strat)
	StrategyMap[strat.Shortcode] = &strat
}

// Ensure all strategies have portfolio entries in the database so metrics are calculated
func LoadStrategyMetricsFromDb() {
	log.Info("refreshing portfolio metrics")
	for ii := range StrategyList {
		strat := StrategyList[ii]

		// load results from the database
		trx, err := database.TrxForUser("pvuser")
		if err != nil {
			log.WithFields(log.Fields{
				"Endpoint": "UpdatePortfolio",
				"Error":    err,
				"UserID":   "pvuser",
			}).Fatal("unable to get database transaction for user")
		}

		row := trx.QueryRow(context.Background(), "SELECT id, cagr_3yr, cagr_5yr, cagr_10yr, std_dev, downside_deviation, max_draw_down, avg_draw_down, sharpe_ratio, sortino_ratio, ulcer_index, ytd_return, cagr_since_inception FROM portfolio WHERE user_id='pvuser' AND name=$1", strat.Name)
		s := strategy.StrategyMetrics{}
		err = row.Scan(&s.ID, &s.CagrThreeYr, &s.CagrFiveYr, &s.CagrTenYr, &s.StdDev, &s.DownsideDeviation, &s.MaxDrawDown, &s.AvgDrawDown, &s.SharpeRatio, &s.SortinoRatio, &s.UlcerIndex, &s.YTDReturn, &s.CagrSinceInception)

		metrics := []*sql.NullFloat64{&s.CagrThreeYr, &s.CagrFiveYr, &s.CagrTenYr, &s.StdDev, &s.DownsideDeviation, &s.MaxDrawDown, &s.AvgDrawDown, &s.SharpeRatio, &s.SortinoRatio, &s.UlcerIndex, &s.YTDReturn, &s.CagrSinceInception}
		for _, m := range metrics {
			if math.IsNaN(m.Float64) || math.IsInf(m.Float64, 0) {
				m.Float64 = 0
				m.Valid = false
			}
		}

		if err != nil {
			log.WithFields(log.Fields{
				"Strategy": strat.Shortcode,
				"Error":    err,
			}).Warn("failed to lookup strategy portfolio in database")
			trx.Rollback(context.Background())
			continue
		}

		StrategyMetricsMap[strat.Shortcode] = s
		strat.Metrics = s
		trx.Commit(context.Background())
	}
}
