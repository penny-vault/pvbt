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

package engine

import "github.com/penny-vault/pvbt/portfolio"

// applyMarginConfig resolves margin settings onto the account in this
// precedence order (lowest to highest): account default, strategy
// Describe() scalars, engine WithMarginModel preset, engine
// WithMaxLeverage / WithGrossMaintenanceLeverage per-knob overrides.
// Account values configured directly via portfolio options take effect
// at construction time and are not overwritten unless an engine option
// or strategy descriptor explicitly supplies a value.
func applyMarginConfig(eng *Engine, acct portfolio.PortfolioManager) {
	var (
		descMax   float64
		descMaint float64
	)

	if desc, ok := eng.strategy.(Descriptor); ok {
		description := desc.Describe()
		descMax = description.MaxLeverage
		descMaint = description.GrossMaintenanceLeverage
	}

	// Strategy-supplied scalars (only when the account has not already
	// been configured via portfolio options).
	if descMax > 0 && !acct.HasMaxLeverage() {
		acct.SetMaxLeverage(descMax)
	}

	if descMaint > 0 && !acct.HasGrossMaintenanceLeverage() {
		acct.SetGrossMaintenanceLeverage(descMaint)
	}

	// WithMarginModel preset (engine option).
	if eng.marginModel != nil {
		if eng.marginModel.Initial > 0 {
			acct.SetMaxLeverage(1.0 / eng.marginModel.Initial)
		}

		if eng.marginModel.Maintenance > 0 {
			acct.SetGrossMaintenanceLeverage(1.0 / eng.marginModel.Maintenance)
		}
	}

	// Per-knob overrides (engine options) take precedence over presets.
	if eng.maxLeverage > 0 {
		acct.SetMaxLeverage(eng.maxLeverage)
	}

	if eng.grossMaintenanceLeverage > 0 {
		acct.SetGrossMaintenanceLeverage(eng.grossMaintenanceLeverage)
	}
}
