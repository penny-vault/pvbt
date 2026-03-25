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
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/engine/middleware/risk"
	"github.com/penny-vault/pvbt/engine/middleware/tax"
	"github.com/penny-vault/pvbt/portfolio"
)

// MiddlewareConfig holds the complete middleware configuration for a run.
type MiddlewareConfig struct {
	Risk risk.RiskConfig
	Tax  tax.TaxConfig
}

// ValidateAndApplyDefaults checks that all configuration values are within
// acceptable bounds and fills in defaults where needed.
func (cfg *MiddlewareConfig) ValidateAndApplyDefaults() error {
	if err := cfg.Risk.Validate(); err != nil {
		return fmt.Errorf("risk config: %w", err)
	}

	if err := cfg.Tax.ApplyDefaults(); err != nil {
		return fmt.Errorf("tax config: %w", err)
	}

	return nil
}

// HasMiddleware reports whether the configuration results in any middleware
// being applied. It returns false only when profile is "none" (or empty) and
// no individual risk overrides are set and tax is disabled.
func (cfg *MiddlewareConfig) HasMiddleware() bool {
	if cfg.Tax.Enabled {
		return true
	}

	rc := cfg.Risk

	if rc.Profile != "" && rc.Profile != "none" {
		return true
	}
	// profile is "" or "none" -- check for explicit overrides
	if rc.MaxPositionSize != nil ||
		rc.MaxPositionCount != nil ||
		rc.DrawdownCircuitBreaker != nil ||
		rc.VolatilityScalerLookback != nil ||
		rc.GrossExposureLimit != nil ||
		rc.NetExposureLimit != nil {
		return true
	}

	return false
}

// buildMiddlewareFromConfig constructs risk and tax middleware from the
// engine's middleware config and registers them on the account.
func (e *Engine) buildMiddlewareFromConfig() error {
	cfg := e.middlewareConfig
	resolved := cfg.Risk.Resolve()

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
