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

import (
	"github.com/penny-vault/pvbt/asset"
)

// HarvesterConfig configures the TaxLossHarvester middleware.
type HarvesterConfig struct {
	// LossThreshold is the minimum unrealized loss percentage to trigger
	// harvesting (e.g., 0.05 for 5%).
	LossThreshold float64

	// GainOffsetOnly restricts harvesting to situations where realized
	// gains exist to offset. When true, the harvester does nothing if
	// RealizedGainsYTD returns zero gains.
	GainOffsetOnly bool

	// Substitutes maps an original asset to a correlated substitute.
	// When a substitute is configured, the harvester can sell the
	// original and buy the substitute even inside a wash sale window.
	Substitutes map[asset.Asset]asset.Asset
}
