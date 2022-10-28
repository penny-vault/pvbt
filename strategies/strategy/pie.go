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

package strategy

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/penny-vault/pv-api/data"
)

type Pie struct {
	Date           time.Time
	Members        map[data.Security]float64
	Justifications map[string]float64
}

type PieList []*Pie

// Last returns the last item in the PieHistory
func (pl PieList) Last() *Pie {
	lastIdx := len(pl) - 1
	if lastIdx >= 0 {
		return pl[lastIdx]
	}

	return nil
}

// Trim the PieHistory to only cover the time period between begin and end (inclusive)
func (pl PieList) Trim(begin, end time.Time) PieList {
	// special case 0: requested range is invalid
	if end.Before(begin) {
		return PieList{}
	}

	// special case 1: pie history is empty
	if len(pl) == 0 {
		return pl
	}

	// special case 2: end time is before pie history start
	if end.Before(pl[0].Date) {
		return PieList{}
	}

	// special case 3: start time is after pie history end
	if begin.After(pl.Last().Date) {
		return PieList{}
	}

	// Use binary search to find the index corresponding to the start and end times
	beginIdx := sort.Search(len(pl), func(i int) bool {
		idxVal := pl[i].Date
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(pl), func(i int) bool {
		idxVal := pl[i].Date
		return (idxVal.After(end) || idxVal.Equal(end))
	})

	if endIdx != len(pl) {
		endIdx += 1
	}

	return pl[beginIdx:endIdx]
}

// StartDate returns the starting date of the pie history
func (pl PieList) StartDate() time.Time {
	if len(pl) == 0 {
		return time.Time{}
	}

	return pl[0].Date
}

// EndDate returns the ending date of the pie history
func (pl PieList) EndDate() time.Time {
	last := pl.Last()
	if last != nil {
		return last.Date
	}
	return time.Time{}
}

// Table prints an ASCII formated table to stdout
func (pl PieList) Table() {
	if len(pl) == 0 {
		return // nothing to do as there is no data available in the pie history
	}

	// construct table header
	tableCols := []string{"Date", "ticker", "Qty"}
	justCols := []string{}
	for title := range pl[0].Justifications {
		tableCols = append(tableCols, title)
		justCols = append(justCols, title)
	}

	// initialize table
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(tableCols)
	table.SetBorder(false) // Set Border to false

	for _, pie := range pl {
		for security, qty := range pie.Members {
			row := []string{pie.Date.Format("2006-01-02"), security.Ticker, fmt.Sprintf("%.2f", qty)}
			for _, col := range justCols {
				row = append(row, fmt.Sprintf("%.2f", pie.Justifications[col]))
			}
			table.Append(row)
		}
	}

	table.Render()
}
