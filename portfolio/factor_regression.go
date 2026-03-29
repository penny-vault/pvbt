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

import (
	"errors"
)

var (
	ErrTooFewObservations = errors.New("fewer than 12 overlapping observations")
	ErrNoFactors          = errors.New("factor DataFrame contains no metrics")
)

// FactorRegression holds the result of an OLS regression of portfolio excess
// returns against one or more factor return series.
type FactorRegression struct {
	Alpha    float64            // intercept: return not explained by factors
	RSquared float64            // fraction of variance explained by the model
	AIC      float64            // Akaike Information Criterion
	Betas    map[string]float64 // factor metric name -> coefficient
}

// StepwiseResult holds the output of forward stepwise factor selection.
type StepwiseResult struct {
	Best  FactorRegression   // the final selected model
	Steps []FactorRegression // one per round (each adds one factor)
}
