package portfolio

import (
	"context"
	"errors"
	"fmt"
	"main/data"
	"main/database"
	"main/dfextras"
	"math"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rocketlaunchr/dataframe-go"
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

	// Get a list of symbols that data should be pulled for
	uniqueSymbols := make(map[string]bool, len(pm.holdings)+1)
	uniqueSymbols[p.Benchmark] = true
	for _, trx := range p.Transactions {
		if trx.Ticker != "" && trx.Ticker != "$CASH" {
			uniqueSymbols[trx.Ticker] = true
		}
	}

	symbols := make([]string, 0, len(uniqueSymbols))
	for k := range uniqueSymbols {
		symbols = append(symbols, k)
	}

	// Calculate performance
	pm.dataProxy.Begin = p.StartDate
	pm.dataProxy.End = through
	pm.dataProxy.Frequency = data.FrequencyDaily

	t1 := time.Now()
	quotes, errs := pm.dataProxy.GetMultipleData(symbols...)
	t2 := time.Now()
	if len(errs) > 0 {
		return nil, errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, val := range quotes {
		eod = append(eod, val)
	}

	// Get benchmark quotes but use adjustedClose prices
	t3 := time.Now()
	metric := pm.dataProxy.Metric
	pm.dataProxy.Metric = data.MetricAdjustedClose
	benchmarkEod, err := pm.dataProxy.GetData(p.Benchmark)
	if err != nil {
		return nil, err
	}
	pm.dataProxy.Metric = metric
	benchColumn, err := benchmarkEod.NameToColumn(p.Benchmark)
	if err != nil {
		return nil, err
	}
	s := benchmarkEod.Series[benchColumn]
	s.Rename("$BENCHMARK")
	eod = append(eod, benchmarkEod)
	t4 := time.Now()

	t5 := time.Now()
	eodQuotes, err := dfextras.Merge(context.TODO(), data.DateIdx, eod...)
	if err != nil {
		return nil, err
	}

	dfextras.DropNA(context.TODO(), eodQuotes, dataframe.FilterOptions{
		InPlace: true,
	})
	t6 := time.Now()

	t7 := time.Now()
	iterator := eodQuotes.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
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

	for {
		row, quotes, _ := iterator(dataframe.SeriesName)
		if row == nil {
			break
		}
		date := quotes[data.DateIdx].(time.Time)

		if benchmarkShares != 0.0 {
			benchmarkValue = benchmarkShares * quotes["$BENCHMARK"].(float64)
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
				continue
			case WithdrawTransaction:
				withdrawnToDate += trx.TotalValue
				riskFreeValue -= trx.TotalValue
				benchmarkValue -= trx.TotalValue
				continue
			case BuyTransaction:
				shares += trx.Shares
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
				log.Debugf("on %s sell %.2f shares of %s for %.2f @ %.2f per share\n", trx.Date, trx.Shares, trx.Ticker, trx.TotalValue, trx.PricePerShare)
			default:
				return nil, errors.New("unrecognized transaction type")
			}

			// Protect against floating point noise
			if shares <= 1.0e-5 {
				shares = 0
			}

			holdings[trx.Ticker] = shares
		}

		// update benchmarkShares to reflect any new deposits or withdrawals
		benchmarkShares = benchmarkValue / quotes["$BENCHMARK"].(float64)

		// iterate through each holding and add value to get total return
		totalVal = 0.0
		for symbol, qty := range holdings {
			if symbol == "$CASH" {
				totalVal += qty
			} else if val, ok := quotes[symbol]; ok {
				price := val.(float64)
				totalVal += price * qty
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", symbol)
			}
		}

		// this is done as a second loop because we need totalVal to be set for
		// percent calculation
		currentAssets := make([]*ReportableHolding, 0, len(holdings))
		for symbol, qty := range holdings {
			var value float64
			if symbol == "$CASH" {
				value = qty
			} else if val, ok := quotes[symbol]; ok {
				price := val.(float64)
				if qty > 1.0e-5 {
					value = price * qty
				}
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", symbol)
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

		if prevVal == -1 {
			prevVal = totalVal
		} else {
			// update riskFreeValue
			rawRate := pm.dataProxy.RiskFreeRate(date)
			riskFreeRate := rawRate / 100.0 / 12.0
			riskFreeValue *= (1 + riskFreeRate)
		}

		prevVal = totalVal

		measurement := PerformanceMeasurement{
			Time:                 date,
			Value:                totalVal,
			BenchmarkValue:       benchmarkValue,
			RiskFreeValue:        riskFreeValue,
			StrategyGrowthOf10K:  stratGrowth,
			BenchmarkGrowthOf10K: benchGrowth,
			RiskFreeGrowthOf10K:  riskFreeGrowth,
			Holdings:             currentAssets,
			TotalDeposited:       depositedToDate,
			TotalWithdrawn:       withdrawnToDate,
		}

		perf.Measurements = append(perf.Measurements, &measurement)

		if len(perf.Measurements) >= 2 {
			// Growth of 10k
			stratRate := perf.TWRR(2, STRATEGY)
			if !math.IsNaN(stratRate) {
				stratGrowth *= (1.0 + stratRate)
			}
			measurement.StrategyGrowthOf10K = stratGrowth

			benchRate := perf.TWRR(2, BENCHMARK)
			if !math.IsNaN(benchRate) {
				benchGrowth *= (1.0 + benchRate)
			}
			measurement.BenchmarkGrowthOf10K = benchGrowth

			rfRate := perf.TWRR(2, RISKFREE)
			if !math.IsNaN(benchRate) {
				riskFreeGrowth *= (1.0 + rfRate)
			}
			measurement.RiskFreeGrowthOf10K *= riskFreeGrowth

			// time-weighted rate of return
			measurement.TWRROneDay = float32(perf.TWRR(2, STRATEGY))
			measurement.TWRROneWeek = float32(perf.TWRR(5, STRATEGY))
			measurement.TWRROneMonth = float32(perf.TWRR(21, STRATEGY))
			measurement.TWRRThreeMonth = float32(perf.TWRR(63, STRATEGY))
			measurement.TWRROneYear = float32(perf.TWRR(252, STRATEGY))
			measurement.TWRRThreeYear = float32(perf.TWRR(756, STRATEGY))
			measurement.TWRRFiveYear = float32(perf.TWRR(1260, STRATEGY))
			measurement.TWRRTenYear = float32(perf.TWRR(2520, STRATEGY))

			// money-weighted rate of return
			measurement.MWRROneDay = float32(perf.MWRR(1, STRATEGY))
			measurement.MWRROneWeek = float32(perf.MWRR(5, STRATEGY))
			measurement.MWRROneMonth = float32(perf.MWRR(21, STRATEGY))
			measurement.MWRRThreeMonth = float32(perf.MWRR(63, STRATEGY))
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
			measurement.CalmarRatio = float32(perf.CalmarRatio(756))             // 3 year lookback
			measurement.DownsideDeviation = float32(perf.DownsideDeviation(756)) // 3 year lookback
			measurement.InformationRatio = float32(perf.InformationRatio(756))   // 3 year lookback
			measurement.KRatio = float32(perf.KRatio(756))                       // 3 year lookback
			measurement.KellerRatio = float32(perf.KellerRatio(756))             // 3 year lookback
			measurement.SharpeRatio = float32(perf.SharpeRatio(63))              // 3 month lookback
			measurement.SortinoRatio = float32(perf.SortinoRatio(63))            // 3 month lookback
			measurement.StdDev = float32(perf.StdDev(63))                        // 3 month lookback
			measurement.TreynorRatio = float32(perf.TreynorRatio(756))
			measurement.UlcerIndex = float32(perf.UlcerIndex())
		}

		if prevDate.Year() != date.Year() {
			if prevMeasurement.TWRROneYear > bestYearPort.Return {
				bestYearPort.Return = prevMeasurement.TWRROneYear
				bestYearPort.Year = uint16(prevDate.Year())
			}

			if prevMeasurement.TWRROneYear < worstYearPort.Return {
				worstYearPort.Return = prevMeasurement.TWRROneYear
				worstYearPort.Year = uint16(prevDate.Year())
			}

			// calculate 1-yr benchmark rate of return
			numMeasurements := len(perf.Measurements)
			if numMeasurements > 252 {

				measYr1 := perf.Measurements[numMeasurements-253]
				rr := float32((prevMeasurement.BenchmarkGrowthOf10K / measYr1.BenchmarkGrowthOf10K) - 1.0)
				if rr > bestYearBenchmark.Return {
					bestYearBenchmark.Return = rr
					bestYearBenchmark.Year = uint16(prevDate.Year())
				}

				if rr < worstYearBenchmark.Return {
					worstYearBenchmark.Return = rr
					worstYearBenchmark.Year = uint16(prevDate.Year())
				}
			}
		}
		prevMeasurement = measurement
		prevDate = date

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAssets = currentAssets
		}
	}

	sinceInceptionPeriods := uint(len(perf.Measurements))
	perf.DrawDowns = perf.Top10DrawDowns(sinceInceptionPeriods)

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
		AvgDrawDown:                     perf.AverageDrawDown(sinceInceptionPeriods),
		BetaSinceInception:              perf.Beta(sinceInceptionPeriods),
		BestYear:                        &bestYearPort,
		DownsideDeviationSinceInception: perf.DownsideDeviation(sinceInceptionPeriods),
		ExcessKurtosisSinceInception:    perf.ExcessKurtosis(sinceInceptionPeriods),
		FinalBalance:                    perf.Measurements[len(perf.Measurements)-1].Value,
		SharpeRatioSinceInception:       perf.SharpeRatio(sinceInceptionPeriods),
		Skewness:                        perf.Skew(sinceInceptionPeriods),
		SortinoRatioSinceInception:      perf.SortinoRatio(sinceInceptionPeriods),
		StdDevSinceInception:            perf.StdDev(sinceInceptionPeriods),
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
		AvgDrawDown:                     perf.AverageDrawDown(sinceInceptionPeriods),
		BestYear:                        &bestYearBenchmark,
		BetaSinceInception:              math.NaN(),
		DownsideDeviationSinceInception: perf.DownsideDeviation(sinceInceptionPeriods),
		ExcessKurtosisSinceInception:    perf.ExcessKurtosis(sinceInceptionPeriods),
		FinalBalance:                    perf.Measurements[len(perf.Measurements)-1].BenchmarkValue,
		SharpeRatioSinceInception:       perf.SharpeRatio(sinceInceptionPeriods),
		Skewness:                        perf.Skew(sinceInceptionPeriods),
		SortinoRatioSinceInception:      perf.SortinoRatio(sinceInceptionPeriods),
		StdDevSinceInception:            perf.StdDev(sinceInceptionPeriods),
		TotalDeposited:                  math.NaN(),
		TotalWithdrawn:                  math.NaN(),
		UlcerIndexAvg:                   perf.AvgUlcerIndex(sinceInceptionPeriods),
		UlcerIndexP50:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .5),
		UlcerIndexP90:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .9),
		UlcerIndexP99:                   perf.UlcerIndexPercentile(sinceInceptionPeriods, .99),
		WorstYear:                       &worstYearBenchmark,
	}

	monthlyRets := perf.monthlyReturn(sinceInceptionPeriods)
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
	t8 := time.Now()

	log.WithFields(log.Fields{
		"QuoteDownload":          t2.Sub(t1).Round(time.Millisecond),
		"BenchmarkDownload":      t4.Sub(t3).Round(time.Millisecond),
		"QuoteMerge":             t6.Sub(t5).Round(time.Millisecond),
		"PerformanceCalculation": t8.Sub(t7).Round(time.Millisecond),
	}).Info("CalculatePerformance runtime")

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
		risk_free_growth_of_10k
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
		$50
	) ON CONFLICT ON CONSTRAINT portfolio_measurement_v1_pkey
	DO UPDATE SET
		risk_free_value=$3,
		total_deposited_to_date=$4,
		total_withdrawn_to_date=$5,
		strategy_value=$7,
		holdings=$8,
		alpha_1yr=$9,
		alpha_3yr=$10,
		alpha_5yr=$11,
		alpha_10yr=$12,
		beta_1yr=$13,
		beta_3yr=$14,
		beta_5yr=$15,
		beta_10yr=$16,
		twrr_1d=$17,
		twrr_1wk=$18,
		twrr_1mo=$19,
		twrr_3mo=$20,
		twrr_1yr=$21,
		twrr_3yr=$22,
		twrr_5yr=$23,
		twrr_10yr=$24,
		mwrr_1d=$25,
		mwrr_1wk=$26,
		mwrr_1mo=$27,
		mwrr_3mo=$28,
		mwrr_1yr=$29,
		mwrr_3yr=$30,
		mwrr_5yr=$31,
		mwrr_10yr=$32,
		active_return_1yr=$33,
		active_return_3yr=$34,
		active_return_5yr=$35,
		active_return_10yr=$36,
		calmar_ratio=$37,
		downside_deviation=$38,
		information_ratio=$39,
		k_ratio=$40,
		keller_ratio=$41,
		sharpe_ratio=$42,
		sortino_ratio=$43,
		std_dev=$44,
		treynor_ratio=$45,
		ulcer_index=$46,
		benchmark_value=$47,
		strategy_growth_of_10k=$48,
		benchmark_growth_of_10k=$49,
		risk_free_growth_of_10k=$50`

	for _, m := range p.Measurements {
		holdings, err := json.Marshal(m.Holdings)
		if err != nil {
			return err
		}

		_, err = trx.Exec(context.Background(), sql,
			m.Time,
			p.PortfolioID,
			m.RiskFreeValue,
			m.TotalDeposited,
			m.TotalWithdrawn,
			userID,
			m.Value,
			holdings,
			m.AlphaOneYear,
			m.AlphaThreeYear,
			m.AlphaFiveYear,
			m.AlphaTenYear,
			m.BetaOneYear,
			m.BetaThreeYear,
			m.BetaFiveYear,
			m.BetaTenYear,
			m.TWRROneDay,
			m.TWRROneWeek,
			m.TWRROneMonth,
			m.TWRRThreeMonth,
			m.TWRROneYear,
			m.TWRRThreeYear,
			m.TWRRFiveYear,
			m.TWRRTenYear,
			m.MWRROneDay,
			m.MWRROneWeek,
			m.MWRROneMonth,
			m.MWRRThreeMonth,
			m.MWRROneYear,
			m.MWRRThreeYear,
			m.MWRRFiveYear,
			m.MWRRTenYear,
			m.ActiveReturnOneYear,
			m.ActiveReturnThreeYear,
			m.ActiveReturnFiveYear,
			m.ActiveReturnTenYear,
			m.CalmarRatio,
			m.DownsideDeviation,
			m.InformationRatio,
			m.KRatio,
			m.KellerRatio,
			m.SharpeRatio,
			m.SortinoRatio,
			m.StdDev,
			m.TreynorRatio,
			m.UlcerIndex,
			m.BenchmarkValue,
			m.StrategyGrowthOf10K,
			m.BenchmarkGrowthOf10K,
			m.RiskFreeGrowthOf10K)
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
