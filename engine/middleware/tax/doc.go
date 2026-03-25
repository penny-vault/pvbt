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

// Package tax provides portfolio middleware for tax-loss harvesting.
//
// [NewTaxLossHarvester] returns a [portfolio.Middleware] that scans
// positions for unrealized losses exceeding a configurable threshold
// and injects sell orders to realize those losses. When substitute
// assets are configured, the harvester buys the substitute to maintain
// market exposure while respecting the 30-day wash-sale window.
//
// # Configuration
//
// [HarvesterConfig] controls harvesting behavior:
//
//   - LossThreshold: minimum unrealized loss (as a fraction of cost
//     basis) before harvesting triggers (e.g., 0.05 for 5%).
//   - GainOffsetOnly: when true, only harvest losses up to the amount
//     needed to offset year-to-date realized gains.
//   - Substitutes: a map from asset to its substitute for maintaining
//     exposure after harvesting (e.g., SPY -> VOO).
//   - DataSource: provides current prices for computing unrealized P&L.
//
// # Usage
//
//	harvester := tax.NewTaxLossHarvester(tax.HarvesterConfig{
//	    LossThreshold: 0.05,
//	    Substitutes:   map[asset.Asset]asset.Asset{spy: voo},
//	    DataSource:    eng,
//	})
//	acct.Use(harvester)
//
// Or use the convenience function:
//
//	acct.Use(tax.TaxEfficient(config)...)
//
// # Config-file approach
//
// Tax middleware can also be configured through a TOML config file
// (pvbt.toml) or --tax CLI flag without modifying Go code.
// See docs/configuration.md for details.
package tax
