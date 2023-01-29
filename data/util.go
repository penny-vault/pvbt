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

// uniqueSecurities filters a list of Securities to only unique values
func uniqueSecurities(securities []*Security) []*Security {
	unique := make(map[string]*Security, len(securities))
	for _, v := range securities {
		unique[v.CompositeFigi] = v
	}
	uniqList := make([]*Security, len(unique))
	j := 0
	for _, v := range unique {
		uniqList[j] = v
		j++
	}
	return uniqList
}
