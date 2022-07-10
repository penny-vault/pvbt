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

package backtest

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/goccy/go-json"
)

var (
	ErrStrategyNotFound = errors.New("strategy not found")
)

type Backtest struct {
	PortfolioModel *portfolio.Model
	Performance    *portfolio.Performance
}

func New(ctx context.Context, shortcode string, params map[string]json.RawMessage, benchmark string, startDate time.Time, endDate time.Time, manager *data.Manager) (*Backtest, error) {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "backtest.New")
	defer span.End()

	span.SetAttributes(attribute.KeyValue{
		Key:   "shortcode",
		Value: attribute.StringValue(shortcode),
	})

	strat, ok := strategies.StrategyMap[shortcode]
	if !ok {
		span.SetStatus(codes.Error, "strategy not found")
		return nil, ErrStrategyNotFound
	}

	stratObject, err := strat.Factory(params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not create strategy")
		return nil, err
	}

	pm := portfolio.NewPortfolio(strat.Name, startDate, 10000, manager)
	pm.Portfolio.Benchmark = benchmark

	manager.Begin = startDate
	manager.End = endDate

	pm.Portfolio.StrategyShortcode = shortcode
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not marshal strategy params")
		return nil, err
	}
	pm.Portfolio.StrategyArguments = string(paramsJSON)
	target, predictedAssets, err := stratObject.Compute(ctx, manager)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not compute strategy portfolio")
		return nil, err
	}

	pm.Portfolio.PredictedAssets = portfolio.BuildPredictedHoldings(predictedAssets.TradeDate, predictedAssets.Target, predictedAssets.Justification)
	if err := pm.TargetPortfolio(ctx, target); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		return nil, err
	}

	// calculate the portfolio's performance
	performance := portfolio.NewPerformance(pm.Portfolio)
	err = performance.CalculateThrough(ctx, pm, manager.End)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "calculate portfolio performance failed")
		return nil, err
	}

	backtest := &Backtest{
		PortfolioModel: pm,
		Performance:    performance,
	}
	return backtest, nil
}

// Save the backtest to the Database
func (b *Backtest) Save(userID string, permanent bool) {
	start := time.Now()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("unable to get database transaction for user")
		return
	}

	err = b.PortfolioModel.SaveWithTransaction(trx, userID, permanent)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("could not save portfolio")
		return
	}

	err = b.Performance.SaveWithTransaction(trx, userID)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("could not save performance measurement")
		return
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("could not commit database transaction")
		return
	}

	stop := time.Now()
	saveDur := stop.Sub(start).Round(time.Millisecond)

	if err != nil {
		log.Warn().Stack().Err(err).Msg("failed to save performance measurements to DB")
		return
	}

	log.Info().Dur("Duration", saveDur).Msg("saved to DB")
}
