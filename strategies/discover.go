// Copyright 2021-2023
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strategies

import (
	"context"
	"embed"
	"fmt"
	"io"
	"math"

	"github.com/jackc/pgtype"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/strategies/adm"
	"github.com/penny-vault/pv-api/strategies/daa"
	"github.com/penny-vault/pv-api/strategies/mdep"
	"github.com/penny-vault/pv-api/strategies/paa"
	"github.com/penny-vault/pv-api/strategies/seek"
	"github.com/penny-vault/pv-api/strategies/strategy"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog/log"
)

//go:embed **/*.md **/*.toml
var resources embed.FS

// StrategyList List of all strategies
var StrategyList = []*strategy.Info{}

// StrategyMap Map of strategies
var StrategyMap = make(map[string]*strategy.Info)

// StrategyMetrics map of updated metrics for each strategy - this is used by the StrategyInfo constrcutors and the GetStrategies endpoint
var StrategyMetricsMap = make(map[string]strategy.Metrics)

// InitializeStrategyMap configure the strategy map
func InitializeStrategyMap() {
	Register("adm", adm.New)
	Register("daa", daa.New)
	Register("mdep", mdep.New)
	Register("paa", paa.New)
	Register("seek", seek.New)
}

func Register(strategyPkg string, factory strategy.Factory) {
	// read description
	fn := fmt.Sprintf("%s/description.md", strategyPkg)
	subLog := log.With().Str("FileName", fn).Logger()
	file, err := resources.Open(fn)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to open file")
	}
	defer file.Close()
	doc, err := io.ReadAll(file)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to read file")
	}
	longDescription := string(doc)

	// load config file
	fn = fmt.Sprintf("%s/strategy.toml", strategyPkg)
	subLog = log.With().Str("FileName", fn).Logger()
	file, err = resources.Open(fn)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to open file")
	}
	defer file.Close()
	doc, err = io.ReadAll(file)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to read file")
	}

	var strat strategy.Info
	err = toml.Unmarshal(doc, &strat)
	if err != nil {
		subLog.Error().Stack().Err(err).Msg("failed to parse toml file")
	}

	strat.LongDescription = longDescription
	strat.Factory = factory

	StrategyList = append(StrategyList, &strat)
	StrategyMap[strat.Shortcode] = &strat
}

// Ensure all strategies have portfolio entries in the database so metrics are calculated
func LoadStrategyMetricsFromDb() {
	ctx := context.Background()
	for ii := range StrategyList {
		strat := StrategyList[ii]
		log.Info().Str("StrategyName", strat.Name).Msg("Refresh metrics for strategy")

		// load results from the database
		trx, err := database.TrxForUser(ctx, "pvuser")
		if err != nil {
			log.Panic().Str("Endpoint", "UpdatePortfolio").Str("UserID", "pvuser").Msg("unable to get database transaction for user")
		}

		row := trx.QueryRow(context.Background(), "SELECT id, cagr_3yr, cagr_5yr, cagr_10yr, std_dev, downside_deviation, max_draw_down, avg_draw_down, sharpe_ratio, sortino_ratio, ulcer_index, ytd_return, cagr_since_inception FROM portfolios WHERE user_id='pvuser' AND name=$1", strat.Name)
		s := strategy.Metrics{}
		err = row.Scan(&s.ID, &s.CagrThreeYr, &s.CagrFiveYr, &s.CagrTenYr, &s.StdDev, &s.DownsideDeviation, &s.MaxDrawDown, &s.AvgDrawDown, &s.SharpeRatio, &s.SortinoRatio, &s.UlcerIndex, &s.YTDReturn, &s.CagrSinceInception)
		if err != nil {
			log.Warn().Stack().Err(err).
				Str("Strategy", strat.Shortcode).
				Msg("failed to lookup strategy portfolio in database")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transaction")
			}
			continue
		}

		for _, m := range []*pgtype.Float4{&s.CagrThreeYr, &s.CagrFiveYr, &s.CagrTenYr, &s.StdDev, &s.DownsideDeviation, &s.MaxDrawDown, &s.AvgDrawDown, &s.SharpeRatio, &s.SortinoRatio, &s.UlcerIndex} {
			if m.Status == pgtype.Present && (math.IsNaN(float64(m.Float)) || math.IsInf(float64(m.Float), 0)) {
				m.Float = 0
				m.Status = pgtype.Null
			}
		}

		for _, m := range []*pgtype.Float8{&s.YTDReturn, &s.CagrSinceInception} {
			if m.Status == pgtype.Present && (math.IsNaN(float64(m.Float)) || math.IsInf(float64(m.Float), 0)) {
				m.Float = 0
				m.Status = pgtype.Null
			}
		}

		StrategyMetricsMap[strat.Shortcode] = s
		strat.Metrics = s
		if err := trx.Commit(context.Background()); err != nil {
			log.Error().Stack().Err(err).Msg("could not commit trx to database")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Stack().Err(err).Msg("could not rollback transactions")
			}
		}
	}
	log.Info().Msg("Finished loading portfolio metrics")
}
