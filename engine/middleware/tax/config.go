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

package tax

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

// ApplyDefaults fills in default values where needed.
func (tc *TaxConfig) ApplyDefaults() error {
	if tc.Enabled && tc.LossThreshold == 0 {
		tc.LossThreshold = DefaultLossThreshold
	}

	return nil
}
