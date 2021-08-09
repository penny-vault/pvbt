package backtest

import (
	"context"
	"errors"
	"main/common"
	"main/data"
	"main/database"
	"main/portfolio"
	"main/strategies"
	"time"

	"github.com/goccy/go-json"
	"github.com/shamaton/msgpack/v2"
	log "github.com/sirupsen/logrus"
)

type Backtest struct {
	Portfolio   *portfolio.Portfolio
	Performance *portfolio.Performance
}

func New(shortcode string, params map[string]json.RawMessage, startDate time.Time, endDate time.Time, manager *data.Manager) (*Backtest, error) {
	if strat, ok := strategies.StrategyMap[shortcode]; ok {
		stratObject, err := strat.Factory(params)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		manager.Begin = startDate
		manager.End = endDate

		start := time.Now()
		p := portfolio.NewPortfolio(strat.Name, startDate, 10000, manager)
		target, err := stratObject.Compute(manager)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		stop := time.Now()
		stratComputeDur := stop.Sub(start).Round(time.Millisecond)

		start = time.Now()
		if err := p.TargetPortfolio(target); err != nil {
			log.Println(err)
			return nil, err
		}

		stop = time.Now()
		targetPortfolioDur := stop.Sub(start).Round(time.Millisecond)

		// calculate the portfolio's performance
		start = time.Now()
		performance, err := p.CalculatePerformance(manager.End)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		stop = time.Now()
		calcPerfDur := stop.Sub(start).Round(time.Millisecond)

		log.WithFields(log.Fields{
			"StratCalcDur":       stratComputeDur,
			"TargetPortfolioDur": targetPortfolioDur,
			"PerfCalcDur":        calcPerfDur,
		}).Info("Backtest runtime performance (s)")

		backtest := &Backtest{
			Portfolio:   p,
			Performance: performance,
		}
		return backtest, nil
	}

	return nil, errors.New("strategy not found")
}

// Serialize the backtest to Redis, the Database, and return a msgpack representation
func (b *Backtest) Serialize(userID string) error {
	start := time.Now()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": b.Portfolio.ID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return err
	}

	err = b.Portfolio.SaveWithTransaction(trx, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": b.Portfolio.ID,
			"UserID":      userID,
		}).Error("could not save portfolio")
		return err
	}

	err = b.Performance.SaveWithTransaction(trx, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": b.Portfolio.ID,
			"UserID":      userID,
		}).Error("could not save performance measurements")
		return err
	}

	err = trx.Commit(context.Background())
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": b.Portfolio.ID,
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

	start = time.Now()

	msgpackTransactions, err := msgpack.Marshal(b.Portfolio.Transactions)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("msgpack serialization failed for transactions")
		return err
	}
	err = common.CacheSet(b.Portfolio.ID.String()+":Transactions", msgpackTransactions)
	if err != nil {
		log.Warn(err)
		return err
	}

	msgpackMeasurements, err := msgpack.Marshal(b.Performance.Measurements)
	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("msgpack serialization of measurements failed")
		return err
	}

	err = common.CacheSet(b.Portfolio.ID.String()+":Measurements", msgpackMeasurements)
	if err != nil {
		log.Warn(err)
		return err
	}

	stop = time.Now()
	calcPerfDur := stop.Sub(start).Round(time.Millisecond)

	log.WithFields(log.Fields{
		"PerfCalcDur": calcPerfDur,
	}).Info("strategy serialization / cache")

	return nil
}
