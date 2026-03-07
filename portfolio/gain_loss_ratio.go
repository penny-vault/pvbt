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

type gainLossRatio struct{}

func (gainLossRatio) Name() string                                      { return "GainLossRatio" }
func (gainLossRatio) Compute(a *Account, window *Period) float64         { return 0 }
func (gainLossRatio) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// GainLossRatio is the average gain on winning periods divided by the
// average loss on losing periods. A ratio above 1.0 means wins are
// larger than losses on average. Combined with NPositivePeriods, this
// gives a complete picture of the win/loss profile.
var GainLossRatio = gainLossRatio{}
