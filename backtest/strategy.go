// Copyright 2021-2022
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

package backtest

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/goccy/go-json"
	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"
	"github.com/penny-vault/pv-api/portfolio"
	"github.com/penny-vault/pv-api/strategies"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrStrategyNotFound = errors.New("strategy not found")
)

type Backtest struct {
	PortfolioModel *portfolio.Model
	Performance    *portfolio.Performance
}

func New(ctx context.Context, shortcode string, params map[string]json.RawMessage, benchmark *data.Security, startDate time.Time, endDate time.Time) (*Backtest, error) {
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
		log.Error().Err(err).Msg("could not create strategy")
		return nil, err
	}

	pm := portfolio.NewPortfolio(strat.Name, startDate, 10000)
	pm.Portfolio.Benchmark = benchmark.CompositeFigi

	pm.Portfolio.StrategyShortcode = shortcode
	paramsJSON, err := json.MarshalContext(ctx, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not marshal strategy params")
		log.Error().Err(err).Msg("could not marshal strategy params")
		return nil, err
	}
	pm.Portfolio.StrategyArguments = string(paramsJSON)
	target, predictedAssets, err := stratObject.Compute(ctx, startDate, endDate)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not compute strategy portfolio")
		log.Error().Err(err).Msg("could not compute strategy portfolio")
		return nil, err
	}

	pm.Portfolio.PredictedAssets = portfolio.BuildPredictedHoldings(predictedAssets.Date, predictedAssets.Members, predictedAssets.Justifications)
	if err := pm.TargetPortfolio(ctx, target); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invest target portfolio failed")
		log.Error().Err(err).Msg("invest target portfolio failed")
		return nil, err
	}

	// calculate the portfolio's performance
	performance := portfolio.NewPerformance(pm.Portfolio)
	err = performance.CalculateThrough(ctx, pm, endDate)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "calculate portfolio performance failed")
		log.Error().Err(err).Msg("calculate portfolio performance failed")
		return nil, err
	}

	backtest := &Backtest{
		PortfolioModel: pm,
		Performance:    performance,
	}
	return backtest, nil
}

// Save the backtest to the Database
func (b *Backtest) Save(ctx context.Context, userID string, permanent bool) {
	start := time.Now()
	trx, err := database.TrxForUser(ctx, userID)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("unable to get database transaction for user")
		return
	}

	err = b.PortfolioModel.SaveWithTransaction(ctx, trx, userID, permanent)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("could not save portfolio")
		return
	}

	err = b.Performance.SaveWithTransaction(ctx, trx, userID)
	if err != nil {
		log.Error().Stack().Err(err).Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(b.PortfolioModel.Portfolio.ID)).Msg("could not save performance measurement")
		return
	}

	err = trx.Commit(ctx)
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
