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

func (ph *PieHistory) Trim(begin, end time.Time) *PieHistory {
	// special case 0: requested range is invalid
	if end.Before(begin) {
		ph.Dates = []time.Time{}
		ph.Pies = []*Pie{}
		return ph
	}

	// special case 1: pie history is empty
	if len(ph.Dates) == 0 {
		return ph
	}

	// special case 2: end time is before pie history start
	if end.Before(ph.Dates[0]) {
		ph.Dates = []time.Time{}
		ph.Pies = []*Pie{}
		return ph
	}

	// special case 3: start time is after pie history end
	if begin.After(ph.Dates[len(ph.Dates)-1]) {
		ph.Dates = []time.Time{}
		ph.Pies = []*Pie{}
		return ph
	}

	// Use binary search to find the index corresponding to the start and end times
	beginIdx := sort.Search(len(ph.Dates), func(i int) bool {
		idxVal := ph.Dates[i]
		return (idxVal.After(begin) || idxVal.Equal(begin))
	})

	endIdx := sort.Search(len(ph.Dates), func(i int) bool {
		idxVal := ph.Dates[i]
		return (idxVal.After(end) || idxVal.Equal(end))
	})

	if endIdx != len(ph.Dates) {
		endIdx += 1
	}
	ph.Dates = ph.Dates[beginIdx:endIdx]
	ph.Pies = ph.Pies[beginIdx:endIdx]

	return ph
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

func (ph *PieHistory) Table() {
	tableData := make([][]string, 0, len(ph.Dates))
	for idx, date := range ph.Dates {
		for security, qty := range ph.Pies[idx].Members {
			tableData = append(tableData, []string{date.Format("2006-01-02"), security.Ticker, fmt.Sprintf("%.2f", qty)})
		}
	}

	data := [][]string{
		[]string{"1/1/2014", "Domain name", "2233", "$10.98"},
		[]string{"1/1/2014", "January Hosting", "2233", "$54.95"},
		[]string{"1/4/2014", "February Hosting", "2233", "$51.00"},
		[]string{"1/4/2014", "February Extra Bandwidth", "2233", "$30.00"},
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Date", "Description", "CV2", "Amount"})
	table.SetFooter([]string{"", "", "Total", "$146.93"}) // Add Footer
	table.SetBorder(false)                                // Set Border to false
	table.AppendBulk(data)                                // Add Bulk Data
	table.Render()
}
