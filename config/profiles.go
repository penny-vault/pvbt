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

package config

// ptrFloat64 returns a pointer to the given float64 value.
func ptrFloat64(val float64) *float64 { return &val }

// ptrInt returns a pointer to the given int value.
func ptrInt(val int) *int { return &val }

// ProfileBaseline returns the canonical RiskConfig baseline for a named
// profile. The values here must stay in sync with the risk package profiles.
// An empty name or "none" returns a zero RiskConfig.
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

// ResolveProfile merges the profile baseline with any explicit overrides from
// cfg.Risk. Non-nil override fields take precedence over the baseline.
func (cfg *Config) ResolveProfile() RiskConfig {
	baseline := ProfileBaseline(cfg.Risk.Profile)
	rc := cfg.Risk

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
