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
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
)

type maxAboveZero struct {
	fallback []asset.Asset
}

// Select filters the DataFrame to the single asset with the highest
// signal value above zero at each timestep. If no assets have a
// positive value at a given timestep, the fallback assets are selected
// instead.
func (m maxAboveZero) Select(df *data.DataFrame) *data.DataFrame { return nil }

// MaxAboveZero returns a Selector that picks the asset with the highest
// signal value above zero. If no assets qualify, the fallback assets
// are used.
func MaxAboveZero(fallback []asset.Asset) Selector {
	return maxAboveZero{fallback: fallback}
}
