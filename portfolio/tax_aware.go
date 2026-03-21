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

package portfolio

import (
	"time"

	"github.com/penny-vault/pvbt/asset"
)

// TaxAware provides tax-lot-level access for tax optimization middleware.
// The concrete Account type implements this interface. Tax middleware
// type-asserts the batch's Portfolio reference to TaxAware; strategies
// and risk middleware only see Portfolio.
type TaxAware interface {
	WashSaleWindow(ast asset.Asset) []WashSaleRecord
	UnrealizedLots(ast asset.Asset) []TaxLot
	RealizedGainsYTD() (ltcg, stcg float64)
	RegisterSubstitution(original, substitute asset.Asset, until time.Time)
	ActiveSubstitutions() map[asset.Asset]Substitution
}

// Substitution records an active asset substitution for tax purposes.
type Substitution struct {
	Original   asset.Asset
	Substitute asset.Asset
	Until      time.Time
}
