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

import "github.com/penny-vault/pvbt/data"

// Selector filters a DataFrame down to only the assets that should be
// held at each timestep. The returned DataFrame has the same structure
// as the input but with unselected assets removed or marked. This is
// the first step in portfolio construction -- selection happens before
// weighting.
type Selector interface {
	Select(df *data.DataFrame) *data.DataFrame
}
