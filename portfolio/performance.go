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

package portfolio

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pv-api/data"
	"github.com/penny-vault/pv-api/data/database"
	"github.com/penny-vault/pv-api/observability/opentelemetry"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var (
	ErrInvalidTransactionType = errors.New("unrecognized transaction type")
	ErrNoTransactions         = errors.New("portfolio has no transactions")
	ErrSerialize              = errors.New("could not serialize data")
)

type cumulativeSums struct {
	Deposited      float64
	BenchmarkValue float64
	RiskFreeValue  float64
	TotalValue     float64
	Withdrawn      float64
}

type dateBundle struct {
	DaysToStartOfMonth    uint
	DaysToStartOfWeek     uint
	DaysToStartOfYear     uint
	SinceInceptionPeriods uint
	StartOfMonth          time.Time
	StartOfWeek           time.Time
	StartOfYear           time.Time
}

// METHODS

func NewPerformance(p *Portfolio) *Performance {
	perf := Performance{
		BenchmarkMetrics: &Metrics{
			AlphaSinceInception:                   math.NaN(),
			AvgDrawDown:                           math.NaN(),
			BetaSinceInception:                    math.NaN(),
			DownsideDeviationSinceInception:       math.NaN(),
			ExcessKurtosisSinceInception:          math.NaN(),
			FinalBalance:                          math.NaN(),
			SharpeRatioSinceInception:             math.NaN(),
			Skewness:                              math.NaN(),
			SortinoRatioSinceInception:            math.NaN(),
			StdDevSinceInception:                  math.NaN(),
			TotalDeposited:                        0.0,
			TotalWithdrawn:                        0.0,
			UlcerIndexAvg:                         math.NaN(),
			UlcerIndexP50:                         math.NaN(),
			UlcerIndexP90:                         math.NaN(),
			UlcerIndexP99:                         math.NaN(),
			DynamicWithdrawalRateSinceInception:   math.NaN(),
			PerpetualWithdrawalRateSinceInception: math.NaN(),
			SafeWithdrawalRateSinceInception:      math.NaN(),
			BestYear: &AnnualReturn{
				Year:   0,
				Return: -99999,
			},
			WorstYear: &AnnualReturn{
				Year:   0,
				Return: 99999,
			},
		},
		ComputedOn:   time.Now(),
		Measurements: make([]*PerformanceMeasurement, 0, 2520),
		PeriodStart:  p.StartDate,
		PortfolioID:  p.ID,
		PortfolioMetrics: &Metrics{
			AlphaSinceInception:                   math.NaN(),
			AvgDrawDown:                           math.NaN(),
			BetaSinceInception:                    math.NaN(),
			DownsideDeviationSinceInception:       math.NaN(),
			ExcessKurtosisSinceInception:          math.NaN(),
			FinalBalance:                          math.NaN(),
			SharpeRatioSinceInception:             math.NaN(),
			Skewness:                              math.NaN(),
			SortinoRatioSinceInception:            math.NaN(),
			StdDevSinceInception:                  math.NaN(),
			TotalDeposited:                        0.0,
			TotalWithdrawn:                        0.0,
			UlcerIndexAvg:                         math.NaN(),
			UlcerIndexP50:                         math.NaN(),
			UlcerIndexP90:                         math.NaN(),
			UlcerIndexP99:                         math.NaN(),
			DynamicWithdrawalRateSinceInception:   math.NaN(),
			PerpetualWithdrawalRateSinceInception: math.NaN(),
			SafeWithdrawalRateSinceInception:      math.NaN(),
			BestYear: &AnnualReturn{
				Year:   0,
				Return: -99999,
			},
			WorstYear: &AnnualReturn{
				Year:   0,
				Return: 99999,
			},
		},
	}

	return &perf
}

// transactionIndexForDate find the transaction index that has the earliest date on or after dt
func (pm *Model) transactionIndexForDate(dt time.Time) int {
	// TODO update to Binary Search
	var val *Transaction
	idx := 0

	// There are a number of cases to consider here:
	// 1) dt is before all transactions
	// 2) dt is somewhere within the transaction stream
	for idx, val = range pm.Portfolio.Transactions {
		if dt.Equal(val.Date) || dt.Before(val.Date) {
			return idx
		}
	}

	// 3) dt is after all transactions
	return idx + 1
}

func (perf *Performance) ValueAtYearStart(dt time.Time) float64 {
	// TODO update to Binary Search
	for _, val := range perf.Measurements {
		if val.Time.Year() == dt.Year() {
			return val.Value
		}
	}
	return 0.0
}

func getHoldingsValue(ctx context.Context, dataManager *data.Manager, holdings map[string]float64, date time.Time) (float64, error) {
	totalVal := 0.0
	for symbol, qty := range holdings {
		if symbol == data.CashAsset {
			if math.IsNaN(qty) {
				log.Warn().Msg("Cash position is NaN")
			} else {
				totalVal += qty
			}
		} else {
			price, err := dataManager.Get(ctx, date, data.MetricClose, symbol)
			if err != nil {
				return totalVal, ErrSecurityPriceNotAvailable
			}
			if math.IsNaN(price) {
				price, err = dataManager.GetLatestDataBefore(ctx, symbol, data.MetricClose, date)
				if err != nil {
					return totalVal, ErrSecurityPriceNotAvailable
				}
			}

			totalVal += price * qty
		}
	}

	return totalVal, nil
}

func updateDateBundle(bundle *dateBundle, date time.Time, last time.Time) {
	if last.Weekday() > date.Weekday() {
		bundle.DaysToStartOfWeek = 1
	} else {
		bundle.DaysToStartOfWeek++
	}

	if last.Month() != date.Month() {
		bundle.DaysToStartOfMonth = 1
	} else {
		bundle.DaysToStartOfMonth++
	}

	if last.Year() != date.Year() {
		bundle.DaysToStartOfYear = 1
	} else {
		bundle.DaysToStartOfYear++
	}
}

func holdingsMapFromMeasurement(measurement *PerformanceMeasurement) map[string]float64 {
	holdings := make(map[string]float64)
	for _, holding := range measurement.Holdings {
		holdings[holding.Ticker] = holding.Shares
	}
	return holdings
}

func processTransactions(p *Portfolio, holdings map[string]float64, trxIdx int, date time.Time, sums *cumulativeSums) (map[string]float64, int, error) {
	numTrxs := len(p.Transactions)

	for ; trxIdx < numTrxs; trxIdx++ {
		trx := p.Transactions[trxIdx]

		// process transactions up to this point in time
		// test if date is Before the trx.Date - if it is then break
		if date.Before(trx.Date) {
			break
		}

		shares := 0.0
		if val, ok := holdings[trx.Ticker]; ok {
			shares = val
		}

		switch trx.Kind {
		case DepositTransaction:
			sums.Deposited += trx.TotalValue
			sums.RiskFreeValue += trx.TotalValue
			sums.BenchmarkValue += trx.TotalValue
			if val, ok := holdings[data.CashAsset]; ok {
				holdings[data.CashAsset] = val + trx.TotalValue
			} else {
				holdings[data.CashAsset] = trx.TotalValue
			}
			continue
		case WithdrawTransaction:
			sums.Withdrawn += trx.TotalValue
			sums.RiskFreeValue -= trx.TotalValue
			sums.BenchmarkValue -= trx.TotalValue
			if val, ok := holdings[data.CashAsset]; ok {
				holdings[data.CashAsset] = val - trx.TotalValue
			}
			continue
		case BuyTransaction:
			shares += trx.Shares
			if val, ok := holdings[data.CashAsset]; ok {
				holdings[data.CashAsset] = val - trx.TotalValue
			}
			log.Debug().Time("Date", trx.Date).Str("Kind", "buy").Float64("Shares", trx.Shares).Str("Ticker", trx.Ticker).Float64("TotalValue", trx.TotalValue).Float64("Price", trx.PricePerShare).Msg("buy shares")
		case DividendTransaction:
			if val, ok := holdings[data.CashAsset]; ok {
				holdings[data.CashAsset] = val + trx.TotalValue
			} else {
				holdings[data.CashAsset] = trx.TotalValue
			}
			log.Debug().Time("Date", trx.Date).Str("Ticker", trx.Ticker).Float64("Amount", trx.TotalValue).Msg("dividend released")
			continue
		case SplitTransaction:
			shares = trx.Shares
			log.Debug().Time("Date", trx.Date).Str("Ticker", trx.Ticker).Float64("Shares", trx.Shares).Msg("asset split")
		case SellTransaction:
			shares -= trx.Shares
			if val, ok := holdings[data.CashAsset]; ok {
				holdings[data.CashAsset] = val + trx.TotalValue
			} else {
				holdings[data.CashAsset] = trx.TotalValue
			}
			log.Debug().Time("Date", trx.Date).Str("Kind", "sell").Float64("Shares", trx.Shares).Str("Ticker", trx.Ticker).Float64("TotalValue", trx.TotalValue).Float64("Price", trx.PricePerShare).Msg("sell shares")
		default:
			log.Warn().Time("TransactionDate", trx.Date).Str("TransactionKind", trx.Kind).Msg("unrecognized transaction")
			return holdings, trxIdx, ErrInvalidTransactionType
		}

		if val, ok := holdings[data.CashAsset]; ok {
			if val <= 1.0e-5 {
				delete(holdings, data.CashAsset)
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

	return holdings, trxIdx, nil
}

func calculateShares(ctx context.Context, dataManager *data.Manager, asset string, date time.Time, dollars float64) (float64, error) {
	price, err := dataManager.Get(ctx, date, data.MetricAdjustedClose, asset)
	if err != nil {
		log.Error().Time("Date", date).Str("Ticker", asset).Err(err).Str("Metric", "AdjustedClose").Msg("error when fetching benchmark adjusted close prices")
		return 0, ErrSecurityPriceNotAvailable
	}
	if math.IsNaN(price) {
		log.Warn().Time("Date", date).Str("Ticker", asset).Err(err).Str("Metric", "AdjustedClose").Msg("benchmark value is NaN")
		return 0, ErrSecurityPriceNotAvailable
	}
	return dollars / price, nil
}

func calculateValue(ctx context.Context, dataManager *data.Manager, asset string, shares float64, date time.Time) float64 {
	price, err := dataManager.Get(ctx, date, data.MetricAdjustedClose, asset)
	if err != nil {
		log.Error().Err(err).Str("Asset", asset).Time("Date", date).Msg("could not get security prices from pvdb")
	}
	return shares * price
}

func buildHoldingsList(ctx context.Context, dataManager *data.Manager, holdings map[string]float64, date time.Time, totalValue float64) ([]*ReportableHolding, error) {
	currentAssets := make([]*ReportableHolding, 0, len(holdings))
	for symbol, qty := range holdings {
		var value float64
		if symbol == data.CashAsset {
			value = qty
		} else if qty > 1.0e-5 {
			price, err := dataManager.Get(ctx, date, data.MetricClose, symbol)
			if err != nil {
				return nil, ErrSecurityPriceNotAvailable
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
				PercentPortfolio: float32(value / totalValue),
				Value:            value,
			})
		}
	}

	// ensure that holdings are sorted
	sort.Slice(currentAssets, func(i, j int) bool {
		return currentAssets[i].Ticker < currentAssets[j].Ticker
	})

	return currentAssets, nil
}

// updateMetrics calculates individual metrics for the BENCHMARK and STRATEGY
func (perf *Performance) updateSummaryMetrics(metrics *Metrics, kind string) {
	sinceInceptionPeriods := uint(len(perf.Measurements) - 1)

	metrics.AvgDrawDown = perf.AverageDrawDown(sinceInceptionPeriods, kind)
	metrics.DownsideDeviationSinceInception = perf.DownsideDeviation(sinceInceptionPeriods, kind)
	metrics.SharpeRatioSinceInception = perf.SharpeRatio(sinceInceptionPeriods, kind)
	metrics.Skewness = perf.Skew(sinceInceptionPeriods, kind)
	metrics.SortinoRatioSinceInception = perf.SortinoRatio(sinceInceptionPeriods, kind)
	metrics.StdDevSinceInception = perf.StdDev(sinceInceptionPeriods, kind)

	switch kind {
	case STRATEGY:
		metrics.AlphaSinceInception = perf.Alpha(sinceInceptionPeriods)
		metrics.BetaSinceInception = perf.Beta(sinceInceptionPeriods)
		metrics.ExcessKurtosisSinceInception = perf.ExcessKurtosis(sinceInceptionPeriods)
		metrics.FinalBalance = perf.Measurements[len(perf.Measurements)-1].Value
		metrics.TotalDeposited = perf.Measurements[len(perf.Measurements)-1].TotalDeposited
		metrics.TotalWithdrawn = perf.Measurements[len(perf.Measurements)-1].TotalWithdrawn
		metrics.UlcerIndexAvg = perf.AvgUlcerIndex(sinceInceptionPeriods)
		metrics.UlcerIndexP50 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .5)
		metrics.UlcerIndexP90 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .9)
		metrics.UlcerIndexP99 = perf.UlcerIndexPercentile(sinceInceptionPeriods, .99)
		perf.DrawDowns = perf.Top10DrawDowns(sinceInceptionPeriods, STRATEGY)
	case BENCHMARK:
		metrics.FinalBalance = perf.Measurements[len(perf.Measurements)-1].BenchmarkValue
	}

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

	perf.PortfolioReturns.MWRRYTD = perf.MWRRYtd(STRATEGY)
	perf.PortfolioReturns.TWRRYTD = perf.TWRRYtd(STRATEGY)
	perf.BenchmarkReturns.MWRRYTD = perf.MWRRYtd(BENCHMARK)
	perf.BenchmarkReturns.TWRRYTD = perf.TWRRYtd(BENCHMARK)
}

func getRiskFreeRate(ctx context.Context, dataManager *data.Manager, date time.Time) float64 {
	rawRate := dataManager.RiskFreeRate(ctx, date)
	riskFreeRate := rawRate / 100.0 / 252.0
	return (1 + riskFreeRate)
}

func (perf *Performance) updateAnnualPerformance(prevDate time.Time, date time.Time, prevMeasurement *PerformanceMeasurement, ytdBench float32) {
	if prevDate.Year() != date.Year() {
		if prevMeasurement.TWRRYearToDate > perf.PortfolioMetrics.BestYear.Return {
			perf.PortfolioMetrics.BestYear.Return = prevMeasurement.TWRRYearToDate
			perf.PortfolioMetrics.BestYear.Year = uint16(prevDate.Year())
		}

		if prevMeasurement.TWRRYearToDate < perf.PortfolioMetrics.WorstYear.Return && prevMeasurement.TWRRYearToDate != 0.0 {
			perf.PortfolioMetrics.WorstYear.Return = prevMeasurement.TWRRYearToDate
			perf.PortfolioMetrics.WorstYear.Year = uint16(prevDate.Year())
		}

		// calculate 1-yr benchmark rate of return
		if ytdBench > perf.BenchmarkMetrics.BestYear.Return {
			perf.BenchmarkMetrics.BestYear.Return = ytdBench
			perf.BenchmarkMetrics.BestYear.Year = uint16(prevDate.Year())
		}

		if ytdBench < perf.BenchmarkMetrics.WorstYear.Return {
			perf.BenchmarkMetrics.WorstYear.Return = ytdBench
			perf.BenchmarkMetrics.WorstYear.Year = uint16(prevDate.Year())
		}
	}
}

func (perf *Performance) calculateReturns(measurement *PerformanceMeasurement, dates *dateBundle) {
	measurement.TWRROneDay = float32(perf.TWRR(1, STRATEGY))
	measurement.TWRRWeekToDate = float32(perf.TWRR(dates.DaysToStartOfWeek, STRATEGY))
	measurement.TWRROneWeek = float32(perf.TWRR(5, STRATEGY))
	measurement.TWRRMonthToDate = float32(perf.TWRR(dates.DaysToStartOfMonth, STRATEGY))
	measurement.TWRROneMonth = float32(perf.TWRR(21, STRATEGY))
	measurement.TWRRThreeMonth = float32(perf.TWRR(63, STRATEGY))
	measurement.TWRRYearToDate = float32(perf.TWRR(dates.DaysToStartOfYear, STRATEGY))
	measurement.TWRROneYear = float32(perf.TWRR(252, STRATEGY))
	measurement.TWRRThreeYear = float32(perf.TWRR(756, STRATEGY))
	measurement.TWRRFiveYear = float32(perf.TWRR(1260, STRATEGY))
	measurement.TWRRTenYear = float32(perf.TWRR(2520, STRATEGY))

	// money-weighted rate of return
	measurement.MWRROneDay = float32(perf.MWRR(1, STRATEGY))
	measurement.MWRRWeekToDate = float32(perf.MWRR(dates.DaysToStartOfWeek, STRATEGY))
	measurement.MWRROneWeek = float32(perf.MWRR(5, STRATEGY))
	measurement.MWRRMonthToDate = float32(perf.MWRR(dates.DaysToStartOfMonth, STRATEGY))
	measurement.MWRROneMonth = float32(perf.MWRR(21, STRATEGY))
	measurement.MWRRThreeMonth = float32(perf.MWRR(63, STRATEGY))
	measurement.MWRRYearToDate = float32(perf.MWRR(dates.DaysToStartOfYear, STRATEGY))
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
}

func (perf *Performance) calculateRiskMeasures(measurement *PerformanceMeasurement) {
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

func (perf *Performance) calculateWithdrawalRates() {
	sinceInceptionPeriods := uint(len(perf.Measurements) - 1)
	monthlyRets := perf.monthlyReturns(sinceInceptionPeriods, STRATEGY)
	if len(monthlyRets) > 0 {
		bootstrap := CircularBootstrap(monthlyRets, 12, 5000, 360)
		perf.PortfolioMetrics.DynamicWithdrawalRateSinceInception = DynamicWithdrawalRate(bootstrap, 0.03)
		perf.PortfolioMetrics.PerpetualWithdrawalRateSinceInception = PerpetualWithdrawalRate(bootstrap, 0.03)
		perf.PortfolioMetrics.SafeWithdrawalRateSinceInception = SafeWithdrawalRate(bootstrap, 0.03)
	}
}

func (perf *Performance) calculateGrowthOf10k(measurement *PerformanceMeasurement) {
	stratGrowth := measurement.StrategyGrowthOf10K
	benchGrowth := measurement.BenchmarkGrowthOf10K
	riskFreeGrowth := measurement.RiskFreeGrowthOf10K

	// Growth of 10k
	stratRate := perf.TWRR(1, STRATEGY)
	if !math.IsNaN(stratRate) && !math.IsInf(stratRate, 1) {
		stratGrowth *= (1.0 + stratRate)
	}
	measurement.StrategyGrowthOf10K = stratGrowth

	benchRate := perf.TWRR(1, BENCHMARK)
	if !math.IsNaN(benchRate) && !math.IsInf(benchRate, 1) {
		benchGrowth *= (1.0 + benchRate)
	}
	measurement.BenchmarkGrowthOf10K = benchGrowth

	rfRate := perf.TWRR(1, RISKFREE)
	if !math.IsNaN(rfRate) && !math.IsInf(rfRate, 1) {
		riskFreeGrowth *= (1.0 + rfRate)
	}
	measurement.RiskFreeGrowthOf10K = riskFreeGrowth
}

// CalculateThrough computes performance metrics for the given portfolio until `through`
func (perf *Performance) CalculateThrough(ctx context.Context, pm *Model, through time.Time) error {
	ctx, span := otel.Tracer(opentelemetry.Name).Start(ctx, "performance.CalculateThrough")
	defer span.End()

	p := pm.Portfolio
	dataManager := pm.dataProxy
	var err error

	// make sure we can check the data
	if len(p.Transactions) == 0 {
		return ErrNoTransactions
	}

	var prevMeasurement *PerformanceMeasurement
	if len(perf.Measurements) == 0 {
		prevMeasurement = &PerformanceMeasurement{
			StrategyGrowthOf10K:  10_000,
			BenchmarkGrowthOf10K: 10_000,
			RiskFreeGrowthOf10K:  10_000,
			TotalDeposited:       0,
			TotalWithdrawn:       0,
			Value:                0,
			BenchmarkValue:       0,
		}
	} else {
		prevMeasurement = perf.Measurements[len(perf.Measurements)-1]
	}

	calculationStart := prevMeasurement.Time.AddDate(0, 0, 1)
	if calculationStart.Before(p.StartDate) {
		calculationStart = p.StartDate
	}

	// calculationStart should be at midnight nyc
	nyc, _ := time.LoadLocation("America/New_York")
	calculationStart = time.Date(calculationStart.Year(), calculationStart.Month(), calculationStart.Day(), 0, 0, 0, 0, nyc)

	log.Info().
		Time("CalculationPeriod.Start", calculationStart).
		Time("CalculationPeriod.End", through).
		Str("PortfolioName", pm.Portfolio.Name).
		Str("PortfolioID", hex.EncodeToString(pm.Portfolio.ID)).
		Msg("calculate performance from")

	// Get the days performance should be calculated on
	today := time.Now()
	prevDate := prevMeasurement.Time
	tradingDays, err := dataManager.TradingDays(ctx, calculationStart, through, data.FrequencyDaily)
	if err != nil {
		return err
	}

	// get transaction start index
	trxIdx := pm.transactionIndexForDate(calculationStart)
	log.Debug().Int("TrxIdx", trxIdx).Msg("starting from transaction index")

	// fill holdings
	holdings := holdingsMapFromMeasurement(prevMeasurement)

	sums := &cumulativeSums{
		TotalValue:     prevMeasurement.Value,
		BenchmarkValue: prevMeasurement.BenchmarkValue,
		Deposited:      prevMeasurement.TotalDeposited,
		RiskFreeValue:  prevMeasurement.RiskFreeValue,
		Withdrawn:      prevMeasurement.TotalWithdrawn,
	}

	// compute # of shares held for benchmark
	var benchmarkShares float64
	if len(perf.Measurements) > 0 {
		benchmarkShares, err = calculateShares(ctx, dataManager, pm.Portfolio.Benchmark, prevMeasurement.Time, sums.BenchmarkValue)
		if err != nil {
			span.RecordError(err)
			msg := "could not get benchmark eod prices"
			span.SetStatus(codes.Error, msg)
			log.Error().Err(err).Msg(msg)
		}
	}

	dates := &dateBundle{
		StartOfMonth: calculationStart.AddDate(0, 0, -1*int(calculationStart.Day())+1),
		StartOfWeek:  calculationStart.AddDate(0, 0, -1*int(calculationStart.Weekday())+1),
		StartOfYear:  calculationStart.AddDate(0, 0, -1*int(calculationStart.YearDay())+1),
	}
	dates.DaysToStartOfMonth = uint(trxIdx - pm.transactionIndexForDate(dates.StartOfMonth))
	dates.DaysToStartOfWeek = uint(trxIdx - pm.transactionIndexForDate(dates.StartOfWeek))
	dates.DaysToStartOfYear = uint(trxIdx - pm.transactionIndexForDate(dates.StartOfYear))

	var ytdBench float32
	if len(perf.Measurements) > 0 {
		ytdBench = float32(perf.TWRR(dates.DaysToStartOfYear, BENCHMARK))
	}

	if len(tradingDays) > 0 {
		log.Info().Time("First", tradingDays[0]).Time("Last", tradingDays[len(tradingDays)-1]).Msg("Date range for calculate performance")
	}

	for _, date := range tradingDays {
		tradingDate := date
		// measurements should be at 23:59:59.999999999
		date = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 999_999_999, nyc)
		updateDateBundle(dates, date, prevDate)

		sums.BenchmarkValue = calculateValue(ctx, dataManager, pm.Portfolio.Benchmark, benchmarkShares, date)

		// update holdings
		log.Debug().Time("TradeDate", date).Msg("update holdings for date")
		holdings, trxIdx, err = processTransactions(p, holdings, trxIdx, date, sums)
		if err != nil {
			return err
		}

		// build justification array
		justificationArray := pm.justifications[tradingDate.String()]

		// update benchmarkShares to reflect any new deposits or withdrawals (BenchmarkValue is updated in processTransactions)
		benchmarkShares, err = calculateShares(ctx, dataManager, pm.Portfolio.Benchmark, date, sums.BenchmarkValue)
		if err != nil {
			return err
		}

		// get value of portfolio
		sums.TotalValue, err = getHoldingsValue(ctx, dataManager, holdings, date)
		if err != nil {
			return err
		}

		// generate a new list of holdings for current measurement date
		currentAssets, err := buildHoldingsList(ctx, dataManager, holdings, date, sums.TotalValue)
		if err != nil {
			return err
		}

		// update riskFreeValue
		sums.RiskFreeValue *= getRiskFreeRate(ctx, dataManager, date)

		measurement := PerformanceMeasurement{
			Time:                 date,
			Justification:        justificationArray,
			Value:                sums.TotalValue,
			BenchmarkValue:       sums.BenchmarkValue,
			RiskFreeValue:        sums.RiskFreeValue,
			StrategyGrowthOf10K:  prevMeasurement.StrategyGrowthOf10K,
			BenchmarkGrowthOf10K: prevMeasurement.BenchmarkGrowthOf10K,
			RiskFreeGrowthOf10K:  prevMeasurement.RiskFreeGrowthOf10K,
			Holdings:             currentAssets,
			TotalDeposited:       sums.Deposited,
			TotalWithdrawn:       sums.Withdrawn,
		}

		perf.Measurements = append(perf.Measurements, &measurement)

		if len(perf.Measurements) >= 2 {

			// time-weighted rate of return
			if int(dates.DaysToStartOfYear) == len(perf.Measurements) {
				dates.DaysToStartOfYear--
			}

			perf.calculateGrowthOf10k(&measurement)
			perf.calculateReturns(&measurement, dates)
			perf.calculateRiskMeasures(&measurement)
		}

		perf.updateAnnualPerformance(prevDate, date, prevMeasurement, ytdBench)
		ytdBench = float32(perf.TWRR(dates.DaysToStartOfYear, BENCHMARK))

		fmt.Printf("%s\t%.2f\t%.2f\n", measurement.Time.Format("2006-01-02"), measurement.StrategyGrowthOf10K, measurement.Value)

		prevMeasurement = &measurement
		prevDate = date

		if date.Before(today) || date.Equal(today) {
			perf.CurrentAssets = currentAssets
		}
	}

	// Update Strategy Metrics
	perf.updateSummaryMetrics(perf.PortfolioMetrics, STRATEGY)
	perf.updateSummaryMetrics(perf.BenchmarkMetrics, BENCHMARK)

	// calculate dynamic, perpetual, and safe withdrawal rates
	perf.calculateWithdrawalRates()

	// Set period end to last measurement
	if len(perf.Measurements) > 0 {
		perf.PeriodStart = perf.Measurements[0].Time
		perf.PeriodEnd = perf.Measurements[len(perf.Measurements)-1].Time
	}

	return nil
}

// DATABASE

// LOAD

func LoadPerformanceFromDB(portfolioID uuid.UUID, userID string) (*Performance, error) {
	subLog := log.With().Str("PortfolioID", portfolioID.String()).Str("UserID", userID).Logger()
	portfolioSQL := `SELECT performance_bytes FROM portfolios WHERE id=$1 AND user_id=$2`
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Err(err).Msg("unable to get database transaction for user")
		return nil, err
	}

	binaryID, err := portfolioID.MarshalBinary()
	if err != nil {
		return nil, err
	}

	portfolio := &Portfolio{
		ID:        binaryID,
		StartDate: time.Now(),
	}

	p := NewPerformance(portfolio)

	var data []byte
	err = trx.QueryRow(context.Background(), portfolioSQL, portfolioID, userID).Scan(&data)

	if err != nil {
		subLog.Warn().Err(err).Msg("query database for performance failed")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return nil, err
	}

	if err := trx.Commit(context.Background()); err != nil {
		subLog.Warn().Err(err).Msg("commit transaction failed")
		return nil, err
	}

	if err := p.UnmarshalBinary(data); err != nil {
		subLog.Warn().Err(err).Msg("unmarshal data failed")
		return nil, err
	}

	return p, nil
}

// loadMeasurementsFromDB populates the measurements array with values from the database
func (perf *Performance) LoadMeasurementsFromDB(userID string) error {
	subLog := log.With().Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(perf.PortfolioID)).Logger()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	measurementSQL := "SELECT event_date, strategy_value, risk_free_value, holdings, benchmark_value, strategy_growth_of_10k, benchmark_growth_of_10k, risk_free_growth_of_10k, total_deposited_to_date, total_withdrawn_to_date FROM portfolio_measurements WHERE portfolio_id=$1 AND user_id=$2 ORDER BY event_date"
	rows, err := trx.Query(context.Background(), measurementSQL, perf.PortfolioID, userID)
	if err != nil {
		subLog.Warn().Err(err).Str("Query", measurementSQL).Msg("failed executing measurement query")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	measurements := make([]*PerformanceMeasurement, 0, 1000)
	for rows.Next() {
		m := PerformanceMeasurement{}
		err := rows.Scan(&m.Time, &m.Value, &m.RiskFreeValue, &m.Holdings, &m.BenchmarkValue, &m.StrategyGrowthOf10K, &m.BenchmarkGrowthOf10K, &m.RiskFreeGrowthOf10K, &m.TotalDeposited, &m.TotalWithdrawn)
		if err != nil {
			subLog.Warn().Err(err).Str("Query", measurementSQL).Msg("failed to scan PerformanceMeasurement row in DB query")
			if err := trx.Rollback(context.Background()); err != nil {
				log.Error().Err(err).Msg("could not rollback transaction")
			}

			return err
		}
		measurements = append(measurements, &m)
	}
	perf.Measurements = measurements

	if err := trx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("could not commit transaction to the database")
	}
	return nil
}

// SAVE

func (perf *Performance) Save(userID string) error {
	subLog := log.With().Str("UserID", userID).Str("PortfolioID", hex.EncodeToString(perf.PortfolioID)).Logger()
	trx, err := database.TrxForUser(userID)
	if err != nil {
		subLog.Error().Err(err).Msg("unable to get database transaction for user")
		return err
	}

	err = perf.SaveWithTransaction(trx, userID)
	if err != nil {
		subLog.Error().Err(err).Msg("unable to save portfolio transactions")
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	err = trx.Commit(context.Background())
	return err
}

func (perf *Performance) SaveWithTransaction(trx pgx.Tx, userID string) error {
	sql := `UPDATE portfolios SET
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

	tmp := perf.Measurements
	perf.Measurements = make([]*PerformanceMeasurement, 0)
	raw, err := perf.MarshalBinary()
	if err != nil {
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}
	perf.Measurements = tmp

	maxDrawDown := 0.0
	if len(perf.DrawDowns) > 0 {
		maxDrawDown = perf.DrawDowns[0].LossPercent
	}

	_, err = trx.Exec(context.Background(), sql,
		perf.PortfolioID,
		raw,
		perf.PortfolioReturns.TWRRYTD,
		perf.PortfolioReturns.TWRRSinceInception,
		perf.PortfolioReturns.TWRRThreeYear,
		perf.PortfolioReturns.TWRRFiveYear,
		perf.PortfolioReturns.TWRRTenYear,
		perf.PortfolioMetrics.StdDevSinceInception,
		perf.PortfolioMetrics.DownsideDeviationSinceInception,
		maxDrawDown,
		perf.PortfolioMetrics.AvgDrawDown,
		perf.PortfolioMetrics.SharpeRatioSinceInception,
		perf.PortfolioMetrics.SortinoRatioSinceInception,
		perf.PortfolioMetrics.UlcerIndexAvg)
	if err != nil {
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	err = perf.saveMeasurements(trx, userID)
	if err != nil {
		if err := trx.Rollback(context.Background()); err != nil {
			log.Error().Err(err).Msg("could not rollback transaction")
		}

		return err
	}

	return nil
}

func (perf *Performance) saveMeasurements(trx pgx.Tx, userID string) error {
	sql := `INSERT INTO portfolio_measurements (
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
	) ON CONFLICT ON CONSTRAINT portfolio_measurements_pkey
	DO NOTHING`

	for _, m := range perf.Measurements {
		holdings, err := json.Marshal(m.Holdings)
		if err != nil {
			for _, holding := range m.Holdings {
				log.Error().Str("Ticker", holding.Ticker).Float64("Shares", holding.Shares).Float32("PercentPorfolio", holding.PercentPortfolio).Float64("Value", holding.Value).Msg("holding")
			}
			return ErrSerialize
		}

		justification, err := json.Marshal(m.Justification)
		if err != nil {
			log.Warn().Err(err).Msg("failed to serialize justification")
			return ErrSerialize
		}

		_, err = trx.Exec(context.Background(), sql,
			m.Time,                  // 1
			perf.PortfolioID,        // 2
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
			log.Error().Err(err).Msg("could not save portfolio measurement")
			return err
		}
	}

	return nil
}
