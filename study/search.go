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

package study

import (
	"fmt"
	"math/rand/v2"
	"strconv"
)

// CombinationScore records the outcome of evaluating one parameter combination.
type CombinationScore struct {
	Params map[string]string
	Preset string
	Score  float64
	Runs   []RunResult
}

// SearchStrategy generates parameter combinations to evaluate.
// Next is called with all previously scored combinations and returns the next
// batch of RunConfigs to execute. done=true signals that no further calls are needed.
type SearchStrategy interface {
	Next(scores []CombinationScore) (configs []RunConfig, done bool)
}

// gridSearch enumerates the full cross-product of parameter sweeps.
type gridSearch struct {
	sweeps []ParamSweep
	called bool
}

// NewGrid returns a SearchStrategy that exhaustively enumerates all combinations
// of the given parameter sweeps. The first call to Next returns all configurations
// and done=true; subsequent calls return nothing.
func NewGrid(sweeps ...ParamSweep) SearchStrategy {
	return &gridSearch{sweeps: sweeps}
}

// Next returns the full cross-product of configurations on the first call.
func (gs *gridSearch) Next(scores []CombinationScore) ([]RunConfig, bool) {
	if gs.called {
		return nil, true
	}

	gs.called = true

	base := []RunConfig{{}}
	configs := CrossProduct(base, gs.sweeps)

	return configs, true
}

// randomSearch samples parameter combinations at random.
type randomSearch struct {
	sweeps  []ParamSweep
	samples int
	rng     *rand.Rand
	called  bool
}

// NewRandom returns a SearchStrategy that samples the given number of random
// parameter combinations. For sweeps with a non-empty Min()/Max(), values are
// drawn uniformly from [min, max] as float64. For sweeps without range bounds,
// values are drawn from the Values() list. seed controls reproducibility.
func NewRandom(sweeps []ParamSweep, samples int, seed int64) SearchStrategy {
	src := rand.NewPCG(uint64(seed), 0)
	rng := rand.New(src)

	return &randomSearch{
		sweeps:  sweeps,
		samples: samples,
		rng:     rng,
	}
}

// Next returns all sampled configurations on the first call.
func (rs *randomSearch) Next(scores []CombinationScore) ([]RunConfig, bool) {
	if rs.called {
		return nil, true
	}

	rs.called = true

	configs := make([]RunConfig, 0, rs.samples)

	for range rs.samples {
		cfg := RunConfig{}

		for _, sweep := range rs.sweeps {
			if sweep.IsPreset() {
				vals := sweep.Values()
				cfg.Preset = vals[rs.rng.IntN(len(vals))]

				continue
			}

			if sweep.Min() != "" && sweep.Max() != "" {
				minVal, err := strconv.ParseFloat(sweep.Min(), 64)
				if err != nil {
					minVal = 0
				}

				maxVal, err := strconv.ParseFloat(sweep.Max(), 64)
				if err != nil {
					maxVal = minVal
				}

				sampled := minVal + rs.rng.Float64()*(maxVal-minVal)

				if cfg.Params == nil {
					cfg.Params = make(map[string]string)
				}

				cfg.Params[sweep.Field()] = fmt.Sprintf("%g", sampled)
			} else {
				vals := sweep.Values()
				chosen := vals[rs.rng.IntN(len(vals))]

				if cfg.Params == nil {
					cfg.Params = make(map[string]string)
				}

				cfg.Params[sweep.Field()] = chosen
			}
		}

		configs = append(configs, cfg)
	}

	return configs, true
}
