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

package risk

import "fmt"

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

// validProfiles is the set of recognized risk profile names.
var validProfiles = map[string]bool{
	"":             true,
	"conservative": true,
	"moderate":     true,
	"aggressive":   true,
	"none":         true,
}

// Validate checks that all configuration values are within acceptable bounds.
func (rc *RiskConfig) Validate() error {
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

// ptrFloat64 returns a pointer to the given float64 value.
func ptrFloat64(val float64) *float64 { return &val }

// ptrInt returns a pointer to the given int value.
func ptrInt(val int) *int { return &val }

// ProfileBaseline returns the canonical RiskConfig baseline for a named
// profile. The values here must stay in sync with the risk profile functions
// (Conservative, Moderate, Aggressive). An empty name or "none" returns a
// zero RiskConfig.
func ProfileBaseline(name string) RiskConfig {
	switch name {
	case "conservative":
		return RiskConfig{
			Profile:                  "conservative",
			MaxPositionSize:          ptrFloat64(0.20),
			DrawdownCircuitBreaker:   ptrFloat64(0.10),
			VolatilityScalerLookback: ptrInt(60),
		}
	case "moderate":
		return RiskConfig{
			Profile:                "moderate",
			MaxPositionSize:        ptrFloat64(0.25),
			DrawdownCircuitBreaker: ptrFloat64(0.15),
		}
	case "aggressive":
		return RiskConfig{
			Profile:                "aggressive",
			MaxPositionSize:        ptrFloat64(0.35),
			DrawdownCircuitBreaker: ptrFloat64(0.25),
		}
	default:
		return RiskConfig{Profile: name}
	}
}

// Resolve merges the profile baseline with any explicit overrides from rc.
// Non-nil override fields take precedence over the baseline.
func (rc *RiskConfig) Resolve() RiskConfig {
	baseline := ProfileBaseline(rc.Profile)

	if rc.MaxPositionSize != nil {
		baseline.MaxPositionSize = rc.MaxPositionSize
	}

	if rc.MaxPositionCount != nil {
		baseline.MaxPositionCount = rc.MaxPositionCount
	}

	if rc.DrawdownCircuitBreaker != nil {
		baseline.DrawdownCircuitBreaker = rc.DrawdownCircuitBreaker
	}

	if rc.VolatilityScalerLookback != nil {
		baseline.VolatilityScalerLookback = rc.VolatilityScalerLookback
	}

	if rc.GrossExposureLimit != nil {
		baseline.GrossExposureLimit = rc.GrossExposureLimit
	}

	if rc.NetExposureLimit != nil {
		baseline.NetExposureLimit = rc.NetExposureLimit
	}

	return baseline
}
