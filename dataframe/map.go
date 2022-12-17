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

package dataframe

import (
	"time"

	"github.com/penny-vault/pv-api/common"
	"github.com/rs/zerolog/log"
)

type DataFrameMap map[string]*DataFrame

// Align finds the maximum start and minimum end across all dataframes and trims them to match
func (dfMap DataFrameMap) Align() DataFrameMap {
	// find max start and min end
	var start time.Time
	var end time.Time

	// initialize end time with a value from dfMap
	for _, df := range dfMap {
		end = df.End()
		break
	}

	for _, df := range dfMap {
		start = common.MaxTime(start, df.Start())
		end = common.MinTime(end, df.End())
	}

	// trim df's to expected time range
	dfMapTrimmed := make(DataFrameMap, len(dfMap))
	for k, df := range dfMap {
		dfMapTrimmed[k] = df.Trim(start, end)
	}

	return dfMapTrimmed
}

// Drop calls dataframe.Drop on each dataframe in the map
func (dfMap DataFrameMap) Drop(val float64) DataFrameMap {
	for _, v := range dfMap {
		v.Drop(val)
	}
	return dfMap
}

func (dfMap DataFrameMap) Frequency(frequency Frequency) DataFrameMap {
	newDfMap := make(DataFrameMap, len(dfMap))
	for k, v := range dfMap {
		newDfMap[k] = v.Frequency(frequency)
	}
	return newDfMap

}

// DataFrame converts each item in the map to a column in the dataframe. If dataframes do not align they are trimmed to the max start and
// min end
func (dfMap DataFrameMap) DataFrame() *DataFrame {
	df := &DataFrame{}
	first := true
	dfMap2 := dfMap.Align()
	for _, v := range dfMap2 {
		if first {
			df.Dates = v.Dates
			df.ColNames = v.ColNames
			df.Vals = v.Vals
			first = false
		} else {
			if len(df.Dates) != len(v.Dates) ||
				!df.Start().Equal(v.Start()) ||
				!df.End().Equal(v.End()) {
				log.Panic().Time("df1.Start", df.Start()).Time("df1.End", df.End()).Time("df2.Start", v.Start()).Time("df2.End", v.End()).Msg("date indexes do not match - cannot merge into single dataframe")
			}
			df.ColNames = append(df.ColNames, v.ColNames...)
			df.Vals = append(df.Vals, v.Vals...)
		}
	}

	return df
}
