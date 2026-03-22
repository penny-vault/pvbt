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

package study

import (
	"fmt"
	"time"
)

// Numeric constrains SweepRange to integer and floating-point types.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ParamSweep describes how to vary a single strategy parameter across runs.
type ParamSweep struct {
	field    string
	values   []string
	isPreset bool
}

// Field returns the name of the parameter field being swept.
func (ps ParamSweep) Field() string { return ps.field }

// Values returns the list of string-encoded values for this sweep.
func (ps ParamSweep) Values() []string { return ps.values }

// IsPreset reports whether this sweep varies named presets rather than a single field.
func (ps ParamSweep) IsPreset() bool { return ps.isPreset }

// SweepRange generates values from min to max (inclusive) with the given step.
func SweepRange[T Numeric](field string, min, max, step T) ParamSweep {
	var values []string
	for val := min; val <= max; val += step {
		values = append(values, fmt.Sprintf("%v", val))
	}

	return ParamSweep{field: field, values: values}
}

// SweepDuration generates duration values from min to max with the given step.
func SweepDuration(field string, min, max, step time.Duration) ParamSweep {
	var values []string
	for val := min; val <= max; val += step {
		values = append(values, val.String())
	}

	return ParamSweep{field: field, values: values}
}

// SweepValues provides explicit string values for a field.
func SweepValues(field string, values ...string) ParamSweep {
	return ParamSweep{field: field, values: values}
}

// SweepPresets varies named parameter presets.
func SweepPresets(presets ...string) ParamSweep {
	return ParamSweep{field: "", values: presets, isPreset: true}
}

// CrossProduct combines base configs with sweeps to produce the full run matrix.
func CrossProduct(base []RunConfig, sweeps []ParamSweep) []RunConfig {
	if len(sweeps) == 0 {
		return base
	}

	result := make([]RunConfig, len(base))
	copy(result, base)

	for _, sweep := range sweeps {
		var expanded []RunConfig

		for _, cfg := range result {
			for _, val := range sweep.Values() {
				newCfg := cloneRunConfig(cfg)

				if sweep.IsPreset() {
					newCfg.Preset = val
					newCfg.Name = appendName(newCfg.Name, val)
				} else {
					if newCfg.Params == nil {
						newCfg.Params = make(map[string]string)
					}

					newCfg.Params[sweep.Field()] = val
					newCfg.Name = appendName(newCfg.Name, fmt.Sprintf("%s=%s", sweep.Field(), val))
				}

				expanded = append(expanded, newCfg)
			}
		}

		result = expanded
	}

	return result
}

// cloneRunConfig deep copies a RunConfig, including its Params and Metadata maps.
func cloneRunConfig(cfg RunConfig) RunConfig {
	cloned := cfg

	if cfg.Params != nil {
		cloned.Params = make(map[string]string, len(cfg.Params))
		for key, val := range cfg.Params {
			cloned.Params[key] = val
		}
	}

	if cfg.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(cfg.Metadata))
		for key, val := range cfg.Metadata {
			cloned.Metadata[key] = val
		}
	}

	return cloned
}

// appendName joins an existing name with a suffix using " / " as separator.
// If the existing name is empty, the suffix is returned as-is.
func appendName(existing, suffix string) string {
	if existing == "" {
		return suffix
	}

	return existing + " / " + suffix
}
