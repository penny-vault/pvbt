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

package data

// Metric identifies an externally-sourced measurement about an asset or the economy.
type Metric string

// Market data metrics.
const (
	Price  Metric = "Price"
	Volume Metric = "Volume"
	Bid    Metric = "Bid"
	Ask    Metric = "Ask"
)

// Fundamental metrics.
const (
	Revenue          Metric = "Revenue"
	NetIncome        Metric = "NetIncome"
	EarningsPerShare Metric = "EarningsPerShare"
	TotalDebt        Metric = "TotalDebt"
	TotalAssets      Metric = "TotalAssets"
	FreeCashFlow     Metric = "FreeCashFlow"
	BookValue        Metric = "BookValue"
	MarketCap        Metric = "MarketCap"
)

// Economic indicator metrics.
const (
	Unemployment Metric = "Unemployment"
)
