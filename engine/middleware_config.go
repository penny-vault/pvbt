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

import (
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/risk"
	"github.com/penny-vault/pvbt/tax"
)

// buildMiddlewareFromConfig constructs risk and tax middleware from the
// engine's middleware config and registers them on the account.
func (e *Engine) buildMiddlewareFromConfig() error {
	cfg := e.middlewareConfig
	resolved := cfg.ResolveProfile()

	var middlewares []portfolio.Middleware

	// Fixed ordering per spec:
	// 1. VolatilityScaler
	// 2. MaxPositionSize
	// 3. MaxPositionCount
	// 4. GrossExposureLimit
	// 5. NetExposureLimit
	// 6. DrawdownCircuitBreaker
	// 7. Tax middleware

	if resolved.VolatilityScalerLookback != nil {
		middlewares = append(middlewares, risk.VolatilityScaler(e, *resolved.VolatilityScalerLookback))
	}

	if resolved.MaxPositionSize != nil {
		middlewares = append(middlewares, risk.MaxPositionSize(*resolved.MaxPositionSize))
	}

	if resolved.MaxPositionCount != nil {
		middlewares = append(middlewares, risk.MaxPositionCount(*resolved.MaxPositionCount))
	}

	if resolved.GrossExposureLimit != nil {
		middlewares = append(middlewares, risk.GrossExposureLimit(*resolved.GrossExposureLimit))
	}

	if resolved.NetExposureLimit != nil {
		middlewares = append(middlewares, risk.NetExposureLimit(*resolved.NetExposureLimit))
	}

	if resolved.DrawdownCircuitBreaker != nil {
		middlewares = append(middlewares, risk.DrawdownCircuitBreaker(*resolved.DrawdownCircuitBreaker))
	}

	// Tax middleware
	if cfg.Tax.Enabled {
		harvesterCfg := tax.HarvesterConfig{
			LossThreshold:  cfg.Tax.LossThreshold,
			GainOffsetOnly: cfg.Tax.GainOffsetOnly,
			DataSource:     e,
		}

		if len(cfg.Tax.Substitutes) > 0 {
			harvesterCfg.Substitutes = make(map[asset.Asset]asset.Asset, len(cfg.Tax.Substitutes))
			for fromTicker, toTicker := range cfg.Tax.Substitutes {
				// Viper lowercases TOML keys; uppercase for ticker resolution.
				fromAsset := e.Asset(strings.ToUpper(fromTicker))
				toAsset := e.Asset(strings.ToUpper(toTicker))
				harvesterCfg.Substitutes[fromAsset] = toAsset
			}
		}

		middlewares = append(middlewares, tax.NewTaxLossHarvester(harvesterCfg))
	}

	// Clear any existing middleware and apply config-driven stack.
	e.account.ClearMiddleware()

	if len(middlewares) > 0 {
		e.account.Use(middlewares...)
	}

	return nil
}
