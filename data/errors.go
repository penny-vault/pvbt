// Copyright 2021-2025
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
	ErrSingle                 = errors.New("cannot call *Single functions with multiple securities or metrics")
	ErrBeginAfterEnd          = errors.New("invalid interval; begin after end date")
	ErrDataLargerThanCache    = errors.New("data larger than cache size")
	ErrDateLengthDoesNotMatch = errors.New("length of date and values arrays must match when using local date index")
	ErrInvalidTimeRange       = errors.New("start must be before end")
	ErrMultipleNotSupported   = errors.New("on-or-before can only return a single metric for a single security")
	ErrNoData                 = errors.New("no data available")
	ErrSecurityNotFound       = errors.New("security not found")
	ErrNoTradingDays          = errors.New("no trading days available")
	ErrOutsideCoveredTime     = errors.New("date range outside of covered time interval")
	ErrRangeDoesNotExist      = errors.New("range does not exist in cache")
	ErrUnsupportedMetric      = errors.New("unsupported metric")
)
