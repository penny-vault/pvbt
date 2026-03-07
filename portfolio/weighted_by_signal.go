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

import "github.com/penny-vault/pvbt/data"

// WeightedBySignal builds a PortfolioPlan from a DataFrame by weighting
// each selected asset proportionally to the values in the named metric
// column. Weights are normalized so they sum to 1.0 at each timestep.
// The DataFrame should have been filtered by a Selector first.
//
// For example, weighting by market cap:
//
//	plan := WeightedBySignal(selected, data.MarketCap)
func WeightedBySignal(df *data.DataFrame, metric data.Metric) PortfolioPlan { return nil }
