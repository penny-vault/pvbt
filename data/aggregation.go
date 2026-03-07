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

import "fmt"

// Aggregation determines how values within a period are combined during resampling.
type Aggregation int

const (
	Last Aggregation = iota
	First
	Sum
	Mean
	Max
	Min
)

// OHLC aliases.
const (
	Close = Last
	Open  = First
	High  = Max
	Low   = Min
)

func (a Aggregation) String() string {
	switch a {
	case Last:
		return "Last"
	case First:
		return "First"
	case Sum:
		return "Sum"
	case Mean:
		return "Mean"
	case Max:
		return "Max"
	case Min:
		return "Min"
	default:
		return fmt.Sprintf("Aggregation(%d)", int(a))
	}
}
