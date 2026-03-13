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

import "time"

// AnnualizationFactor estimates periods-per-year from timestamps.
// If the average gap exceeds 20 calendar days, returns 12 (monthly);
// otherwise returns 252 (daily). Defaults to 252 for fewer than 2 timestamps.
func AnnualizationFactor(times []time.Time) float64 {
	if len(times) < 2 {
		return 252
	}
	avgDays := times[len(times)-1].Sub(times[0]).Hours() / 24 / float64(len(times)-1)
	if avgDays > 20 {
		return 12
	}
	return 252
}
