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

type mwrr struct{}

func (mwrr) Name() string { return "MWRR" }

func (mwrr) Description() string {
	return "Money-weighted rate of return (internal rate of return). Measures the actual return experienced by the investor, accounting for the timing and size of cash flows. Sensitive to when deposits and withdrawals occur."
}

// Compute returns the money-weighted (XIRR) annual rate of return.
// Cash flows: deposits are negative (investor pays in), withdrawals are
// positive (investor receives). The final portfolio value is added as a
// positive terminal cash flow. Newton-Raphson is used to find the rate r
// such that sum(cf_i / (1+r)^(t_i/365)) = 0.
func (mwrr) Compute(a *Account, window *Period) (float64, error) {
	equity := a.EquityCurve()
	times := a.EquityTimes()
	if len(equity) < 2 {
		return 0, nil
	}

	// Build cash flow list from transactions.
	type cashFlow struct {
		date   time.Time
		amount float64
	}

	var flows []cashFlow

	startDate := times[0]

	for _, tx := range a.Transactions() {
		switch tx.Type {
		case DepositTransaction:
			// From investor perspective, deposits are outflows (negative).
			d := tx.Date
			if d.IsZero() {
				d = startDate
			}
			flows = append(flows, cashFlow{date: d, amount: -tx.Amount})
		case WithdrawalTransaction:
			// Withdrawals have negative Amount in the tx log; from investor
			// perspective they are inflows (positive), so negate again.
			d := tx.Date
			if d.IsZero() {
				d = startDate
			}
			flows = append(flows, cashFlow{date: d, amount: -tx.Amount})
		}
	}

	// If there are zero external flows, use a synthetic initial outflow
	// equal to the first equity value.
	if len(flows) == 0 {
		flows = append(flows, cashFlow{date: startDate, amount: -equity[0]})
	}

	// Terminal cash flow: ending portfolio value (positive, investor could
	// liquidate).
	endDate := times[len(times)-1]
	endValue := equity[len(equity)-1]
	flows = append(flows, cashFlow{date: endDate, amount: endValue})

	// Reference date for day offsets.
	t0 := flows[0].date

	// NPV(r) = sum(cf_i / (1+r)^(d_i/365))
	npv := func(r float64) float64 {
		sum := 0.0
		for _, cf := range flows {
			days := cf.date.Sub(t0).Hours() / 24.0
			sum += cf.amount / math.Pow(1+r, days/365.0)
		}
		return sum
	}

	// NPV'(r) = sum(-cf_i * (d_i/365) / (1+r)^(d_i/365 + 1))
	npvDeriv := func(r float64) float64 {
		sum := 0.0
		for _, cf := range flows {
			days := cf.date.Sub(t0).Hours() / 24.0
			exp := days / 365.0
			sum += -cf.amount * exp / math.Pow(1+r, exp+1)
		}
		return sum
	}

	// Newton-Raphson with initial guess of 10%.
	r := 0.10
	for i := 0; i < 100; i++ {
		f := npv(r)
		fp := npvDeriv(r)
		if math.Abs(fp) < 1e-15 {
			break
		}
		rNew := r - f/fp
		if math.Abs(rNew-r) < 1e-12 {
			r = rNew
			break
		}
		r = rNew
	}

	if math.IsNaN(r) || math.IsInf(r, 0) {
		return 0, nil
	}

	return r, nil
}

// ComputeSeries returns nil; MWRR is a single scalar metric.
func (mwrr) ComputeSeries(a *Account, window *Period) ([]float64, error) { return nil, nil }

// MWRR is the money-weighted rate of return: accounts for the timing
// and size of cash flows (deposits/withdrawals) using XIRR. Unlike
// TWRR, this metric reflects the investor's actual experience.
var MWRR PerformanceMetric = mwrr{}
