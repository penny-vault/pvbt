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
	"crypto/rand"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
	"gonum.org/v1/gonum/stat"
)

const (
	STRATEGY  = "STRATEGY"
	BENCHMARK = "BENCHMARK"
	RISKFREE  = "RISKFREE"
)

type cashflow struct {
	date  time.Time
	value float64
}

func isNaN(x float32) bool {
	return math.IsNaN(float64(x))
}

// Metric Functions

// ActiveReturn calculates the difference in return vs a benchmark
// this is considered the amount of return that the "active" management
// yielded. The value of this metric is highly dependent on appropriate
// selection of benchmark. For example, comparing a small-cap value fund to
// the S&P500 benchmark doesn't say much because the underlying return
// of the assets held does not match the S&P500 well.
func (perf *Performance) ActiveReturn(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	rP := perf.TWRR(periods, STRATEGY)
	rB := perf.TWRR(periods, BENCHMARK)

	return rP - rB
}

// alpha is a measure of excess return of a portfolio
// α = Rp – [Rf + (Rm – Rf) β]
func (perf *Performance) Alpha(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	rP := perf.TWRR(periods, STRATEGY)
	rF := perf.TWRR(periods, RISKFREE)
	rB := perf.TWRR(periods, BENCHMARK)
	b := perf.Beta(periods)

	return rP - (rF + (rB-rF)*b)
}

// AverageDrawDown computes the average portfolio draw down. A draw down
// is defined as the period in which a portfolio falls from its previous peak.
// Draw downs include the time period of the loss, percent of loss, and when
// the portfolio recovered
func (perf *Performance) AverageDrawDown(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	allDrawDowns := perf.AllDrawDowns(periods, kind)
	dd := make([]float64, len(allDrawDowns))
	for ii, xx := range allDrawDowns {
		dd[ii] = xx.LossPercent
	}
	return stat.Mean(dd, nil)
}

// AllDrawDowns computes all portfolio draw downs. A draw down
// is defined as the period in which a portfolio falls from its previous peak.
// Draw downs include the time period of the loss, percent of loss, and when
// the portfolio recovered
func (perf *Performance) AllDrawDowns(periods uint, kind string) []*DrawDown {
	allDrawDowns := []*DrawDown{}

	n := len(perf.Measurements)
	if periods < 2 {
		return allDrawDowns
	}

	if uint(n) <= periods {
		periods = uint(n) - 1
	}

	startIdx := len(perf.Measurements) - int(periods) - 1
	if startIdx < 0 {
		return allDrawDowns
	}

	m0 := perf.Measurements[startIdx]

	var peak float64
	switch kind {
	case STRATEGY:
		peak = m0.Value
	case BENCHMARK:
		peak = m0.BenchmarkValue
	case RISKFREE:
		peak = m0.RiskFreeValue
	}

	var drawDown *DrawDown
	var prev time.Time
	for _, v := range perf.Measurements[startIdx:] {
		var value float64
		switch kind {
		case STRATEGY:
			value = v.Value
		case BENCHMARK:
			value = v.BenchmarkValue
		case RISKFREE:
			value = v.RiskFreeValue
		}
		peak = math.Max(peak, value)
		diff := value - peak
		if diff < 0 {
			if drawDown == nil {
				drawDown = &DrawDown{
					Begin:       prev,
					End:         v.Time,
					LossPercent: float64((value / peak) - 1.0),
				}
			}

			loss := (value/peak - 1.0)
			if loss < drawDown.LossPercent {
				drawDown.End = v.Time
				drawDown.LossPercent = float64(loss)
			}
		} else if drawDown != nil {
			drawDown.Recovery = v.Time
			allDrawDowns = append(allDrawDowns, drawDown)
			drawDown = nil
		}
		prev = v.Time
	}

	return allDrawDowns
}

// AvgUlcerIndex compute average ulcer index over the last N periods
func (perf *Performance) AvgUlcerIndex(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	startIdx := len(perf.Measurements) - int(periods) - 1
	if startIdx < 0 {
		return math.NaN()
	}

	u := make([]float64, 0, len(perf.Measurements))
	for _, xx := range perf.Measurements[startIdx:] {
		if !isNaN(xx.UlcerIndex) {
			u = append(u, float64(xx.UlcerIndex))
		}
	}

	avgUlcerIndex := stat.Mean(u, nil)
	return avgUlcerIndex
}

// Beta is a measure of the volatility—or systematic risk—of a security or portfolio
// compared to the market as a whole. Beta is used in the capital asset pricing model
// (CAPM), which describes the relationship between systematic risk and expected
// return for assets (usually stocks). CAPM is widely used as a method for pricing
// risky securities and for generating estimates of the expected returns of assets,
// considering both the risk of those assets and the cost of capital.
func (perf *Performance) Beta(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	retA := perf.periodReturns(periods, STRATEGY)
	retB := perf.periodReturns(periods, BENCHMARK)

	sigma := stat.Covariance(retA, retB, nil)
	return sigma / stat.Variance(retB, nil)
}

// CalmarRatio is a gauge of the risk adjusted performance of a portfolio.
// It is a function of  the fund's average compounded annual rate of return
// versus its maximum drawdown. The higher the Calmar ratio, the better
// the portfolio performed on a risk-adjusted basis during the given time
// frame, which is typically set at 36 months.
func (perf *Performance) CalmarRatio(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	cagr := perf.TWRR(periods, kind)
	maxDrawDown := perf.MaxDrawDown(periods, kind)
	if maxDrawDown != nil {
		return cagr / (-1 * maxDrawDown.LossPercent)
	}
	return cagr
}

// DownsideDeviation compute the standard deviation of negative
// excess returns on a monthly basis, result is annualized
func (perf *Performance) DownsideDeviation(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	Rp := perf.monthlyReturns(periods, kind)
	Rf := perf.monthlyReturns(periods, RISKFREE)
	downside := 0.0
	for ii := range Rp {
		excessReturn := Rp[ii] - Rf[ii]
		if excessReturn < 0 {
			downside += excessReturn * excessReturn // much faster than math.Pow
		}
	}

	return math.Sqrt(downside/float64(len(Rp))) * math.Sqrt(12)
}

// DynamicWithdrawalRate calculates the maximum % that can be withdrawn per year and
// expect the balance to be greater than or equal to the inflation adjusted starting
// balance. Inflation should be provided as an annual rate.
func DynamicWithdrawalRate(mc [][]float64, inflation float64) float64 {
	rets := make([]float64, len(mc))
	final := 1_000_000 * math.Pow(1+.03, 29)
	for ii, xx := range mc {
		f := func(r float64) float64 { return dynamicWithdrawalRate(r, inflation, xx) - final }
		x0, err := fsolve(f, .05)
		if err != nil {
			// if it didn't converge just continue
			continue
		}
		rets[ii] = x0
	}
	return stat.Mean(rets, nil)
}

// ExcessKurtosis calculates the amount of kurtosis relative to the normal distribution.
// Kurtosis is a statistical measure that is used to describe the size of the tails on a
// distribution. Excess kurtosis helps determine how much risk is involved in a specific
// investment. It signals that the probability of obtaining an extreme outcome or value from
// the event in question is higher than would be found in a probabilistically normal
// distribution of outcomes.
func (perf *Performance) ExcessKurtosis(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	v := make([]float64, periods+1)
	startIdx := len(perf.Measurements) - int(periods) - 1
	if startIdx < 0 {
		return math.NaN()
	}

	idx := 0
	for _, xx := range perf.Measurements[startIdx:] {
		v[idx] = xx.Value
		idx++
	}
	return stat.ExKurtosis(v, nil)
}

// InformationRatio is a measurement of portfolio returns beyond the returns of the benchmark,
// compared to the volatility of those returns.
func (perf *Performance) InformationRatio(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	Rp := stat.Mean(perf.periodReturns(periods, STRATEGY), nil)
	Rb := stat.Mean(perf.periodReturns(periods, BENCHMARK), nil)

	excessReturn := Rp - Rb
	trackingError := perf.TrackingError(periods)

	ir := excessReturn / trackingError
	return ir * math.Sqrt(252)
}

// KellerRatio adjusts return for drawdown such as to reflect the severity
// of the observed maximum drawdown. In case maximum drawdown is small, the
// return adjustment is only limited. But with large maximum drawdown, the
// impact of the return adjustment is amplified.
//
// K = R * ( 1 - D / ( 1 - D ) ), if R >= 0% and D <= 50%, and K = 0% otherwise
func (perf *Performance) KellerRatio(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	cagr := perf.TWRR(periods, kind)
	maxDD := perf.MaxDrawDown(periods, kind)
	var d float64
	if maxDD != nil {
		d = (perf.MaxDrawDown(periods, kind)).LossPercent
	} else {
		d = 0
	}

	if cagr >= 0 && d <= .5 {
		return cagr * (1 - d/(1-d))
	}

	return 0
}

// KRatio The K-ratio is a valuation metric that examines the consistency of an equity's return over time.
// k-ratio = (Slope logVAMI regression line) / n(Standard Error of the Slope)
func (perf *Performance) KRatio(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	// log(VAMI)
	y := perf.vami(periods)
	x := make([]float64, len(y))
	for ii, v := range y {
		y[ii] = math.Log(v)
		x[ii] = float64(ii)
	}

	// linear regression
	slope, _ := stat.LinearRegression(x, y, nil, false)

	return slope / (float64(periods) * stat.StdErr(slope, float64(periods)/21))
}

// MaxDrawDown returns the largest drawdown over the given number of periods
func (perf *Performance) MaxDrawDown(periods uint, kind string) *DrawDown {
	if periods < 1 {
		return nil
	}

	n := len(perf.Measurements)
	if uint(n) < periods {
		periods = uint(n)
	}

	top10 := perf.Top10DrawDowns(periods, kind)

	if len(top10) > 1 {
		return top10[0]
	}

	return nil
}

// MWRR computes the money-weighted rate of return for the specified number of periods
// if periods = 2, then return (p1 - deposits + withdraws) / p0
// if 1 < periods < 252, then return xirr(cashflows) un-annualized
// else return xirr(cashflows) which is annualized return
func (perf *Performance) MWRR(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	pp := int(periods)

	rate := 1.0
	startIdx := len(perf.Measurements) - pp - 1
	endIdx := len(perf.Measurements) - 1
	if startIdx < 0 {
		return math.NaN()
	}

	start := perf.Measurements[startIdx].Time
	end := perf.Measurements[endIdx].Time
	duration := end.Sub(start)

	// if period is greater than a year then annualize the result
	years := toYears(duration)

	if periods == 1 {
		m0 := perf.Measurements[startIdx]
		m1 := perf.Measurements[endIdx]

		deposited := m1.TotalDeposited - m0.TotalDeposited
		withdrawn := m1.TotalWithdrawn - m0.TotalWithdrawn

		switch kind {
		case STRATEGY:
			rate = float64((m1.Value - deposited + withdrawn) / m0.Value)
		case BENCHMARK:
			rate = float64((m1.BenchmarkValue - deposited + withdrawn) / m0.BenchmarkValue)
		case RISKFREE:
			rate = float64((m1.RiskFreeValue - deposited + withdrawn) / m0.RiskFreeValue)
		}
		if years > 1 {
			return math.Pow(rate, 1.0/years) - 1.0
		}
		return rate - 1.0
	}

	cashflows := make([]cashflow, 0, 5)

	var val float64
	switch kind {
	case STRATEGY:
		val = float64(perf.Measurements[startIdx].Value) * -1.0
	case BENCHMARK:
		val = float64(perf.Measurements[startIdx].BenchmarkValue) * -1.0
	case RISKFREE:
		val = float64(perf.Measurements[startIdx].RiskFreeValue) * -1.0
	}

	cashflows = append(cashflows, cashflow{
		date:  perf.Measurements[startIdx].Time,
		value: val,
	})

	for ii, jj := startIdx, startIdx+1; jj < endIdx; ii, jj = ii+1, jj+1 {
		m0 := perf.Measurements[ii]
		m1 := perf.Measurements[jj]
		deposited := m1.TotalDeposited - m0.TotalDeposited
		withdrawn := m1.TotalWithdrawn - m0.TotalWithdrawn
		change := float64(deposited - withdrawn)
		if math.Abs(change) > 0 {
			cashflows = append(cashflows, cashflow{
				date:  perf.Measurements[jj].Time,
				value: change * -1.0,
			})
		}
	}

	switch kind {
	case STRATEGY:
		val = perf.Measurements[n-1].Value
	case BENCHMARK:
		val = perf.Measurements[n-1].BenchmarkValue
	case RISKFREE:
		val = perf.Measurements[n-1].RiskFreeValue
	}

	cashflows = append(cashflows, cashflow{
		date:  perf.Measurements[n-1].Time,
		value: val,
	})

	// performance optimization when there have been no cashflows over the period
	if len(cashflows) == 2 {
		rate = cashflows[1].value / (-1.0 * cashflows[0].value)
		if years > 1 {
			return math.Pow(rate, 1.0/years) - 1.0
		}
		return rate - 1.0
	}

	// regular MWRR with XIRR
	rate = xirr(cashflows) / 100
	if years < 1 {
		return math.Pow((1+rate), years) - 1.0
	}
	return rate
}

// MWRRYtd calculates the money weighted YTD return
func (perf *Performance) MWRRYtd(kind string) float64 {
	periods := perf.ytdPeriods()
	if len(perf.Measurements) == int(periods) {
		periods--
	}
	return perf.MWRR(periods, kind)
}

// NetProfit total profit earned on portfolio
func (perf *Performance) NetProfit() float64 {
	m1 := perf.Measurements[len(perf.Measurements)-1]
	return m1.Value - m1.TotalDeposited + m1.TotalWithdrawn
}

// NetProfitPercent profit earned on portfolio expressed as a percent
func (perf *Performance) NetProfitPercent() float64 {
	m1 := perf.Measurements[len(perf.Measurements)-1]
	return (m1.TotalDeposited+perf.NetProfit())/m1.TotalDeposited - 1.0
}

// PerpetualWithdrawalRate
func PerpetualWithdrawalRate(mc [][]float64, inflation float64) float64 {
	rets := make([]float64, len(mc))
	final := 1_000_000 * math.Pow(1+inflation, 29)
	for ii, xx := range mc {
		f := func(r float64) float64 { return constantWithdrawalRate(r, inflation, xx) - final }
		x0, err := fsolve(f, .05)
		if err == nil {
			rets[ii] = x0
		}
	}
	return stat.Mean(rets, nil)
}

// SafeWithdrawalRate
func SafeWithdrawalRate(mc [][]float64, inflation float64) float64 {
	rets := make([]float64, len(mc))
	for ii, xx := range mc {
		f := func(r float64) float64 { return constantWithdrawalRate(r, inflation, xx) }
		x0, err := fsolve(f, .05)
		if err != nil {
			log.Warn().Stack().Err(err).Msg("fsolve failed")
			continue
		}
		rets[ii] = x0
	}
	swr := stat.Mean(rets, nil)
	return swr
}

// SharpeRatio The ratio is the average return earned in excess of the risk-free
// rate per unit of volatility or total risk. Volatility is a measure of the price
// fluctuations of an asset or portfolio.
//
// Sharpe = (Rp - Rf) / (annualized std. dev)
//
// Monthly values are chosen here to remain consistent with
// Morningstar and other online data providers.
func (perf *Performance) SharpeRatio(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	Rp := perf.monthlyReturns(periods, kind)
	Rf := perf.monthlyReturns(periods, RISKFREE)
	excessReturn := 1.0
	for ii := range Rp {
		excessReturn *= (1.0 + Rp[ii] - Rf[ii])
	}
	stdev := stat.StdDev(Rp, nil) * math.Sqrt(12.0)

	startIdx := (len(perf.Measurements) - int(periods) - 1)
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := len(perf.Measurements) - 1
	years := toYears(perf.Measurements[endIdx].Time.Sub(perf.Measurements[startIdx].Time))
	if years > 1.0 {
		// annualize
		excessReturn = math.Pow(excessReturn, 1.0/years) - 1.0
	} else {
		excessReturn -= 1.0
	}

	sharpe := excessReturn / stdev

	return sharpe
}

// Skew computes the skew of the portfolio measurements relative to the normal distribution
func (perf *Performance) Skew(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	rets := perf.monthlyReturns(periods, kind)
	return stat.Skew(rets, nil)
}

// SortinoRatio a variation of the Sharpe ratio that differentiates harmful
// volatility from total overall volatility by using the asset's standard deviation
// of negative portfolio returns—downside deviation—instead of the total standard
// deviation of portfolio returns. The Sortino ratio takes an asset or portfolio's
// return and subtracts the risk-free rate, and then divides that amount by the
// asset's downside deviation.
//
// Calculation is based on this paper by Red Rock Capital
// http://www.redrockcapital.com/Sortino__A__Sharper__Ratio_Red_Rock_Capital.pdf
func (perf *Performance) SortinoRatio(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	Rp := perf.periodReturns(periods, kind)
	Rf := perf.periodReturns(periods, RISKFREE)

	excessReturn := 1.0
	for ii := range Rp {
		excessReturn *= 1.0 + Rp[ii] - Rf[ii]
	}

	startIdx := (len(perf.Measurements) - int(periods) - 1)
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := len(perf.Measurements) - 1
	years := toYears(perf.Measurements[endIdx].Time.Sub(perf.Measurements[startIdx].Time))
	if years > 1.0 {
		// annualize
		excessReturn = math.Pow(excessReturn, 1.0/years) - 1.0
	} else {
		excessReturn -= 1.0
	}

	downsideDeviation := perf.DownsideDeviation(periods, kind)
	sortino := excessReturn / downsideDeviation

	return sortino
}

// StdDev calculates the annualized standard deviation based off of
// the monthly price changes. Monthly values are chosen here to remain
// consistent with Morningstar and other online data providers.
func (perf *Performance) StdDev(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	rets := perf.monthlyReturns(periods, kind)
	return stat.StdDev(rets, nil) * math.Sqrt(12.0)
}

// Top10DrawDowns computes the top 10 portfolio draw downs. A draw down
// is defined as the period in which a portfolio falls from its previous peak.
// Draw downs include the time period of the loss, percent of loss, and when
// the portfolio recovered
func (perf *Performance) Top10DrawDowns(periods uint, kind string) []*DrawDown {
	n := len(perf.Measurements)
	if len(perf.Measurements) == 0 || uint(n) < periods {
		return []*DrawDown{}
	}

	allDrawDowns := perf.AllDrawDowns(periods, kind)

	sort.Slice(allDrawDowns, func(i, j int) bool {
		return allDrawDowns[i].LossPercent < allDrawDowns[j].LossPercent
	})

	return allDrawDowns[0:minInt(10, len(allDrawDowns))]
}

// TrackingError is the divergence between the price behavior of a portfolio and
// the price behavior of a benchmark.
func (perf *Performance) TrackingError(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	Rp := perf.periodReturns(periods, STRATEGY)
	Rb := perf.periodReturns(periods, BENCHMARK)

	excessReturns := make([]float64, len(Rp))
	for ii := range Rp {
		excessReturns[ii] = Rp[ii] - Rb[ii]
	}

	return stat.StdDev(excessReturns, nil)
}

// TreynorRatio also known as the reward-to-volatility ratio, is a performance
// metric for determining how much excess return was generated for each unit of risk
// taken on by a portfolio.
// treynor = Excess Return / Beta
func (perf *Performance) TreynorRatio(periods uint) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	excessReturn := perf.excessReturn(periods)
	return stat.Mean(excessReturn, nil) / perf.Beta(periods)
}

// TWRR computes the time-weighted rate of return for the specified number of periods
func (perf *Performance) TWRR(periods uint, kind string) float64 {
	n := len(perf.Measurements)
	if (periods+1) > uint(n) || periods < 1 {
		return math.NaN()
	}

	pp := int(periods)

	rate := 1.0
	startIdx := len(perf.Measurements) - pp - 1
	endIdx := len(perf.Measurements) - 1
	if startIdx < 0 {
		return math.NaN()
	}

	for ii, jj := startIdx, startIdx+1; jj < n; ii, jj = ii+1, jj+1 {
		s := perf.Measurements[ii]
		e := perf.Measurements[jj]
		deposit := e.TotalDeposited - s.TotalDeposited
		withdraw := e.TotalWithdrawn - s.TotalWithdrawn
		var sValue float64
		var eValue float64
		switch kind {
		case STRATEGY:
			sValue = s.Value
			eValue = e.Value
		case BENCHMARK:
			sValue = s.BenchmarkValue
			eValue = e.BenchmarkValue
		case RISKFREE:
			sValue = s.RiskFreeValue
			eValue = e.RiskFreeValue
		}
		r0 := (eValue - deposit + withdraw) / sValue
		rate *= r0
	}

	start := perf.Measurements[startIdx].Time
	end := perf.Measurements[endIdx].Time
	duration := end.Sub(start)

	// if period is greater than a year then annualize the result
	years := toYears(duration)
	if years > 1 {
		return math.Pow(rate, 1.0/years) - 1
	}

	return rate - 1
}

// TWRRYtd calculates the time-weighted YTD return
func (perf *Performance) TWRRYtd(kind string) float64 {
	periods := perf.ytdPeriods()
	if len(perf.Measurements) == int(periods) {
		periods--
	}
	return perf.TWRR(periods, kind)
}

// UlcerIndex The Ulcer Index (UI) is a technical indicator that measures downside
// risk in terms of both the depth and duration of price declines. The index
// increases in value as the price moves farther away from a recent high and falls as
// the price rises to new highs. The indicator is usually calculated over a 14-day
// period, with the Ulcer Index showing the percentage drawdown a trader can expect
// from the high over that period.
//
// The greater the value of the Ulcer Index, the longer it takes for a stock to get
// back to the former high. Simply stated, it is designed as one measure of
// volatility only on the downside.
//
// Percentage Drawdown = [(Close - 14-period High Close)/14-period High Close] x 100
// Squared Average = (14-period Sum of Percentage Drawdown Squared)/14
// Ulcer Index = Square Root of Squared Average
//
// period is number of days to lookback
func (perf *Performance) UlcerIndex() float64 {
	period := 14
	N := len(perf.Measurements)

	if N < period {
		return math.NaN()
	}

	lookback := make([]float64, 0, period)
	m := perf.Measurements

	for _, xx := range m[(len(m) - period):] {
		lookback = append(lookback, xx.StrategyGrowthOf10K)
	}

	// Find max close over period
	maxClose := lookback[0]
	var sqSum float64
	for _, yy := range lookback {
		if yy > maxClose {
			maxClose = yy
		}
		percentDrawDown := ((yy - maxClose) / maxClose) * 100
		sqSum += percentDrawDown * percentDrawDown
	}
	sqAvg := sqSum / float64(period)
	return math.Sqrt(sqAvg)
}

// UlcerIndexPercentile compute average ulcer index over the last N periods
func (perf *Performance) UlcerIndexPercentile(periods uint, percentile float64) float64 {
	n := len(perf.Measurements)
	if periods > uint(n) || periods < 1 {
		return math.NaN()
	}

	if percentile > 1.0 || percentile < 0.0 {
		return math.NaN()
	}

	startIdx := len(perf.Measurements) - int(periods) - 1
	if startIdx < 0 {
		return math.NaN()
	}

	u := make([]float64, 0, len(perf.Measurements))
	for _, xx := range perf.Measurements[startIdx:] {
		u = append(u, float64(xx.UlcerIndex))
	}

	sort.Float64s(u)

	cnt := len(u)
	percentileIdx := minInt(int(math.Ceil(float64(cnt)*percentile))-1, len(u)-1)
	if percentileIdx < 0 {
		percentileIdx = 0
	}

	return u[percentileIdx]
}

// RSquared

// ValueAtRisk

// UpsideCaptureRatio

// DownsideCaptureRatio

// NPositivePeriods

// GainLossRatio

// HELPER FUNCTIONS

func (perf *Performance) periodReturns(periods uint, kind string) []float64 {
	n := len(perf.Measurements)
	pp := int(periods)
	rets := make([]float64, 0, periods)
	startIdx := len(perf.Measurements) - pp - 1
	if startIdx < 0 {
		return nil
	}

	for ii, jj := startIdx, startIdx+1; jj < n; ii, jj = ii+1, jj+1 {
		s := perf.Measurements[ii]
		e := perf.Measurements[jj]
		deposit := e.TotalDeposited - s.TotalDeposited
		withdraw := e.TotalWithdrawn - s.TotalWithdrawn
		var sValue float64
		var eValue float64
		switch kind {
		case STRATEGY:
			sValue = s.Value
			eValue = e.Value
		case BENCHMARK:
			sValue = s.BenchmarkValue
			eValue = e.BenchmarkValue
		case RISKFREE:
			sValue = s.RiskFreeValue
			eValue = e.RiskFreeValue
		}
		r0 := (eValue-deposit+withdraw)/sValue - 1.0
		rets = append(rets, r0)
	}
	return rets
}

// CircularBootstrap returns n arrays if length m of bootstrapped values
// from timeSeries
func CircularBootstrap(timeSeries []float64, blockSize int, n int, m int) [][]float64 {
	// construct blocks of requested length
	N := len(timeSeries)
	blocks := make([][]float64, N)
	for ii := range timeSeries {
		block := make([]float64, blockSize)
		for jj := range block {
			idx := (ii + jj) % N
			block[jj] = timeSeries[idx]
		}
		blocks[ii] = block
	}

	// sample blocks with replacement
	result := make([][]float64, n)
	bigN := big.NewInt(int64(N))
	for ii := range result {
		sample := make([]float64, 0, m)
		for len(sample) < m {
			idx, err := rand.Int(rand.Reader, bigN)
			if err != nil {
				log.Panic().Err(err).Msg("could not get random number")
			}
			sample = append(sample, blocks[idx.Int64()]...)
		}
		result[ii] = monthlyReturnToAnnual(sample[:m])
	}

	return result
}

func constantWithdrawalRate(rate float64, inflation float64, mc []float64) float64 {
	b := 1_000_000.0
	w := b * rate
	for _, ret := range mc {
		b = b*(1.0+ret) - w
		w *= (1.0 + inflation)
	}
	return b
}

func dynamicWithdrawalRate(rate float64, inflation float64, mc []float64) float64 {
	b := 1_000_000.0
	w0 := b * rate
	w := w0
	for _, ret := range mc {
		b = b*(1.0+ret) - w
		w0 *= (1.0 + inflation)
		w = min(w0, b*rate)
	}
	return b
}

// excessReturn compute the rate of return that is in excess of the risk free rate
func (perf *Performance) excessReturn(periods uint) []float64 {
	rets := make([]float64, 0, periods)

	Rp := perf.periodReturns(periods, STRATEGY)
	Rf := perf.periodReturns(periods, RISKFREE)

	for ii, xx := range Rp {
		rets = append(rets, xx-Rf[ii])
	}
	return rets
}

func min(x, y float64) float64 {
	if x < y {
		return x
	}
	return y
}

func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func (perf *Performance) monthlyReturns(periods uint, kind string) []float64 {
	rets := make([]float64, 0, 360)
	m := perf.Measurements
	startIdx := (len(m) - int(periods) - 1)
	if startIdx < 0 {
		startIdx = 0
	}
	lastMonth := perf.Measurements[startIdx].Time.Month()
	last := perf.Measurements[startIdx]
	prev := perf.Measurements[startIdx]
	for _, curr := range m[startIdx:] {
		t := curr.Time
		if lastMonth != t.Month() {
			var r float64
			switch kind {
			case STRATEGY:
				r = (prev.StrategyGrowthOf10K / last.StrategyGrowthOf10K) - 1.0
			case BENCHMARK:
				r = (prev.BenchmarkGrowthOf10K / last.BenchmarkGrowthOf10K) - 1.0
			case RISKFREE:
				r = (prev.RiskFreeGrowthOf10K / last.RiskFreeGrowthOf10K) - 1.0
			}
			last = prev
			lastMonth = t.Month()
			rets = append(rets, r)
		}
		prev = curr
	}
	return rets
}

// monthlyReturnToAnnual converts monthly returns into annual returns
func monthlyReturnToAnnual(ts []float64) []float64 {
	cnt := 0
	val := 1.0
	res := make([]float64, 0, int(math.Ceil(float64(len(ts))/12.0)))
	for _, x := range ts {
		cnt++
		val *= (1 + x)
		if cnt == 12 {
			val -= 1.0
			res = append(res, val)
			val = 1.0
			cnt = 0
		}
	}
	return res
}

// vami returns the hypothetical return of a $1,000 investment over the
// given number of periods
func (perf *Performance) vami(periods uint) []float64 {
	rP := perf.periodReturns(periods-1, STRATEGY)
	v := make([]float64, periods)
	v[0] = 1000
	for ii, r := range rP {
		v[ii+1] = v[ii] * (1 + r)
	}
	return v
}

// xirr returns the Internal Rate of Return (IRR) for an irregular series of cash flows
func xirr(cashflows []cashflow) float64 {
	if len(cashflows) == 0 {
		log.Error().Stack().Msg("cashflows cannot be 0")
		return 0
	}

	var years []float64
	for _, cf := range cashflows {
		years = append(years, (cf.date.Sub(cashflows[0].date).Hours()/24)/365)
	}

	residual := 1.0
	step := 0.05
	guess := 0.1
	epsilon := 0.0001
	limit := 10000

	for math.Abs(residual) > epsilon && limit > 0 {
		limit--

		residual = 0.0

		for i, cf := range cashflows {
			residual += cf.value / math.Pow(guess, years[i])
		}

		if math.Abs(residual) > epsilon {
			if residual > 0 {
				guess += step
			} else {
				guess -= step
				step /= 2.0
			}
		}
	}

	return math.Round(((guess-1)*100)*100) / 100
}

func (perf *Performance) ytdPeriods() uint {
	today := time.Now()
	year := today.Year()
	var periods uint
	for _, x := range perf.Measurements {
		if year == x.Time.Year() {
			periods++
		}
	}
	return periods
}

func toYears(d time.Duration) float64 {
	return d.Hours() / (24 * 365.2425)
}
