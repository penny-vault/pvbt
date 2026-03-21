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

package asset

// Asset represents a single tradeable instrument.
type Asset struct {
	CompositeFigi string
	Ticker        string
}

// EconomicIndicator is a sentinel asset for metrics not tied to a specific instrument.
var EconomicIndicator = Asset{Ticker: "$ECONOMIC_INDICATOR"}

// CashAsset is a sentinel asset representing uninvested cash in a portfolio.
// Used by ChildAllocations to represent a child strategy's cash position.
var CashAsset = Asset{Ticker: "$CASH", CompositeFigi: "$CASH"}
