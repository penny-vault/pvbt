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

// Package config holds the configuration types and validation logic for
// risk and tax middleware that can be specified via CLI flags or config file.
package config

import "fmt"

// Config holds the complete middleware configuration for a run.
type Config struct {
	Risk RiskConfig
	Tax  TaxConfig
}

// RiskConfig holds risk middleware configuration parameters.
type RiskConfig struct {
	Profile                  string   `mapstructure:"profile"`
	MaxPositionSize          *float64 `mapstructure:"max_position_size"`
	MaxPositionCount         *int     `mapstructure:"max_position_count"`
	DrawdownCircuitBreaker   *float64 `mapstructure:"drawdown_circuit_breaker"`
	VolatilityScalerLookback *int     `mapstructure:"volatility_scaler_lookback"`
	GrossExposureLimit       *float64 `mapstructure:"gross_exposure_limit"`
	NetExposureLimit         *float64 `mapstructure:"net_exposure_limit"`
}

// TaxConfig holds tax-loss harvesting middleware configuration.
type TaxConfig struct {
	Enabled        bool              `mapstructure:"enabled"`
	LossThreshold  float64           `mapstructure:"loss_threshold"`
	GainOffsetOnly bool              `mapstructure:"gain_offset_only"`
	Substitutes    map[string]string `mapstructure:"substitutes"`
}

// DefaultLossThreshold is applied when tax harvesting is enabled and no
// explicit threshold is set.
const DefaultLossThreshold = 0.05

// validProfiles is the set of recognized risk profile names.
var validProfiles = map[string]bool{
	"":             true,
	"conservative": true,
	"moderate":     true,
	"aggressive":   true,
	"none":         true,
}

// ValidateAndApplyDefaults checks that all configuration values are within
// acceptable bounds and fills in defaults where needed.
func (cfg *Config) ValidateAndApplyDefaults() error {
	if err := cfg.Risk.validate(); err != nil {
		return fmt.Errorf("risk config: %w", err)
	}

	if err := cfg.Tax.applyDefaults(); err != nil {
		return fmt.Errorf("tax config: %w", err)
	}

	return nil
}

func (rc *RiskConfig) validate() error {
	if !validProfiles[rc.Profile] {
		return fmt.Errorf("unknown profile %q: must be one of conservative, moderate, aggressive, none, or empty", rc.Profile)
	}

	if rc.MaxPositionSize != nil && (*rc.MaxPositionSize < 0 || *rc.MaxPositionSize > 1.0) {
		return fmt.Errorf("max_position_size must be between 0 and 1.0, got %g", *rc.MaxPositionSize)
	}

	if rc.MaxPositionCount != nil && *rc.MaxPositionCount < 0 {
		return fmt.Errorf("max_position_count must be >= 0, got %d", *rc.MaxPositionCount)
	}

	if rc.DrawdownCircuitBreaker != nil && (*rc.DrawdownCircuitBreaker < 0 || *rc.DrawdownCircuitBreaker > 1.0) {
		return fmt.Errorf("drawdown_circuit_breaker must be between 0 and 1.0, got %g", *rc.DrawdownCircuitBreaker)
	}

	if rc.VolatilityScalerLookback != nil && *rc.VolatilityScalerLookback < 1 {
		return fmt.Errorf("volatility_scaler_lookback must be >= 1, got %d", *rc.VolatilityScalerLookback)
	}

	if rc.GrossExposureLimit != nil && *rc.GrossExposureLimit < 0 {
		return fmt.Errorf("gross_exposure_limit must be >= 0, got %g", *rc.GrossExposureLimit)
	}

	if rc.NetExposureLimit != nil && *rc.NetExposureLimit < 0 {
		return fmt.Errorf("net_exposure_limit must be >= 0, got %g", *rc.NetExposureLimit)
	}

	return nil
}

func (tc *TaxConfig) applyDefaults() error {
	if tc.Enabled && tc.LossThreshold == 0 {
		tc.LossThreshold = DefaultLossThreshold
	}

	return nil
}

// HasMiddleware reports whether the configuration results in any middleware
// being applied. It returns false only when profile is "none" (or empty) and
// no individual risk overrides are set and tax is disabled.
func (cfg *Config) HasMiddleware() bool {
	if cfg.Tax.Enabled {
		return true
	}

	rc := cfg.Risk

	if rc.Profile != "" && rc.Profile != "none" {
		return true
	}
	// profile is "" or "none" — check for explicit overrides
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
