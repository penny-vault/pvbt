// Copyright 2021-2022
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

	"github.com/penny-vault/pv-api/data"
)

type Pie struct {
	Members        map[data.Security]float64
	Justifications map[string]float64
}

type PieHistory struct {
	Dates []time.Time
	Pies  []*Pie
}

type PieHistoryIterator struct {
	CurrentIndex int
	History      *PieHistory
}

func (ph *PieHistory) Iterator() *PieHistoryIterator {
	return &PieHistoryIterator{
		CurrentIndex: -1,
		History:      ph,
	}
}

func (iter *PieHistoryIterator) Next() bool {
	iter.CurrentIndex++
	return iter.CurrentIndex >= len(iter.History.Dates)
}

func (iter *PieHistoryIterator) Date() time.Time {
	if iter.CurrentIndex < 0 || iter.CurrentIndex >= len(iter.History.Dates) {
		return time.Time{}
	}
	return iter.History.Dates[iter.CurrentIndex]
}

func (iter *PieHistoryIterator) Val() *Pie {
	if iter.CurrentIndex < 0 || iter.CurrentIndex >= len(iter.History.Dates) {
		return nil
	}
	return iter.History.Pies[iter.CurrentIndex]
}

func (ph *PieHistory) StartDate() time.Time {
	if len(ph.Dates) > 0 {
		return ph.Dates[0]
	}
	return time.Time{}
}

func (ph *PieHistory) EndDate() time.Time {
	if len(ph.Dates) > 0 {
		return ph.Dates[len(ph.Dates)-1]
	}
	return time.Time{}
}
