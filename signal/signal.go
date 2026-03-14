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

package signal

import "github.com/penny-vault/pvbt/data"

// Signal output names. These are typed as data.Metric so they can be
// used in DataFrame operations, but they represent computed signals,
// not raw market data. The Signal suffix avoids collision with the
// function names in this package.
const (
	MomentumSignal      data.Metric = "Momentum"
	VolatilitySignal    data.Metric = "Volatility"
	EarningsYieldSignal data.Metric = "EarningsYield"
)
