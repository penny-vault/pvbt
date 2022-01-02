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

package portfolio

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"main/database"
	"math"
	"sort"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	log "github.com/sirupsen/logrus"
)

// METHODS

// CalculatePerformance calculates various performance metrics of portfolio
func (pm *PortfolioModel) CalculatePerformance(through time.Time) (*Performance, error) {
	p := pm.Portfolio
	if len(p.Transactions) == 0 {
		return nil, errors.New("cannot calculate performance for portfolio with no transactions")
	}

	perf := Performance{
		PortfolioID: p.ID,
		PeriodStart: p.StartDate,
		PeriodEnd:   through,
		ComputedOn:  time.Now(),
	}

	// Calculate performance
	pm.dataProxy.Begin = p.StartDate
	pm.dataProxy.End = through
	pm.dataProxy.Frequency = data.FrequencyDaily

	// t1 := time.Now()
	tradingDays := pm.dataProxy.TradingDays(p.StartDate, through)
	// t2 := time.Now()

	trxIdx := 0
	numTrxs := len(p.Transactions)
	holdings := make(map[string]float64)
	nYears := int(math.Ceil(through.Sub(p.StartDate).Hours() / (24 * 365)))
	perf.Measurements = make([]*PerformanceMeasurement, 0, 252*nYears)
	var prevVal float64 = -1
	today := time.Now()
	currYear := today.Year()
	bestYearPort := AnnualReturn{
		Year:   0,
		Return: -99999,
	}
	worstYearPort := AnnualReturn{
		Year:   0,
		Return: 99999,
	}
	bestYearBenchmark := AnnualReturn{
		Year:   0,
		Return: -99999,
	}
	worstYearBenchmark := AnnualReturn{
		Year:   0,
		Return: 99999,
	}
	prevDate := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	prevMeasurement := PerformanceMeasurement{}
	var totalVal float64
	var currYearStartValue float64 = -1
	var riskFreeValue float64 = 0

	depositedToDate := 0.0
	withdrawnToDate := 0.0
	benchmarkValue := 10_000.0
	benchmarkShares := 0.0

	stratGrowth := 10_000.0
	benchGrowth := 10_000.0
	riskFreeGrowth := 10_000.0

	daysToStartOfWeek := uint(0)
	daysToStartOfMonth := uint(0)
	daysToStartOfYear := uint(0)

	ytdBench := float32(0.0)

	var last time.Time
	var lastAssets []*ReportableHolding

	for _, date := range tradingDays {
		if last.Weekday() > date.Weekday() {
			daysToStartOfWeek = 1
		} else {
			daysToStartOfWeek++
		}

		if last.Month() != date.Month() {
			daysToStartOfMonth = 1
		} else {
			daysToStartOfMonth++
		}

		if last.Year() != date.Year() {
			daysToStartOfYear = 1
		} else {
			daysToStartOfYear++
		}

		last = date

		if benchmarkShares != 0.0 {
			benchmarkPrice, err := pm.dataProxy.Get(date, data.MetricAdjustedClose, pm.Portfolio.Benchmark)
			if err != nil {
				log.Error(err)
			}
			benchmarkValue = benchmarkShares * benchmarkPrice
		}

		// check if this is the current year
		if date.Year() == currYear && currYearStartValue == -1.0 {
			currYearStartValue = prevVal
		}

		// update holdings?
		for ; trxIdx < numTrxs; trxIdx++ {
			trx := p.Transactions[trxIdx]

			// process transactions up to this point in time
			// test if date is Before the trx.Date - if it is then break
			if date.Before(trx.Date) {
				break
			}

			if trx.Kind == DepositTransaction || trx.Kind == WithdrawTransaction {
				switch trx.Kind {
				case DepositTransaction:
					depositedToDate += trx.TotalValue
					riskFreeValue += trx.TotalValue
				case WithdrawTransaction:
					withdrawnToDate += trx.TotalValue
					riskFreeValue -= trx.TotalValue
				}
				continue
			}

			shares := 0.0
			if val, ok := holdings[trx.Ticker]; ok {
				shares = val
			}
			switch trx.Kind {
			case DepositTransaction:
				depositedToDate += trx.TotalValue
				riskFreeValue += trx.TotalValue
				benchmarkValue += trx.TotalValue
				if val, ok := holdings["$CASH"]; ok {
					holdings["$CASH"] = val + trx.TotalValue
				} else {
					holdings["$CASH"] = trx.TotalValue
				}
				continue
			case WithdrawTransaction:
				withdrawnToDate += trx.TotalValue
				riskFreeValue -= trx.TotalValue
				benchmarkValue -= trx.TotalValue
				if val, ok := holdings["$CASH"]; ok {
					holdings["$CASH"] = val - trx.TotalValue
				}
				continue
			case BuyTransaction:
				shares += trx.Shares
				if val, ok := holdings["$CASH"]; ok {
					holdings["$CASH"] = val - trx.TotalValue
				}
				log.Debugf("on %s buy %.2f shares of %s for %.2f @ %.2f per share", trx.Date, trx.Shares, trx.Ticker, trx.TotalValue, trx.PricePerShare)
			case DividendTransaction:
				if val, ok := holdings["$CASH"]; ok {
					holdings["$CASH"] = val + trx.TotalValue
				} else {
					holdings["$CASH"] = trx.TotalValue
				}
				log.Debugf("on %s, %s released a dividend and the portfolio gained $%.2f", trx.Date, trx.Ticker, trx.TotalValue)
				continue
			case SplitTransaction:
				shares = trx.Shares
				log.Debugf("on %s, %s split and %.5f shares were added", trx.Date, trx.Ticker, trx.Shares)
			case SellTransaction:
				shares -= trx.Shares
				if val, ok := holdings["$CASH"]; ok {
					holdings["$CASH"] = val + trx.TotalValue
				} else {
					holdings["$CASH"] = trx.TotalValue
				}
				log.Debugf("on %s sell %.2f shares of %s for %.2f @ %.2f per share", trx.Date, trx.Shares, trx.Ticker, trx.TotalValue, trx.PricePerShare)
			default:
				log.Debugf("on %s unrecognized transaction of type %s", trx.Date, trx.Kind)
				return nil, errors.New("unrecognized transaction type")
			}

			if val, ok := holdings["$CASH"]; ok {
				if val <= 1.0e-5 {
					delete(holdings, "$CASH")
				}
			}

			// Protect against floating point noise
			if shares <= 1.0e-5 {
				shares = 0
			}

			if shares == 0 {
				delete(holdings, trx.Ticker)
			} else {
				holdings[trx.Ticker] = shares
			}
		}

		log.Debugf("Date: %s", date)
		for ticker, holding := range holdings {
			log.Debugf("\tHolding: %s = %.2f", ticker, holding)
		}

		// build justification array
		justificationArray := pm.justifications[date.String()]

		// update benchmarkShares to reflect any new deposits or withdrawals
		benchmarkPrice, err := pm.dataProxy.Get(date, data.MetricAdjustedClose, pm.Portfolio.Benchmark)
		if err != nil {
			log.Error(err)
		}
		if math.IsNaN(benchmarkPrice) {
			log.Warnf("Benchmark %s is NaN", pm.Portfolio.Benchmark)
		}
		benchmarkShares = benchmarkValue / benchmarkPrice

		// iterate through each holding and add value to get total return
		totalVal = 0.0
		for symbol, qty := range holdings {
			if symbol == "$CASH" {
				if math.IsNaN(qty) {
					log.Warn("Cash position is NaN")
				} else {
					totalVal += qty
				}
			} else {
				price, err := pm.dataProxy.Get(date, data.MetricClose, symbol)
				if err != nil {
					return nil, fmt.Errorf("no quote for symbol: %s", symbol)
				}
				if math.IsNaN(price) {
					price, err = pm.dataProxy.GetLatestDataBefore(symbol, data.MetricClose, date)
					//log.Warnf("Price is NaN for %s; last price = %.2f", symbol, price)
					if err != nil {
						return nil, fmt.Errorf("no quote for symbol: %s", symbol)
					}
				}

				totalVal += price * qty
			}
		}

		// this is done as a second loop because we need totalVal to be set for
		// percent calculation
		currentAssets := make([]*ReportableHolding, 0, len(holdings))
		for symbol, qty := range holdings {
			var value float64
			if symbol == "$CASH" {
				value = qty
			} else if qty > 1.0e-5 {
				price, err := pm.dataProxy.Get(date, data.MetricClose, symbol)
				if err != nil {
					return nil, fmt.Errorf("no quote for symbol: %s", symbol)
				}
				value = price * qty
			}
			if math.IsNaN(value) {
				value = 0.0
			}
			if qty > 1.0e-5 {
				currentAssets = append(currentAssets, &ReportableHolding{
					Ticker:           symbol,
					Shares:           qty,
					PercentPortfolio: float32(value / totalVal),
					Value:            value,
				})
			}
		}

		if lastAssets == nil {
			lastAssets = currentAssets
		}

		// update riskFreeValue
		rawRate := pm.dataProxy.RiskFreeRate(date)
		riskFreeRate := rawRate / 100.0 / 252.0
		riskFreeValue *= (1 + riskFreeRate)

		prevVal = totalVal

		// ensure that holdings are sorted
		sort.Slice(lastAssets, func(i, j int) bool {
			return lastAssets[i].Ticker < lastAssets[j].Ticker
		})

		measurement := PerformanceMeasurement{
			Time:                 date,
			Justification:        justificationArray,
			Value:                totalVal,
			BenchmarkValue:       benchmarkValue,
			RiskFreeValue:        riskFreeValue,
			StrategyGrowthOf10K:  stratGrowth,
			BenchmarkGrowthOf10K: benchGrowth,
			RiskFreeGrowthOf10K:  riskFreeGrowth,
			Holdings:             lastAssets,
			TotalDeposited:       depositedToDate,
			TotalWithdrawn:       withdrawnToDate,
		}

		lastAssets = currentAssets
		perf.Measurements = append(perf.Measurements, &measurement)

		if len(perf.Measurements) >= 2 {
			// Growth of 10k
			stratRate := perf.TWRR(1, STRATEGY)
			if !math.IsNaN(stratRate) {
				stratGrowth *= (1.0 + stratRate)
			}
			measurement.StrategyGrowthOf10K = stratGrowth

			benchRate := perf.TWRR(1, BENCHMARK)
			if !math.IsNaN(benchRate) {
				benchGrowth *= (1.0 + benchRate)
			}
			measurement.BenchmarkGrowthOf10K = benchGrowth

			rfRate := perf.TWRR(1, RISKFREE)
			if !math.IsNaN(rfRate) {
				riskFreeGrowth *= (1.0 + rfRate)
			}
			measurement.RiskFreeGrowthOf10K = riskFreeGrowth

			// time-weighted rate of return
			if int(daysToStartOfYear) == len(perf.Measurements) {
				daysToStartOfYear -= 1
			}

			measurement.TWRROneDay = float32(perf.TWRR(1, STRATEGY))
			measurement.TWRRWeekToDate = float32(perf.TWRR(daysToStartOfWeek, STRATEGY))
			measurement.TWRROneWeek = float32(perf.TWRR(5, STRATEGY))
			measurement.TWRRMonthToDate = float32(perf.TWRR(daysToStartOfMonth, STRATEGY))
			measurement.TWRROneMonth = float32(perf.TWRR(21, STRATEGY))
			measurement.TWRRThreeMonth = float32(perf.TWRR(63, STRATEGY))
			measurement.TWRRYearToDate = float32(perf.TWRR(daysToStartOfYear, STRATEGY))
			measurement.TWRROneYear = float32(perf.TWRR(252, STRATEGY))
			measurement.TWRRThreeYear = float32(perf.TWRR(756, STRATEGY))
			measurement.TWRRFiveYear = float32(perf.TWRR(1260, STRATEGY))
			measurement.TWRRTenYear = float32(perf.TWRR(2520, STRATEGY))

			// money-weighted rate of return
			measurement.MWRROneDay = float32(perf.MWRR(1, STRATEGY))
			measurement.MWRRWeekToDate = float32(perf.MWRR(daysToStartOfWeek, STRATEGY))
			measurement.MWRROneWeek = float32(perf.MWRR(5, STRATEGY))
			measurement.MWRRMonthToDate = float32(perf.MWRR(daysToStartOfMonth, STRATEGY))
			measurement.MWRROneMonth = float32(perf.MWRR(21, STRATEGY))
			measurement.MWRRThreeMonth = float32(perf.MWRR(63, STRATEGY))
			measurement.MWRRYearToDate = float32(perf.MWRR(daysToStartOfYear, STRATEGY))
			measurement.MWRROneYear = float32(perf.MWRR(252, STRATEGY))
			measurement.MWRRThreeYear = float32(perf.MWRR(756, STRATEGY))
			measurement.MWRRFiveYear = float32(perf.MWRR(1260, STRATEGY))
			measurement.MWRRTenYear = float32(perf.MWRR(2520, STRATEGY))

			// active return
			measurement.ActiveReturnOneYear = float32(perf.ActiveReturn(252))
			measurement.ActiveReturnThreeYear = float32(perf.ActiveReturn(756))
			measurement.ActiveReturnFiveYear = float32(perf.ActiveReturn(1260))
			measurement.ActiveReturnTenYear = float32(perf.ActiveReturn(2520))

			// alpha
			measurement.AlphaOneYear = float32(perf.Alpha(252))
			measurement.AlphaThreeYear = float32(perf.Alpha(756))
			measurement.AlphaFiveYear = float32(perf.Alpha(1260))
			measurement.AlphaTenYear = float32(perf.Alpha(2520))

			// beta
			measurement.BetaOneYear = float32(perf.Beta(252))
			measurement.BetaThreeYear = float32(perf.Beta(756))
			measurement.BetaFiveYear = float32(perf.Beta(1260))
			measurement.BetaTenYear = float32(perf.Beta(2520))

			// ratios
			measurement.CalmarRatio = float32(perf.CalmarRatio(756, STRATEGY))             // 3 year lookback
			measurement.DownsideDeviation = float32(perf.DownsideDeviation(756, STRATEGY)) // 3 year lookback
			measurement.InformationRatio = float32(perf.InformationRatio(756))             // 3 year lookback
			measurement.KRatio = float32(perf.KRatio(756))                                 // 3 year lookback
			measurement.KellerRatio = float32(perf.KellerRatio(756, STRATEGY))             // 3 year lookback
			measurement.SharpeRatio = float32(perf.SharpeRatio(756, STRATEGY))             // 1 year lookback
			measurement.SortinoRatio = float32(perf.SortinoRatio(756, STRATEGY))           // 1 year lookback
			measurement.StdDev = float32(perf.StdDev(63, STRATEGY))                        // 3 month lookback
			measurement.TreynorRatio = float32(perf.TreynorRatio(756))                     // 3 year lookback
			measurement.UlcerIndex = float32(perf.UlcerIndex())
		}

		if prevDate.Year() != date.Year() {
			if prevMeasurement.TWRRYearToDate > bestYearPort.Return {
				bestYearPort.Return = prevMeasurement.TWRRYearToDate
				bestYearPort.Year = uint16(prevDate.Year())
			}

			if prevMeasurement.TWRRYearToDate < worstYearPort.Return {
				worstYearPort.Return = prevMeasurement.TWRRYearToDate
				worstYearPort.Year = uint16(prevDate.Year())
			}

			// calculate 1-yr benchmark rate of return
			if ytdBench > bestYearBenchmark.Return {
				bestYearBenchmark.Return = ytdBench
				bestYearBenchmark.Year = uint16(prevDate.Year())
			}

			if ytdBench < worstYearBenchmark.Return {
				worstYearBenchmark.Return = ytdBench
				worstYearBenchmark.Year = uint16(prevDate.Year())
			}
		}

		ytdBench = float32(perf.TWRR(daysToStartOfYear, BENCHMARK))
		prevMeasurement = measurement
		prevDate = date

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAssets = currentAssets
		}
	}

	sinceInceptionPeriods := uint(len(perf.Measurements) - 1)
	perf.DrawDowns = perf.Top10DrawDowns(sinceInceptionPeriods, STRATEGY)

	perf.PortfolioReturns = &Returns{
		MWRRSinceInception: perf.MWRR(sinceInceptionPeriods, STRATEGY),
		MWRROneYear:        perf.MWRR(252, STRATEGY),
		MWRRThreeYear:      perf.MWRR(756, STRATEGY),
		MWRRFiveYear:       perf.MWRR(1260, STRATEGY),
		MWRRTenYear:        perf.MWRR(2520, STRATEGY),

		TWRRSinceInception: perf.TWRR(sinceInceptionPeriods, STRATEGY),
		TWRROneYear:        perf.TWRR(252, STRATEGY),
		TWRRThreeYear:      perf.TWRR(756, STRATEGY),
		TWRRFiveYear:       perf.TWRR(1260, STRATEGY),
		TWRRTenYear:        perf.TWRR(2520, STRATEGY),
	}

	perf.BenchmarkReturns = &Returns{
		MWRRSinceInception: perf.MWRR(sinceInceptionPeriods, BENCHMARK),
		MWRROneYear:        perf.MWRR(252, BENCHMARK),
		MWRRThreeYear:      perf.MWRR(756, BENCHMARK),
		MWRRFiveYear:       perf.MWRR(1260, BENCHMARK),
		MWRRTenYear:        perf.MWRR(2520, BENCHMARK),

		TWRRSinceInception: perf.TWRR(sinceInceptionPeriods, BENCHMARK),
		TWRROneYear:        perf.TWRR(252, BENCHMARK),
		TWRRThreeYear:      perf.TWRR(756, BENCHMARK),
		TWRRFiveYear:       perf.TWRR(1260, BENCHMARK),
		TWRRTenYear:        perf.TWRR(2520, BENCHMARK),
	}

	perf.PortfolioMetrics = &Metrics{
		AlphaSinceInception:             perf.Alpha(sinceInceptionPeriods),
		AvgDrawDown:                     perf.AverageDrawDown(sinceInceptionPeriods, STRATEGY),
		BetaSinceInception:              perf.Beta(sinceInceptionPeriods),
		BestYear:                        &bestYearPort,
		DownsideDeviationSinceInception: perf.DownsideDeviation(sinceInceptionPeriods, STRATEGY),
		ExcessKurtosisSinceInception:    perf.ExcessKurtosis(sinceInceptionPeriods),
		FinalBalance:                    perf.Measurements[len(perf.Measurements)-1].Value,
		SharpeRatioSinceInception:       perf.SharpeRatio(sinceInceptionPeriods, STRATEGY),
		Skewness:                        perf.Skew(sinceInceptionPeriods, STRATEGY),
		SortinoRatioSinceInception:      perf.SortinoRatio(sinceInceptionPeriods, STRATEGY),
		StdDevSinceInception:            perf.StdDev(sinceInceptionPeriods, STRATEGY),
		TotalDeposited:                  perf.Measurements[len(perf.Measurements)-1].TotalDeposited,
		TotalWithdrawn:                  perf.Measurements[len(perf.Measurements)-1].TotalWithdrawn,
		UlcerIndexAvg:                   perf.AvgUlcerIndex(sinceInceptionPeriods),
		UlcerIndexP50:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .5),
		UlcerIndexP90:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .9),
		UlcerIndexP99:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .99),
		WorstYear:                       &worstYearPort,
	}

	perf.BenchmarkMetrics = &Metrics{
		AlphaSinceInception:             math.NaN(), // alpha doesn't make sense for benchmark
		AvgDrawDown:                     perf.AverageDrawDown(sinceInceptionPeriods, BENCHMARK),
		BestYear:                        &bestYearBenchmark,
		BetaSinceInception:              math.NaN(),
		DownsideDeviationSinceInception: perf.DownsideDeviation(sinceInceptionPeriods, BENCHMARK),
		ExcessKurtosisSinceInception:    perf.ExcessKurtosis(sinceInceptionPeriods),
		FinalBalance:                    perf.Measurements[len(perf.Measurements)-1].BenchmarkValue,
		SharpeRatioSinceInception:       perf.SharpeRatio(sinceInceptionPeriods, BENCHMARK),
		Skewness:                        perf.Skew(sinceInceptionPeriods, BENCHMARK),
		SortinoRatioSinceInception:      perf.SortinoRatio(sinceInceptionPeriods, BENCHMARK),
		StdDevSinceInception:            perf.StdDev(sinceInceptionPeriods, BENCHMARK),
		TotalDeposited:                  math.NaN(),
		TotalWithdrawn:                  math.NaN(),
		WorstYear:                       &worstYearBenchmark,
	}

	monthlyRets := perf.monthlyReturns(sinceInceptionPeriods, STRATEGY)
	bootstrap := CircularBootstrap(monthlyRets, 12, 5000, 360)
	perf.PortfolioMetrics.DynamicWithdrawalRateSinceInception = DynamicWithdrawalRate(bootstrap, 0.03)
	perf.PortfolioMetrics.PerpetualWithdrawalRateSinceInception = PerpetualWithdrawalRate(bootstrap, 0.03)
	perf.PortfolioMetrics.SafeWithdrawalRateSinceInception = SafeWithdrawalRate(bootstrap, 0.03)

	if currYearStartValue <= 0 {
		perf.PortfolioReturns.MWRRYTD = 0.0
		perf.PortfolioReturns.TWRRYTD = 0.0
		perf.BenchmarkReturns.MWRRYTD = 0.0
		perf.BenchmarkReturns.TWRRYTD = 0.0
	} else {
		perf.PortfolioReturns.MWRRYTD = perf.MWRRYtd(STRATEGY)
		perf.PortfolioReturns.TWRRYTD = perf.TWRRYtd(STRATEGY)
		perf.BenchmarkReturns.MWRRYTD = perf.MWRRYtd(BENCHMARK)
		perf.BenchmarkReturns.TWRRYTD = perf.TWRRYtd(BENCHMARK)
	}
	// t8 := time.Now()

	/*
		log.WithFields(log.Fields{
			"QuoteDownload":          t2.Sub(t1).Round(time.Millisecond),
			"BenchmarkDownload":      t4.Sub(t3).Round(time.Millisecond),
			"QuoteMerge":             t6.Sub(t5).Round(time.Millisecond),
			"PerformanceCalculation": t8.Sub(t7).Round(time.Millisecond),
		}).Info("CalculatePerformance runtime")
	*/

	return &perf, nil
}

// DATABASE

// LOAD

func LoadPerformanceFromDB(portfolioID uuid.UUID, userID string) (*Performance, error) {
	portfolioSQL := `SELECT performance_json FROM portfolio_v1 WHERE id=$1 AND user_id=$2`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return nil, err
	}

	binaryID, err := portfolioID.MarshalBinary()
	if err != nil {
		return nil, err
	}
	p := Performance{
		PortfolioID: binaryID,
	}
	err = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID).Scan(&p)

	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("query database for performance failed")
		trx.Rollback(context.Background())
		return nil, err
	}

	if err := p.loadMeasurementsFromDB(trx, userID); err != nil {
		// logged from loadMeasurementsFromDB
		trx.Rollback(context.Background())
		return nil, err
	}

	if err := trx.Commit(context.Background()); err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": portfolioID,
			"UserID":      userID,
		}).Warn("commit transaction failed")
		return nil, err
	}
	return &p, nil
}

// loadMeasurementsFromDB populates the measurements array with values from the database
func (p *Performance) loadMeasurementsFromDB(trx pgx.Tx, userID string) error {
	measurementSQL := "SELECT extract(epoch from event_date), value, risk_free_value, holdings, percent_return FROM portfolio_measurement_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY event_date"
	rows, err := trx.Query(context.Background(), measurementSQL, p.PortfolioID, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.PortfolioID,
			"UserID":      userID,
			"Query":       measurementSQL,
		}).Warn("failed executing measurement query")
		trx.Rollback(context.Background())
		return err
	}

	measurements := make([]*PerformanceMeasurement, 0, 1000)
	for rows.Next() {
		m := PerformanceMeasurement{}
		err := rows.Scan(&m.Time, &m.Value, &m.RiskFreeValue, &m.Holdings)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":       err,
				"PortfolioID": p.PortfolioID,
				"UserID":      userID,
				"Query":       measurementSQL,
			}).Warn("failed to scan PerformanceMeasurement row in DB query")
			trx.Rollback(context.Background())
			return err
		}
		measurements = append(measurements, &m)
	}
	p.Measurements = measurements
	return nil
}

// SAVE

func (p *Performance) Save(userID string) error {
	trx, err := database.TrxForUser(userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.PortfolioID,
			"UserID":      userID,
		}).Error("unable to get database transaction for user")
		return err
	}

	err = p.SaveWithTransaction(trx, userID)
	if err != nil {
		log.WithFields(log.Fields{
			"Error":       err,
			"PortfolioID": p.PortfolioID,
			"UserID":      userID,
		}).Error("unable to save portfolio transactions")
		trx.Rollback(context.Background())
		return err
	}

	err = trx.Commit(context.Background())
	return err
}

func (p *Performance) SaveWithTransaction(trx pgx.Tx, userID string) error {
	sql := `UPDATE portfolio_v1 SET
		performance_bytes=$2,
		ytd_return=$3,
		cagr_since_inception=$4,
		cagr_3yr=$5,
		cagr_5yr=$6,
		cagr_10yr=$7,
		std_dev=$8,
		downside_deviation=$9,
		max_draw_down=$10,
		avg_draw_down=$11,
		sharpe_ratio=$12,
		sortino_ratio=$13,
		ulcer_index=$14
	WHERE id=$1`

	tmp := p.Measurements
	p.Measurements = make([]*PerformanceMeasurement, 0)
	raw, err := p.MarshalBinary()
	if err != nil {
		trx.Rollback(context.Background())
		return err
	}
	p.Measurements = tmp

	maxDrawDown := 0.0
	if len(p.DrawDowns) > 0 {
		maxDrawDown = p.DrawDowns[0].LossPercent
	}

	_, err = trx.Exec(context.Background(), sql,
		p.PortfolioID,
		raw,
		p.PortfolioReturns.TWRRYTD,
		p.PortfolioReturns.TWRRSinceInception,
		p.PortfolioReturns.TWRRThreeYear,
		p.PortfolioReturns.TWRRFiveYear,
		p.PortfolioReturns.TWRRTenYear,
		p.PortfolioMetrics.StdDevSinceInception,
		p.PortfolioMetrics.DownsideDeviationSinceInception,
		maxDrawDown,
		p.PortfolioMetrics.AvgDrawDown,
		p.PortfolioMetrics.SharpeRatioSinceInception,
		p.PortfolioMetrics.SortinoRatioSinceInception,
		p.PortfolioMetrics.UlcerIndexAvg)
	if err != nil {
		trx.Rollback(context.Background())
		return err
	}

	err = p.saveMeasurements(trx, userID)
	if err != nil {
		trx.Rollback(context.Background())
		return err
	}

	return nil
}

func (p *Performance) saveMeasurements(trx pgx.Tx, userID string) error {
	sql := `INSERT INTO portfolio_measurement_v1 (
		event_date,
		portfolio_id,
		risk_free_value,
		total_deposited_to_date,
		total_withdrawn_to_date,
		user_id,
		strategy_value,
		holdings,
		justification,
		alpha_1yr,
		alpha_3yr,
		alpha_5yr,
		alpha_10yr,
		beta_1yr,
		beta_3yr,
		beta_5yr,
		beta_10yr,
		twrr_1d,
		twrr_1wk,
		twrr_1mo,
		twrr_3mo,
		twrr_1yr,
		twrr_3yr,
		twrr_5yr,
		twrr_10yr,
		mwrr_1d,
		mwrr_1wk,
		mwrr_1mo,
		mwrr_3mo,
		mwrr_1yr,
		mwrr_3yr,
		mwrr_5yr,
		mwrr_10yr,
		active_return_1yr,
		active_return_3yr,
		active_return_5yr,
		active_return_10yr,
		calmar_ratio,
		downside_deviation,
		information_ratio,
		k_ratio,
		keller_ratio,
		sharpe_ratio,
		sortino_ratio,
		std_dev,
		treynor_ratio,
		ulcer_index,
		benchmark_value,
		strategy_growth_of_10k,
		benchmark_growth_of_10k,
		risk_free_growth_of_10k,
		twrr_wtd,
		twrr_mtd,
		twrr_ytd,
		mwrr_wtd,
		mwrr_mtd,
		mwrr_ytd
	) VALUES (
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7,
		$8,
		$9,
		$10,
		$11,
		$12,
		$13,
		$14,
		$15,
		$16,
		$17,
		$18,
		$19,
		$20,
		$21,
		$22,
		$23,
		$24,
		$25,
		$26,
		$27,
		$28,
		$29,
		$30,
		$31,
		$32,
		$33,
		$34,
		$35,
		$36,
		$37,
		$38,
		$39,
		$40,
		$41,
		$42,
		$43,
		$44,
		$45,
		$46,
		$47,
		$48,
		$49,
		$50,
		$51,
		$52,
		$53,
		$54,
		$55,
		$56,
		$57
	) ON CONFLICT ON CONSTRAINT portfolio_measurement_v1_pkey
	DO UPDATE SET
		risk_free_value=$3,
		total_deposited_to_date=$4,
		total_withdrawn_to_date=$5,
		strategy_value=$7,
		holdings=$8,
		justification=$9,
		alpha_1yr=$10,
		alpha_3yr=$11,
		alpha_5yr=$12,
		alpha_10yr=$13,
		beta_1yr=$14,
		beta_3yr=$15,
		beta_5yr=$16,
		beta_10yr=$17,
		twrr_1d=$18,
		twrr_1wk=$19,
		twrr_1mo=$20,
		twrr_3mo=$21,
		twrr_1yr=$22,
		twrr_3yr=$23,
		twrr_5yr=$24,
		twrr_10yr=$25,
		mwrr_1d=$26,
		mwrr_1wk=$27,
		mwrr_1mo=$28,
		mwrr_3mo=$29,
		mwrr_1yr=$30,
		mwrr_3yr=$31,
		mwrr_5yr=$32,
		mwrr_10yr=$33,
		active_return_1yr=$34,
		active_return_3yr=$35,
		active_return_5yr=$36,
		active_return_10yr=$37,
		calmar_ratio=$38,
		downside_deviation=$39,
		information_ratio=$40,
		k_ratio=$41,
		keller_ratio=$42,
		sharpe_ratio=$43,
		sortino_ratio=$44,
		std_dev=$45,
		treynor_ratio=$46,
		ulcer_index=$47,
		benchmark_value=$48,
		strategy_growth_of_10k=$49,
		benchmark_growth_of_10k=$50,
		risk_free_growth_of_10k=$51,
		twrr_wtd=$52,
		twrr_mtd=$53,
		twrr_ytd=$54,
		mwrr_wtd=$55,
		mwrr_mtd=$56,
		mwrr_ytd=$57`

	for _, m := range p.Measurements {
		holdings, err := json.Marshal(m.Holdings)
		if err != nil {
			for _, holding := range m.Holdings {
				log.Error(fmt.Sprintf("[%s] %.2f shares (%.2f%%) = $%.2f", holding.Ticker, holding.Shares, holding.PercentPortfolio, holding.Value))
			}
			return fmt.Errorf("failed to serialize holdings: %s", err)
		}

		justification, err := json.Marshal(m.Justification)
		if err != nil {
			log.Debug(fmt.Sprintf("%+v", m.Justification))
			return fmt.Errorf("failed to serialize justification: %s", err)
		}

		_, err = trx.Exec(context.Background(), sql,
			m.Time,                  // 1
			p.PortfolioID,           // 2
			m.RiskFreeValue,         // 3
			m.TotalDeposited,        // 4
			m.TotalWithdrawn,        // 5
			userID,                  // 6
			m.Value,                 // 7
			holdings,                // 8
			justification,           // 9
			m.AlphaOneYear,          // 10
			m.AlphaThreeYear,        // 11
			m.AlphaFiveYear,         // 12
			m.AlphaTenYear,          // 13
			m.BetaOneYear,           // 14
			m.BetaThreeYear,         // 15
			m.BetaFiveYear,          // 16
			m.BetaTenYear,           // 17
			m.TWRROneDay,            // 18
			m.TWRROneWeek,           // 19
			m.TWRROneMonth,          // 20
			m.TWRRThreeMonth,        // 21
			m.TWRROneYear,           // 22
			m.TWRRThreeYear,         // 23
			m.TWRRFiveYear,          // 24
			m.TWRRTenYear,           // 25
			m.MWRROneDay,            // 26
			m.MWRROneWeek,           // 27
			m.MWRROneMonth,          // 28
			m.MWRRThreeMonth,        // 29
			m.MWRROneYear,           // 30
			m.MWRRThreeYear,         // 31
			m.MWRRFiveYear,          // 32
			m.MWRRTenYear,           // 33
			m.ActiveReturnOneYear,   // 34
			m.ActiveReturnThreeYear, // 35
			m.ActiveReturnFiveYear,  // 36
			m.ActiveReturnTenYear,   // 37
			m.CalmarRatio,           // 38
			m.DownsideDeviation,     // 39
			m.InformationRatio,      // 40
			m.KRatio,                // 41
			m.KellerRatio,           // 42
			m.SharpeRatio,           // 43
			m.SortinoRatio,          // 44
			m.StdDev,                // 45
			m.TreynorRatio,          // 46
			m.UlcerIndex,            // 47
			m.BenchmarkValue,        // 48
			m.StrategyGrowthOf10K,   // 49
			m.BenchmarkGrowthOf10K,  // 50
			m.RiskFreeGrowthOf10K,   // 51
			m.TWRRWeekToDate,        // 52
			m.TWRRMonthToDate,       // 53
			m.TWRRYearToDate,        // 54
			m.MWRRWeekToDate,        // 55
			m.MWRRMonthToDate,       // 56
			m.MWRRYearToDate,        // 57
		)
		if err != nil {
			log.WithFields(log.Fields{
				"Error":       err,
				"Measurement": fmt.Sprintf("%+v", m),
			}).Debug("could not save portfolio measurement")
			return err
		}
	}

	return nil
}
