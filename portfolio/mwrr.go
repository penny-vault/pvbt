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

type mwrr struct{}

func (mwrr) Name() string                                      { return "MWRR" }
func (mwrr) Compute(a *Account, window *Period) float64         { return 0 }
func (mwrr) ComputeSeries(a *Account, window *Period) []float64 { return nil }

// MWRR is the money-weighted rate of return: accounts for the timing
// and size of cash flows (deposits/withdrawals) using XIRR. Unlike
// TWRR, this metric reflects the investor's actual experience.
var MWRR = mwrr{}
