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

package portfolio

// WithInitialMargin returns an Option that sets the initial margin rate
// required when opening a short position. The default is 0.50 (50%).
func WithInitialMargin(rate float64) Option {
	return func(a *Account) {
		a.initialMargin = rate
	}
}

// WithMaintenanceMargin returns an Option that sets the maintenance
// margin rate. If equity falls below this fraction of short market
// value, a margin call is triggered. The default is 0.30 (30%).
func WithMaintenanceMargin(rate float64) Option {
	return func(a *Account) {
		a.maintenanceMargin = rate
	}
}

// WithBorrowRate returns an Option that sets the annualized borrow rate
// charged on short positions.
func WithBorrowRate(rate float64) Option {
	return func(a *Account) {
		a.borrowRate = rate
	}
}
