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

package data

import "time"

// Intervals store a beginning and ending time period
type Interval struct {
	Begin time.Time
	End   time.Time
}

// Adjacent checks if offset touches the begining or ending of interval (daily resolution)
// NOTE: Adjaceny implies the two intervals DO NOT overlap
func (interval *Interval) Adjacent(other *Interval) bool {
	return interval.AdjacentLeft(other) && interval.AdjacentRight(other)
}

// Adjacent checks if offset touches the begining of interval (daily resolution)
// NOTE: Adjaceny implies the two intervals DO NOT overlap
func (interval *Interval) AdjacentLeft(other *Interval) bool {
	// if other is prior to interval - check if they touch
	otherEnd := other.End.AddDate(0, 0, 1)
	if otherEnd.Equal(interval.Begin) {
		return true
	}

	return false
}

// Adjacent checks if offset touches the ending of interval (daily resolution)
// NOTE: Adjaceny implies the two intervals DO NOT overlap
func (interval *Interval) AdjacentRight(other *Interval) bool {
	// if other is after interval - check if they touch
	otherBegin := other.Begin.AddDate(0, 0, -1)
	if otherBegin.Equal(interval.End) {
		return true
	}
	return false
}

// Contains returns true if interval completely contains other
func (interval *Interval) Contains(other *Interval) bool {
	if (other.Begin.Before(interval.End) || other.Begin.Equal(interval.End)) && (other.End.After(interval.Begin) || other.End.Equal(interval.Begin)) {
		return true
	}
	return false
}

// Contiguous returns true if the other interval is adjacent to or overlaps with this interval
// NOTE: contiguous implies that other is not a subset of interval
func (interval *Interval) Contiguous(other *Interval) bool {
	if interval.Adjacent(other) {
		return true
	}

	// if the two intervals overlap then they are contiguous
	if interval.Contains(other) {
		return false
	}

	if interval.Overlaps(other) {
		return true
	}

	return false
}

// Overlaps returns true if interval and other overlap
func (interval *Interval) Overlaps(other *Interval) bool {
	if other.Begin.Before(interval.End) && other.End.After(interval.Begin) {
		return true
	}
	return false
}

// Valid checks if the given interval is valid range and returns an error if not
func (interval *Interval) Valid() error {
	if interval.Begin.After(interval.End) {
		return ErrBeginAfterEnd
	}

	return nil
}
