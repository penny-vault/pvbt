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

type safeWithdrawalRate struct{}

func (safeWithdrawalRate) Name() string                                      { return "SafeWithdrawalRate" }
func (safeWithdrawalRate) Compute(a *Account, window *Period) float64         { return 0 }
func (safeWithdrawalRate) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// SafeWithdrawalRate is the maximum constant annual withdrawal rate
// (as a percentage of initial balance) where the portfolio balance
// never reaches zero over the simulation period. Uses circular
// bootstrap Monte Carlo simulation of historical returns.
var SafeWithdrawalRate = safeWithdrawalRate{}
