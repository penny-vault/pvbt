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

type twrr struct{}

func (twrr) Name() string                                         { return "TWRR" }
func (twrr) Compute(a *Account, window *Period) float64        { return 0 }
func (twrr) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// TWRR is the time-weighted rate of return, which eliminates the effect
// of cash flows (deposits/withdrawals) on portfolio returns.
var TWRR = twrr{}
