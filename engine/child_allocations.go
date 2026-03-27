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

package engine

import (
	"fmt"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/portfolio"
)

// ChildAllocations returns an Allocation that expands each child strategy's
// current portfolio holdings into underlying asset weights, scaled by the
// child's target weight. Call with no arguments to use declared weights,
// or pass a map to override weights dynamically.
func (eng *Engine) ChildAllocations(overrides ...map[string]float64) (portfolio.Allocation, error) {
	if len(eng.children) == 0 {
		return portfolio.Allocation{}, nil
	}

	weights := make(map[string]float64, len(eng.children))
	for _, child := range eng.children {
		weights[child.name] = child.weight
	}

	if len(overrides) > 0 && overrides[0] != nil {
		for childName, childWeight := range overrides[0] {
			weights[childName] = childWeight
		}
	}

	// Validate weight sum.
	totalWeight := 0.0
	for _, childWeight := range weights {
		totalWeight += childWeight
	}

	if totalWeight > 1.0+1e-9 {
		return portfolio.Allocation{}, fmt.Errorf(
			"engine: child weights sum to %.4f, must be at most 1.0", totalWeight)
	}

	members := make(map[asset.Asset]float64)

	for _, child := range eng.children {
		childWeight := weights[child.name]
		if childWeight == 0 {
			continue
		}

		childValue := child.account.Value()
		if childValue == 0 {
			members[asset.CashAsset] += childWeight
			continue
		}

		cashFraction := child.account.Cash() / childValue
		if cashFraction > 0 {
			members[asset.CashAsset] += childWeight * cashFraction
		}

		for held := range child.account.Holdings() {
			posValue := child.account.PositionValue(held)
			posWeight := posValue / childValue

			members[held] += childWeight * posWeight
		}
	}

	return portfolio.Allocation{
		Date:    eng.currentDate,
		Members: members,
	}, nil
}

// ChildPortfolios returns the child strategy portfolios keyed by name.
func (eng *Engine) ChildPortfolios() map[string]portfolio.Portfolio {
	result := make(map[string]portfolio.Portfolio, len(eng.children))
	for _, child := range eng.children {
		result[child.name] = child.account
	}

	return result
}
