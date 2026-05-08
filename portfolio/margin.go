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

	"github.com/penny-vault/pvbt/data"
)

const (
	defaultInitialMarginRate     = 0.50
	defaultMaintenanceMarginRate = 0.30
	defaultMaxLeverage           = 1.0
)

// ShortMarketValue returns the total absolute market value of all short
// positions (negative holdings). Returns 0 if prices have not been set.
func (a *Account) ShortMarketValue() float64 {
	if a.prices == nil {
		return 0
	}

	var total float64

	for ast, qty := range a.holdings {
		if qty < 0 {
			price := a.prices.Value(ast, data.MetricClose)
			if !math.IsNaN(price) {
				total += math.Abs(qty) * price
			}
		}
	}

	return total
}

// LongMarketValue returns the total market value of all long positions
// (positive holdings). Returns 0 if prices have not been set.
func (a *Account) LongMarketValue() float64 {
	if a.prices == nil {
		return 0
	}

	var total float64

	for ast, qty := range a.holdings {
		if qty > 0 {
			price := a.prices.Value(ast, data.MetricClose)
			if !math.IsNaN(price) {
				total += qty * price
			}
		}
	}

	return total
}

// Equity returns cash plus long market value minus short market value.
func (a *Account) Equity() float64 {
	return a.cash + a.LongMarketValue() - a.ShortMarketValue()
}

// MarginRatio returns equity divided by short market value. Returns NaN
// if there are no short positions.
func (a *Account) MarginRatio() float64 {
	smv := a.ShortMarketValue()
	if smv == 0 {
		return math.NaN()
	}

	return a.Equity() / smv
}

// MarginDeficiency returns the dollar amount of position notional that
// must be unwound to restore margin compliance. It is the worst of two
// breaches: the short-side maintenance margin shortfall (notional of
// shorts to cover so SMV*maintenanceRate <= equity), and the gross
// leverage shortfall (notional to close so (LMV+SMV)/equity <= the
// configured cap). Returns 0 if the account is healthy.
func (a *Account) MarginDeficiency() float64 {
	equity := a.Equity()

	var shortDeficit float64

	smv := a.ShortMarketValue()
	if smv > 0 {
		rate := a.maintenanceMarginRate()
		if rate > 0 {
			needed := smv - equity/rate
			if needed > 0 {
				shortDeficit = needed
			}
		}
	}

	var leverageDeficit float64

	maxLev := a.MaxLeverage()
	gross := a.LongMarketValue() + smv

	if maxLev > 0 && gross > 0 {
		if equity <= 0 {
			leverageDeficit = gross
		} else if gross/equity > maxLev {
			leverageDeficit = gross - maxLev*equity
		}
	}

	if shortDeficit > leverageDeficit {
		return shortDeficit
	}

	return leverageDeficit
}

// GrossLeverage returns (LongMarketValue + ShortMarketValue) / Equity.
// Returns 0 if there are no positions, or NaN if equity is non-positive
// while positions exist (an irrecoverable account state).
func (a *Account) GrossLeverage() float64 {
	gross := a.LongMarketValue() + a.ShortMarketValue()
	if gross == 0 {
		return 0
	}

	equity := a.Equity()
	if equity <= 0 {
		return math.NaN()
	}

	return gross / equity
}

// MaxLeverage returns the configured gross-leverage cap, or the default
// of 1.0 if none was set.
func (a *Account) MaxLeverage() float64 {
	if a.maxLeverage > 0 {
		return a.maxLeverage
	}

	return defaultMaxLeverage
}

// SetMaxLeverage sets the gross-leverage cap. Used by the engine to
// apply a strategy- or CLI-supplied value when the account itself
// hasn't been configured with WithMaxLeverage. Values <= 0 leave the
// existing setting unchanged.
func (a *Account) SetMaxLeverage(ratio float64) {
	if ratio > 0 {
		a.maxLeverage = ratio
	}
}

// HasMaxLeverage reports whether WithMaxLeverage (or SetMaxLeverage)
// has been used to configure a non-default cap.
func (a *Account) HasMaxLeverage() bool {
	return a.maxLeverage > 0
}

// LeverageHeadroom returns the additional notional (in dollars) that
// can be opened before the gross-leverage cap is breached. Returns a
// negative value when the account is already over the cap.
func (a *Account) LeverageHeadroom() float64 {
	equity := a.Equity()
	gross := a.LongMarketValue() + a.ShortMarketValue()

	return a.MaxLeverage()*equity - gross
}

// BuyingPower returns cash minus the initial margin reserved for
// short positions.
func (a *Account) BuyingPower() float64 {
	return a.cash - a.ShortMarketValue()*a.initialMarginRate()
}

// initialMarginRate returns the configured initial margin rate, or the
// default of 0.50 if none was set.
func (a *Account) initialMarginRate() float64 {
	if a.initialMargin > 0 {
		return a.initialMargin
	}

	return defaultInitialMarginRate
}

// maintenanceMarginRate returns the configured maintenance margin rate,
// or the default of 0.30 if none was set.
func (a *Account) maintenanceMarginRate() float64 {
	if a.maintenanceMargin > 0 {
		return a.maintenanceMargin
	}

	return defaultMaintenanceMarginRate
}
