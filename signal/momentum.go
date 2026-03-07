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

package signal

import "github.com/penny-vault/pvbt/data"

// Momentum computes the percent change over the given number of periods
// for each asset in the DataFrame. The input DataFrame must contain
// price data. Returns a DataFrame with one column per asset containing
// the momentum score at each timestamp.
func Momentum(df *data.DataFrame, periods int) *data.DataFrame {
	// Compute percent change over the lookback period for each
	// asset's price column. Return a new DataFrame with a single
	// metric ("Momentum") and the same assets and timestamps.
	return nil
}
