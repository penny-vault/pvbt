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

type sortino struct{}

func (sortino) Name() string                                         { return "Sortino" }
func (sortino) Compute(a *Account, window *Period) float64        { return 0 }
func (sortino) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// Sortino is the Sortino ratio: like Sharpe but uses downside deviation
// instead of total standard deviation, penalizing only negative volatility.
var Sortino = sortino{}
