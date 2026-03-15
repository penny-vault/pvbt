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

// RatingFilter holds an explicit set of rating values to match against.
// The zero value matches nothing.
type RatingFilter struct {
	Values []int
}

// Matches reports whether rating is in the filter's value set.
// A zero-value RatingFilter (empty Values) always returns false.
func (f RatingFilter) Matches(rating int) bool {
	for _, v := range f.Values {
		if v == rating {
			return true
		}
	}
	return false
}

// RatingEq returns a RatingFilter that matches exactly one value.
func RatingEq(v int) RatingFilter {
	return RatingFilter{Values: []int{v}}
}

// RatingIn returns a RatingFilter that matches any of the given values.
// If no values are provided, the returned filter matches nothing.
func RatingIn(vs ...int) RatingFilter {
	return RatingFilter{Values: vs}
}

// RatingLTE returns a RatingFilter that matches ratings 1 through v inclusive.
// If v <= 0, the returned filter matches nothing (zero value).
func RatingLTE(v int) RatingFilter {
	if v <= 0 {
		return RatingFilter{}
	}
	vals := make([]int, v)
	for i := range vals {
		vals[i] = i + 1
	}
	return RatingFilter{Values: vals}
}
