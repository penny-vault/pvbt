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

import "github.com/rs/zerolog/log"

type DataFrameMap map[string]*DataFrame

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

// DataFrame converts each item in the map to a column in the dataframe. If dataframes do not align then gaps are filled with math.NaN()
func (dfMap DataFrameMap) DataFrame() *DataFrame {
	df := &DataFrame{}
	first := true
	for _, v := range dfMap {
		if first {
			df.Dates = v.Dates
			df.ColNames = v.ColNames
			df.Vals = v.Vals
			first = false
		} else {
			if len(df.Dates) != len(v.Dates) ||
				!df.Dates[0].Equal(v.Dates[0]) ||
				!df.Dates[len(df.Dates)-1].Equal(v.Dates[len(v.Dates)-1]) {
				// TODO: align date indexes
				log.Panic().Msg("date indexes do not align!")
			}
			df.ColNames = append(df.ColNames, v.ColNames...)
			df.Vals = append(df.Vals, v.Vals...)
		}
	}

	return df
}
