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

import "fmt"

// TradeDirection indicates whether a round-trip trade was a long or short position.
type TradeDirection int

const (
	TradeLong TradeDirection = iota
	TradeShort
)

// String returns "Long" or "Short".
func (d TradeDirection) String() string {
	switch d {
	case TradeLong:
		return "Long"
	case TradeShort:
		return "Short"
	default:
		return fmt.Sprintf("TradeDirection(%d)", int(d))
	}
}
