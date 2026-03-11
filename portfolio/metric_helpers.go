// Copyright 2021-2026
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

package portfolio

import (
	"math"
	"time"
)

// returns computes period-over-period returns from a price series.
// Returns empty slice for single-element input.
func returns(prices []float64) []float64 {
	if len(prices) < 2 {
		return []float64{}
	}

	r := make([]float64, len(prices)-1)
	for i := 0; i < len(prices)-1; i++ {
		r[i] = (prices[i+1] - prices[i]) / prices[i]
	}

	return r
}

// excessReturns subtracts risk-free returns from portfolio returns element-wise.
func excessReturns(r, rf []float64) []float64 {
	n := len(r)
	if len(rf) < n {
		n = len(rf)
	}

	er := make([]float64, n)
	for i := 0; i < n; i++ {
		er[i] = r[i] - rf[i]
	}

	return er
}

// windowSlice trims a series to the trailing window based on timestamps.
// If window is nil, returns the full series.
func windowSlice(series []float64, times []time.Time, window *Period) []float64 {
	if window == nil {
		return series
	}

	if len(times) == 0 {
		return series
	}

	last := times[len(times)-1]

	var cutoff time.Time
	switch window.Unit {
	case UnitDay:
		cutoff = last.AddDate(0, 0, -window.N)
	case UnitMonth:
		cutoff = last.AddDate(0, -window.N, 0)
	case UnitYear:
		cutoff = last.AddDate(-window.N, 0, 0)
	}

	for i, t := range times {
		if !t.Before(cutoff) {
			return series[i:]
		}
	}

	return series
}

// mean computes the arithmetic mean.
func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range x {
		sum += v
	}

	return sum / float64(len(x))
}

// variance computes the sample variance (using N-1 denominator).
func variance(x []float64) float64 {
	n := len(x)
	if n < 2 {
		return 0
	}

	m := mean(x)
	sum := 0.0
	for _, v := range x {
		d := v - m
		sum += d * d
	}

	return sum / float64(n-1)
}

// stddev computes the sample standard deviation (using N-1 denominator).
func stddev(x []float64) float64 {
	return math.Sqrt(variance(x))
}

// covariance computes the sample covariance between x and y.
func covariance(x, y []float64) float64 {
	n := len(x)
	if len(y) < n {
		n = len(y)
	}

	if n < 2 {
		return 0
	}

	mx := mean(x[:n])
	my := mean(y[:n])

	sum := 0.0
	for i := 0; i < n; i++ {
		sum += (x[i] - mx) * (y[i] - my)
	}

	return sum / float64(n-1)
}

// cagr computes the compound annual growth rate.
func cagr(startValue, endValue, years float64) float64 {
	if startValue <= 0 || endValue <= 0 || years <= 0 {
		return 0
	}

	return math.Pow(endValue/startValue, 1.0/years) - 1
}

// windowSliceTimes trims a time series to the trailing window.
// If window is nil, returns the full series.
func windowSliceTimes(times []time.Time, window *Period) []time.Time {
	if window == nil || len(times) == 0 {
		return times
	}

	last := times[len(times)-1]

	var cutoff time.Time
	switch window.Unit {
	case UnitDay:
		cutoff = last.AddDate(0, 0, -window.N)
	case UnitMonth:
		cutoff = last.AddDate(0, -window.N, 0)
	case UnitYear:
		cutoff = last.AddDate(-window.N, 0, 0)
	}

	for i, t := range times {
		if !t.Before(cutoff) {
			return times[i:]
		}
	}

	return times
}

// annualizationFactor estimates the number of periods per year from timestamps.
// If the average gap between timestamps exceeds 20 calendar days, it assumes
// monthly data (factor 12); otherwise it assumes daily (factor 252).
func annualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 252 // default daily
	}
	avgDays := times[len(times)-1].Sub(times[0]).Hours() / 24 / float64(len(times)-1)
	if avgDays > 20 {
		return 12 // monthly
	}
	return 252 // daily
}

// roundTrip represents a completed buy-sell pair matched via FIFO.
type roundTrip struct {
	pnl      float64
	holdDays float64
}

// roundTrips builds round-trip trades from transactions using FIFO matching.
// It also returns the total sell value for turnover calculation.
func roundTrips(txns []Transaction) ([]roundTrip, float64) {
	type openLot struct {
		date  time.Time
		qty   float64
		price float64
	}

	openLots := make(map[string][]openLot) // keyed by CompositeFigi
	var trips []roundTrip
	var totalSellValue float64

	for _, tx := range txns {
		key := tx.Asset.CompositeFigi
		switch tx.Type {
		case BuyTransaction:
			openLots[key] = append(openLots[key], openLot{
				date:  tx.Date,
				qty:   tx.Qty,
				price: tx.Price,
			})
		case SellTransaction:
			totalSellValue += tx.Price * tx.Qty
			remaining := tx.Qty
			lots := openLots[key]
			for len(lots) > 0 && remaining > 0 {
				matched := lots[0].qty
				if matched > remaining {
					matched = remaining
				}
				pnl := (tx.Price - lots[0].price) * matched
				days := tx.Date.Sub(lots[0].date).Hours() / 24.0
				trips = append(trips, roundTrip{pnl: pnl, holdDays: days})

				lots[0].qty -= matched
				remaining -= matched
				if lots[0].qty == 0 {
					lots = lots[1:]
				}
			}
			openLots[key] = lots
		}
	}

	return trips, totalSellValue
}

// realizedGains replays the transaction log with FIFO lot matching to
// compute realized long-term capital gains, short-term capital gains,
// qualified dividend income, and non-qualified dividend income.
func realizedGains(txns []Transaction) (ltcg, stcg, qualDiv, nonQualDiv float64) {
	type lot struct {
		date  time.Time
		qty   float64
		price float64
	}

	lots := make(map[string][]lot) // keyed by CompositeFigi

	for _, tx := range txns {
		key := tx.Asset.CompositeFigi
		switch tx.Type {
		case BuyTransaction:
			lots[key] = append(lots[key], lot{
				date:  tx.Date,
				qty:   tx.Qty,
				price: tx.Price,
			})
		case SellTransaction:
			remaining := tx.Qty
			ll := lots[key]
			i := 0
			for i < len(ll) && remaining > 0 {
				matched := ll[i].qty
				if matched > remaining {
					matched = remaining
				}
				gain := (tx.Price - ll[i].price) * matched
				holdingDays := tx.Date.Sub(ll[i].date).Hours() / 24
				if holdingDays > 365 {
					ltcg += gain
				} else {
					stcg += gain
				}
				if ll[i].qty <= remaining {
					remaining -= ll[i].qty
					i++
				} else {
					ll[i].qty -= remaining
					remaining = 0
				}
			}
			lots[key] = ll[i:]
		case DividendTransaction:
			if tx.Qualified {
				qualDiv += tx.Amount
			} else {
				nonQualDiv += tx.Amount
			}
		}
	}

	return
}

// drawdownSeries computes the drawdown at each point in the equity curve.
// Values are negative (or zero at peaks).
func drawdownSeries(equity []float64) []float64 {
	dd := make([]float64, len(equity))
	peak := math.Inf(-1)

	for i, v := range equity {
		if v > peak {
			peak = v
		}
		dd[i] = (v - peak) / peak
	}

	return dd
}
