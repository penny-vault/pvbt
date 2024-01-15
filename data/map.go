// Copyright 2021-2023
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

/*
func (secMap SecurityMap) DataFrame() *dataframe.DataFrame[time.Time] {
	df := &dataframe.DataFrame[time.Time]{}
	first := true
	for k, v := range secMap {
		if first {
			df.Index = v.Index
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
*/
