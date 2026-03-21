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

// LotSelection determines which tax lots are consumed when selling a position.
type LotSelection int

const (
	// LotFIFO sells the earliest-acquired lots first (default).
	LotFIFO LotSelection = iota
	// LotLIFO sells the most-recently-acquired lots first.
	LotLIFO
	// LotHighestCost sells the lot with the highest cost basis first,
	// producing the largest realized loss when the position is underwater.
	LotHighestCost
	// LotSpecificID sells a specific lot identified by ID.
	LotSpecificID
)
