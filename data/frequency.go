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

// Frequency represents data publication frequency.
type Frequency int

const (
	Tick Frequency = iota
	Daily
	Weekly
	Monthly
	Quarterly
	Yearly
)

func (f Frequency) String() string {
	switch f {
	case Tick:
		return "Tick"
	case Daily:
		return "Daily"
	case Weekly:
		return "Weekly"
	case Monthly:
		return "Monthly"
	case Quarterly:
		return "Quarterly"
	case Yearly:
		return "Yearly"
	default:
		return fmt.Sprintf("Frequency(%d)", int(f))
	}
}
