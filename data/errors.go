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

import "errors"

var (
	ErrNotFound            = errors.New("security not found")
	ErrInvalidTimeRange    = errors.New("start must be before end")
	ErrNoTradingDays       = errors.New("no trading days available")
	ErrRangeDoesNotExist   = errors.New("range does not exist in cache")
	ErrUnsupportedMetric   = errors.New("unsupported metric")
	ErrDataLargerThanCache = errors.New("data larger than cache size")
	ErrOutsideCoveredTime  = errors.New("date range outside of covered time interval")
	ErrBeginAfterEnd       = errors.New("invalid interval; begin after end date")
)
