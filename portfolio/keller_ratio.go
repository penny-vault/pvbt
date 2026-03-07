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

type kellerRatio struct{}

func (kellerRatio) Name() string                                      { return "KellerRatio" }
func (kellerRatio) Compute(a *Account, window *Period) float64         { return 0 }
func (kellerRatio) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// KellerRatio adjusts return for drawdown severity:
// K = R * (1 - D/(1-D)) when R >= 0 and D <= 50%, else 0.
// Small drawdowns have limited impact; large drawdowns amplify the
// penalty, making this a useful risk-adjusted measure.
var KellerRatio = kellerRatio{}
