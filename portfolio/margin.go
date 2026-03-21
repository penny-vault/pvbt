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

// MarginDeficiency returns the dollar amount needed to restore
// maintenance margin. Returns 0 if the account is healthy or has no
// short positions.
func (a *Account) MarginDeficiency() float64 {
	smv := a.ShortMarketValue()
	if smv == 0 {
		return 0
	}

	requiredEquity := smv * (1 + a.maintenanceMarginRate())
	deficit := requiredEquity - a.Equity()

	if deficit > 0 {
		return deficit
	}

	return 0
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
