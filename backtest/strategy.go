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

package backtest

import (
	"context"
	"encoding/hex"
	"errors"
	"main/data"
	"main/database"
	"main/portfolio"
	"main/strategies"
	"time"

	"github.com/goccy/go-json"
	log "github.com/sirupsen/logrus"
)

type Backtest struct {
	PortfolioModel *portfolio.PortfolioModel
	Performance    *portfolio.Performance
}

func New(shortcode string, params map[string]json.RawMessage, startDate time.Time, endDate time.Time, manager *data.Manager) (*Backtest, error) {
	if strat, ok := strategies.StrategyMap[shortcode]; ok {
		stratObject, err := strat.Factory(params)
		if err != nil {
			log.Warn(err)
			return nil, err
		}

		start := time.Now()
		pm := portfolio.NewPortfolio(strat.Name, startDate, 10000, manager)

		manager.Begin = startDate
		manager.End = endDate

		pm.Portfolio.StrategyShortcode = shortcode
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			log.Warn(err)
			return nil, err
		}
		pm.Portfolio.StrategyArguments = string(paramsJSON)
		target, predictedAssets, err := stratObject.Compute(manager)
		if err != nil {
			log.Warn(err)
			return nil, err
		}
		stop := time.Now()
		stratComputeDur := stop.Sub(start).Round(time.Millisecond)

		pm.Portfolio.PredictedAssets = portfolio.BuildPredictedHoldings(predictedAssets.TradeDate, predictedAssets.Target, predictedAssets.Justification)
		start = time.Now()
		if err := pm.TargetPortfolio(target); err != nil {
			log.Warn(err)
			return nil, err
		}

		stop = time.Now()
		targetPortfolioDur := stop.Sub(start).Round(time.Millisecond)

		// calculate the portfolio's performance
		start = time.Now()
		performance, err := pm.CalculatePerformance(manager.End)
		if err != nil {
			log.Warn(err)
			return nil, err
		}
		stop = time.Now()
		calcPerfDur := stop.Sub(start).Round(time.Millisecond)

		log.WithFields(log.Fields{
			"StratCalcDur":       stratComputeDur,
			"TargetPortfolioDur": targetPortfolioDur,
			"PerfCalcDur":        calcPerfDur,
		}).Info("Backtest runtime performance")

		backtest := &Backtest{
			PortfolioModel: pm,
			Performance:    performance,
		}
		return backtest, nil
	}

	return nil, errors.New("strategy not found")
}

// Save the backtest to the Database
func (b *Backtest) Save(userID string, permanent bool) error {
	start := time.Now()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": hex.EncodeToString(b.PortfolioModel.Portfolio.ID),
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return err
	}

	err = b.PortfolioModel.SaveWithTransaction(trx, userID, permanent)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": hex.EncodeToString(b.PortfolioModel.Portfolio.ID),
			"UserID":      userID,
		}).Error("could not save portfolio")
		return err
	}

	err = b.Performance.SaveWithTransaction(trx, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": hex.EncodeToString(b.PortfolioModel.Portfolio.ID),
			"UserID":      userID,
		}).Error("could not save performance measurements")
		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": hex.EncodeToString(b.PortfolioModel.Portfolio.ID),
			"UserID":      userID,
		}).Error("could not commit database transaction")
		return err
	}

	stop := time.Now()
	saveDur := stop.Sub(start).Round(time.Millisecond)

	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Warn("failed to save performance measurements to DB")
		return err
	}

	log.WithFields(log.Fields{
		"Dur": saveDur,
	}).Info("Saved to DB")

	return nil
}
