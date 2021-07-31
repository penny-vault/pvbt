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

// TYPES

type DrawDown struct {
	Begin       int64   `json:"begin"`
	End         int64   `json:"end"`
	Recovery    int64   `json:"recovery"`
	LossPercent float64 `json:"lossPercent"`
}

// Performance of portfolio
type Performance struct {
	PortfolioID uuid.UUID `json:"portfolioID"`
	PeriodStart int64     `json:"periodStart"`
	PeriodEnd   int64     `json:"periodEnd"`
	ComputedOn  int64     `json:"computedOn"`

	CurrentAssets []*ReportableHolding      `json:"currentAssets"`
	Measurements  []*PerformanceMeasurement `json:"measurements"`
	DrawDowns     []*DrawDown               `json:"drawDowns"`

	MWRRSinceInception float64 `json:"mwrrSinceInception"`
	MWRReturnYTD       float64 `json:"mwrrYtd"`
	TWRRSinceInception float64 `json:"twrrSinceInception"`
	TWRReturnYTD       float64 `json:"twrrYtd"`

	AlphaSinceInception          float64 `json:"alpha"`
	AvgDrawDown                  float64 `json:"avgDrawDown"`
	BetaSinceInception           float64 `json:"beta"`
	ExcessKurtosisSinceInception float64 `json:"excessKurtosis"`
	SharpeRatioSinceInception    float64 `json:"sharpRatio"`
	Skewness                     float64 `json:"skewness"`
	SortinoRatioSinceInception   float64 `json:"sortinoRatio"`
	UlcerIndexAvg                float64 `json:"ulcerIndexAvg"`
	UlcerIndexP50                float64 `json:"ulcerIndexP50"`
	UlcerIndexP90                float64 `json:"uclerIndexP90"`
	UlcerIndexP99                float64 `json:"ulcerIndexP99"`

	DynamicWithdrawalRateSinceInception   float64 `json:"dynamicWithdrawalRate"`
	PerpetualWithdrawalRateSinceInception float64 `json:"perpetualWithdrawalRate"`
	SafeWithdrawalRateSinceInception      float64 `json:"safeWithdrawalRate"`

	//	UpsideCaptureRatio   float64 `json:"upsideCaptureRatio"`
	//	DownsideCaptureRatio float64 `json:"downsideCaptureRatio"`
}

type PerformanceMeasurement struct {
	Time int64 `json:"time"`

	Value          float64 `json:"value"`
	BenchmarkValue float64 `json:"benchmarkValue"`
	RiskFreeValue  float64 `json:"riskFreeValue"`

	StrategyGrowthOf10K  float64 `json:"strategyGrowthOf10K"`
	BenchmarkGrowthOf10K float64 `json:"benchmarkGrowthOf10K"`
	RiskFreeGrowthOf10K  float64 `json:"riskFreeGrowthOf10K"`

	Holdings       []*ReportableHolding   `json:"holdings"`
	TotalDeposited float64                `json:"totalDeposited"`
	TotalWithdrawn float64                `json:"totalWithdrawn"`
	Justification  map[string]interface{} `json:"justification"`

	// Time-weighted rate of return
	TWRROneDay     float64 `json:"twrrOneDay"`
	TWRROneWeek    float64 `json:"twrrOneWeek"`
	TWRROneMonth   float64 `json:"twrrOneMonth"`
	TWRRThreeMonth float64 `json:"twrrThreeMonth"`
	TWRROneYear    float64 `json:"twrrOneYear"`
	TWRRThreeYear  float64 `json:"twrrThreeYear"`
	TWRRFiveYear   float64 `json:"twrrFiveYear"`
	TWRRTenYear    float64 `json:"twrrTenYear"`

	// Money-weighted rate of return
	MWRROneDay     float64 `json:"mwrrOneDay"`
	MWRROneWeek    float64 `json:"mwrrOneWeek"`
	MWRROneMonth   float64 `json:"mwrrOneMonth"`
	MWRRThreeMonth float64 `json:"mwrrThreeMonth"`
	MWRROneYear    float64 `json:"mwrrOneYear"`
	MWRRThreeYear  float64 `json:"mwrrThreeYear"`
	MWRRFiveYear   float64 `json:"mwrrFiveYear"`
	MWRRTenYear    float64 `json:"mwrrTenYear"`

	// active return
	ActiveReturnOneYear   float64 `json:"activeReturnOneYear"`
	ActiveReturnThreeYear float64 `json:"activeReturnThreeYear"`
	ActiveReturnFiveYear  float64 `json:"activeReturnFiveYear"`
	ActiveReturnTenYear   float64 `json:"activeReturnTenYear"`

	// alpha
	AlphaOneYear   float64 `json:"alphaOneYear"`
	AlphaThreeYear float64 `json:"alphaThreeYear"`
	AlphaFiveYear  float64 `json:"alphaFiveYear"`
	AlphaTenYear   float64 `json:"alphaTenYear"`

	// beta
	BetaOneYear   float64 `json:"betaOneYear"`
	BetaThreeYear float64 `json:"betaThreeYear"`
	BetaFiveYear  float64 `json:"betaFiveYear"`
	BetaTenYear   float64 `json:"betaTenYear"`

	// ratios
	CalmarRatio       float64 `json:"calmarRatio"`
	DownsideDeviation float64 `json:"downsideDeviation"`
	InformationRatio  float64 `json:"informationRatio"`
	KRatio            float64 `json:"kRatio"`
	KellerRatio       float64 `json:"kellerRatio"`
	SharpeRatio       float64 `json:"sharpeRatio"`
	SortinoRatio      float64 `json:"sortinoRatio"`
	StdDev            float64 `json:"stdDev"`
	TreynorRatio      float64 `json:"treynorRatio"`
	UlcerIndex        float64 `json:"ulcerIndex"`
}

type ReportableHolding struct {
	Ticker           string  `json:"ticker"`
	Shares           float64 `json:"shares"`
	PercentPortfolio float64 `json:"percentPortfolio"`
	Value            float64 `json:"value"`
}

// METHODS

// CalculatePerformance calculates various performance metrics of portfolio
func (p *Portfolio) CalculatePerformance(through time.Time) (*Performance, error) {
	if len(p.Transactions) == 0 {
		return nil, errors.New("cannot calculate performance for portfolio with no transactions")
	}

	perf := Performance{
		PortfolioID: p.ID,
		PeriodStart: p.StartDate.Unix(),
		PeriodEnd:   through.Unix(),
		ComputedOn:  time.Now().Unix(),
	}

	// Get a list of symbols that data should be pulled for
	uniqueSymbols := make(map[string]bool, len(p.Holdings)+1)
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
	p.dataProxy.Begin = p.StartDate
	p.dataProxy.End = through
	p.dataProxy.Frequency = data.FrequencyDaily

	quotes, errs := p.dataProxy.GetMultipleData(symbols...)
	if len(errs) > 0 {
		return nil, errors.New("failed to download data for tickers")
	}

	var eod = []*dataframe.DataFrame{}
	for _, val := range quotes {
		eod = append(eod, val)
	}

	// Get benchmark quotes but use adjustedClose prices
	metric := p.dataProxy.Metric
	p.dataProxy.Metric = data.MetricAdjustedClose
	benchmarkEod, err := p.dataProxy.GetData(p.Benchmark)
	if err != nil {
		return nil, err
	}
	p.dataProxy.Metric = metric
	benchColumn, err := benchmarkEod.NameToColumn(p.Benchmark)
	if err != nil {
		return nil, err
	}
	s := benchmarkEod.Series[benchColumn]
	s.Rename("$BENCHMARK")
	eod = append(eod, benchmarkEod)

	eodQuotes, err := dfextras.Merge(context.TODO(), data.DateIdx, eod...)
	if err != nil {
		return nil, err
	}

	dfextras.DropNA(context.TODO(), eodQuotes, dataframe.FilterOptions{
		InPlace: true,
	})

	iterator := eodQuotes.ValuesIterator(dataframe.ValuesOptions{InitialRow: 0, Step: 1, DontReadLock: false})
	trxIdx := 0
	numTrxs := len(p.Transactions)
	holdings := make(map[string]float64)
	nYears := int(math.Ceil(through.Sub(p.StartDate).Hours() / (24 * 365)))
	perf.Measurements = make([]*PerformanceMeasurement, 0, 252*nYears)
	var prevVal float64 = -1
	today := time.Now()
	currYear := today.Year()
	var totalVal float64
	var currYearStartValue float64 = -1
	var riskFreeValue float64 = 0

	var lastJustification map[string]interface{}

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

		if benchmarkShares == 0.0 {
			benchmarkShares = benchmarkValue / quotes["$BENCHMARK"].(float64)
		} else {
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

			lastJustification = trx.Justification

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
				log.Debugf("[meas update value] %s ** %s ** price=%.5f ** value=%.2f", date, symbol, price, value)
			} else {
				return nil, fmt.Errorf("no quote for symbol: %s", symbol)
			}
			if qty > 1.0e-5 {
				currentAssets = append(currentAssets, &ReportableHolding{
					Ticker:           symbol,
					Shares:           qty,
					PercentPortfolio: value / totalVal,
					Value:            value,
				})
			}
		}

		if prevVal == -1 {
			prevVal = totalVal
		} else {
			// update riskFreeValue
			rawRate := p.dataProxy.RiskFreeRate(date)
			riskFreeRate := rawRate / 100.0 / 12.0
			riskFreeValue *= (1 + riskFreeRate)
		}

		prevVal = totalVal

		measurement := PerformanceMeasurement{
			Time:                 date.Unix(),
			Value:                totalVal,
			BenchmarkValue:       benchmarkValue,
			RiskFreeValue:        riskFreeValue,
			StrategyGrowthOf10K:  stratGrowth,
			BenchmarkGrowthOf10K: benchGrowth,
			RiskFreeGrowthOf10K:  riskFreeGrowth,
			Holdings:             currentAssets,
			TotalDeposited:       depositedToDate,
			TotalWithdrawn:       withdrawnToDate,
			Justification:        lastJustification,
		}

		log.Debugf("[create meas] %s ** %.5f", date, totalVal)

		perf.Measurements = append(perf.Measurements, &measurement)

		if len(perf.Measurements) >= 2 {
			// Growth of 10k
			measurement.StrategyGrowthOf10K = stratGrowth * perf.TWRR(2, STRATEGY)
			measurement.BenchmarkGrowthOf10K = benchGrowth * perf.TWRR(2, BENCHMARK)
			measurement.RiskFreeGrowthOf10K = riskFreeGrowth * perf.TWRR(2, RISKFREE)

			stratGrowth = measurement.StrategyGrowthOf10K
			benchGrowth = measurement.BenchmarkGrowthOf10K
			riskFreeGrowth = measurement.RiskFreeGrowthOf10K

			// time-weighted rate of return
			measurement.TWRROneDay = perf.TWRR(2, STRATEGY)
			measurement.TWRROneWeek = perf.TWRR(5, STRATEGY)
			measurement.TWRROneMonth = perf.TWRR(21, STRATEGY)
			measurement.TWRRThreeMonth = perf.TWRR(63, STRATEGY)
			measurement.TWRROneYear = perf.TWRR(252, STRATEGY)
			measurement.TWRRThreeYear = perf.TWRR(756, STRATEGY)
			measurement.TWRRFiveYear = perf.TWRR(1260, STRATEGY)
			measurement.TWRRTenYear = perf.TWRR(2520, STRATEGY)

			// money-weighted rate of return
			measurement.MWRROneDay = perf.MWRR(1)
			measurement.MWRROneWeek = perf.MWRR(5)
			measurement.MWRROneMonth = perf.MWRR(21)
			measurement.MWRRThreeMonth = perf.MWRR(63)
			measurement.MWRROneYear = perf.MWRR(252)
			measurement.MWRRThreeYear = perf.MWRR(756)
			measurement.MWRRFiveYear = perf.MWRR(1260)
			measurement.MWRRTenYear = perf.MWRR(2520)

			// active return
			measurement.ActiveReturnOneYear = perf.ActiveReturn(252)
			measurement.ActiveReturnThreeYear = perf.ActiveReturn(756)
			measurement.ActiveReturnFiveYear = perf.ActiveReturn(1260)
			measurement.ActiveReturnTenYear = perf.ActiveReturn(2520)

			// alpha
			measurement.AlphaOneYear = perf.Alpha(252)
			measurement.AlphaThreeYear = perf.Alpha(756)
			measurement.AlphaFiveYear = perf.Alpha(1260)
			measurement.AlphaTenYear = perf.Alpha(2520)

			// beta
			measurement.BetaOneYear = perf.Beta(252)
			measurement.BetaThreeYear = perf.Beta(756)
			measurement.BetaFiveYear = perf.Beta(1260)
			measurement.BetaTenYear = perf.Beta(2520)

			// ratios
			measurement.CalmarRatio = perf.CalmarRatio(756)             // 3 year lookback
			measurement.DownsideDeviation = perf.DownsideDeviation(756) // 3 year lookback
			measurement.InformationRatio = perf.InformationRatio(756)   // 3 year lookback
			measurement.KRatio = perf.KRatio(756)                       // 3 year lookback
			measurement.KellerRatio = perf.KellerRatio(756)             // 3 year lookback
			measurement.SharpeRatio = perf.SharpeRatio(63)              // 3 month lookback
			measurement.SortinoRatio = perf.SortinoRatio(63)            // 3 month lookback
			measurement.StdDev = perf.StdDev(63)                        // 3 month lookback
			measurement.TreynorRatio = perf.TreynorRatio(756)
			measurement.UlcerIndex = perf.UlcerIndex()
		}

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAssets = currentAssets
		}
	}

	sinceInceptionPeriods := uint(len(perf.Measurements))
	perf.AlphaSinceInception = perf.Alpha(sinceInceptionPeriods)
	perf.AvgDrawDown = perf.AverageDrawDown(sinceInceptionPeriods)
	perf.BetaSinceInception = perf.Beta(sinceInceptionPeriods)
	perf.ExcessKurtosisSinceInception = perf.ExcessKurtosis(sinceInceptionPeriods)
	perf.SharpeRatioSinceInception = perf.SharpeRatio(sinceInceptionPeriods)
	perf.Skewness = perf.Skew(sinceInceptionPeriods)
	perf.SortinoRatioSinceInception = perf.SortinoRatio(sinceInceptionPeriods)
	perf.UlcerIndexAvg = perf.AvgUlcerIndex(sinceInceptionPeriods)
	perf.UlcerIndexP50 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .5)
	perf.UlcerIndexP90 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .9)
	perf.UlcerIndexP99 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .99)

	monthlyRets := perf.monthlyReturn(sinceInceptionPeriods)
	bootstrap := CircularBootstrap(monthlyRets, 12, 5000, 360)
	perf.DynamicWithdrawalRateSinceInception = DynamicWithdrawalRate(bootstrap, 0.03)
	perf.PerpetualWithdrawalRateSinceInception = PerpetualWithdrawalRate(bootstrap, 0.03)
	perf.SafeWithdrawalRateSinceInception = SafeWithdrawalRate(bootstrap, 0.03)

	if currYearStartValue <= 0 {
		perf.MWRReturnYTD = 0.0
		perf.TWRReturnYTD = 0.0
	} else {
		perf.MWRReturnYTD = perf.MWRRYtd()
		perf.TWRReturnYTD = perf.TWRRYtd()
	}

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

	p := Performance{
		PortfolioID: portfolioID,
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
	measurementSQL := "SELECT extract(epoch from event_date), value, risk_free_value, holdings, percent_return, justification FROM portfolio_measurement_v1 WHERE portfolio_id=$1 AND user_id=$2 ORDER BY event_date"
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
		err := rows.Scan(&m.Time, &m.Value, &m.RiskFreeValue, &m.Holdings, &m.Justification)
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
		return nil
	}

	sql := `UPDATE TABLE portfolio_v1 SET
		performance_json=$2,
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
	performanceJSON, err := json.Marshal(p)
	if err != nil {
		return err
	}

	fmt.Println(sql)
	fmt.Println(performanceJSON)

	/*
		_, err = trx.Exec(context.Background(), sql, p.PortfolioID, performanceJSON, p.YTDReturn,
			p.CagrSinceInception, p.MetricsBundle.CAGRS.ThreeYear, p.MetricsBundle.CAGRS.FiveYear,
			p.MetricsBundle.CAGRS.TenYear, p.MetricsBundle.StdDev, p.MetricsBundle.DownsideDeviation,
			p.MetricsBundle.MaxDrawDown.LossPercent, p.MetricsBundle.AvgDrawDown,
			p.MetricsBundle.SharpeRatio, p.MetricsBundle.SortinoRatio, p.MetricsBundle.UlcerIndexAvg)
		if err != nil {
			return err
		}
	*/

	trx.Commit(context.Background())
	return nil
}

func (p *Performance) saveMeasurements(trx pgx.Tx, userID string) error {
	sql := `INSERT INTO portfolio_performance_v1 (
		event_date,
		justification,
		percent_return,
		portfolio_id,
		risk_free_value,
		total_deposited_to_date,
		total_withdrawn_to_date,
		user_id,
		value,
		holdings
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
		$10
	) ON CONFLICT ON CONSTRAINT portfolio_performance_v1_pkey
	DO UPDATE SET
	WHERE portfolio_id=$4`

	fmt.Println(sql)
	return nil
}
