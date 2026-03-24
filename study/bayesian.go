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
	"math"
	"math/rand/v2"
	"strconv"
)

const (
	defaultBatchSize      = 10
	defaultMaxIterations  = 20
	defaultCandidateCount = 1000
	defaultGPVariance     = 1.0
	defaultGPLength       = 0.3
	defaultGPJitter       = 1e-6
)

// BayesianOption configures the Bayesian search strategy.
type BayesianOption func(*bayesianStrategy)

// WithBatchSize sets the number of candidates returned per guided iteration.
func WithBatchSize(size int) BayesianOption {
	return func(bs *bayesianStrategy) {
		bs.batchSize = size
	}
}

// WithMaxIterations sets the maximum number of guided (non-initial) iterations
// before the strategy signals completion.
func WithMaxIterations(max int) BayesianOption {
	return func(bs *bayesianStrategy) {
		bs.maxIterations = max
	}
}

// WithInitialSamples sets the number of random samples drawn on the first call to Next.
func WithInitialSamples(count int) BayesianOption {
	return func(bs *bayesianStrategy) {
		bs.initialSamples = count
	}
}

// bayesianStrategy implements SearchStrategy using Gaussian process surrogate
// modeling with Expected Improvement acquisition for parameter optimization.
type bayesianStrategy struct {
	sweeps         []ParamSweep
	batchSize      int
	maxIterations  int
	initialSamples int
	guidedCount    int
	rng            *rand.Rand
}

// NewBayesian creates a Bayesian optimization search strategy. It uses a Gaussian
// process surrogate model and Expected Improvement acquisition to guide the search.
func NewBayesian(sweeps []ParamSweep, seed int64, opts ...BayesianOption) SearchStrategy {
	src := rand.NewPCG(uint64(seed), 0)
	rng := rand.New(src)

	bs := &bayesianStrategy{
		sweeps:        sweeps,
		batchSize:     defaultBatchSize,
		maxIterations: defaultMaxIterations,
		rng:           rng,
	}

	for _, opt := range opts {
		opt(bs)
	}

	// Default initialSamples to batchSize if not explicitly set.
	if bs.initialSamples == 0 {
		bs.initialSamples = bs.batchSize
	}

	return bs
}

// Next returns the next batch of parameter configurations to evaluate.
// On the first call (scores is nil or empty), it returns random initial samples.
// Subsequent calls fit a GP to the observed scores and use Expected Improvement
// to select candidates. Returns done=true after maxIterations guided batches.
func (bs *bayesianStrategy) Next(scores []CombinationScore) ([]RunConfig, bool) {
	if len(scores) == 0 {
		return bs.generateInitialSamples(), false
	}

	if bs.guidedCount >= bs.maxIterations {
		return nil, true
	}

	bs.guidedCount++

	configs, err := bs.generateGuidedSamples(scores)
	if err != nil {
		// Fall back to random sampling if GP fitting fails.
		return bs.generateRandomBatch(bs.batchSize), false
	}

	return configs, bs.guidedCount >= bs.maxIterations
}

// generateInitialSamples creates random parameter configurations using the same
// logic as the Random search strategy.
func (bs *bayesianStrategy) generateInitialSamples() []RunConfig {
	return bs.generateRandomBatch(bs.initialSamples)
}

// generateRandomBatch creates count random parameter configurations.
func (bs *bayesianStrategy) generateRandomBatch(count int) []RunConfig {
	configs := make([]RunConfig, 0, count)

	for range count {
		cfg := RunConfig{}

		for _, sweep := range bs.sweeps {
			if sweep.IsPreset() {
				vals := sweep.Values()
				cfg.Preset = vals[bs.rng.IntN(len(vals))]

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

				sampled := minVal + bs.rng.Float64()*(maxVal-minVal)

				if cfg.Params == nil {
					cfg.Params = make(map[string]string)
				}

				cfg.Params[sweep.Field()] = fmt.Sprintf("%g", sampled)
			} else {
				vals := sweep.Values()
				chosen := vals[bs.rng.IntN(len(vals))]

				if cfg.Params == nil {
					cfg.Params = make(map[string]string)
				}

				cfg.Params[sweep.Field()] = chosen
			}
		}

		configs = append(configs, cfg)
	}

	return configs
}

// generateGuidedSamples fits a GP to observed scores and selects candidates
// that maximize Expected Improvement.
func (bs *bayesianStrategy) generateGuidedSamples(scores []CombinationScore) ([]RunConfig, error) {
	dims := len(bs.sweeps)

	// Encode observed points into normalized [0,1] space.
	xTrain := make([][]float64, len(scores))
	yTrain := make([]float64, len(scores))
	bestScore := math.Inf(-1)

	for ii, sc := range scores {
		xTrain[ii] = bs.encode(sc)
		yTrain[ii] = sc.Score

		if sc.Score > bestScore {
			bestScore = sc.Score
		}
	}

	// Fit GP surrogate model.
	gp := newGaussianProcess(defaultGPVariance, defaultGPLength, defaultGPJitter)

	if err := gp.Fit(xTrain, yTrain); err != nil {
		return nil, fmt.Errorf("bayesian guided samples: %w", err)
	}

	// Generate random candidate points and evaluate EI on each.
	candidates := make([][]float64, defaultCandidateCount)
	eiValues := make([]float64, defaultCandidateCount)

	for ii := range defaultCandidateCount {
		candidate := make([]float64, dims)
		for dd := range dims {
			candidate[dd] = bs.rng.Float64()
		}

		candidates[ii] = candidate

		mu, sigma := gp.Predict(candidate)
		eiValues[ii] = expectedImprovement(mu, sigma, bestScore)
	}

	// Select top batchSize candidates by EI value.
	selected := bs.topKIndices(eiValues, bs.batchSize)

	// Decode selected candidates back to RunConfigs.
	configs := make([]RunConfig, 0, len(selected))
	for _, idx := range selected {
		cfg := bs.decode(candidates[idx])
		configs = append(configs, cfg)
	}

	return configs, nil
}

// encode maps a CombinationScore to a normalized [0,1]^D point.
func (bs *bayesianStrategy) encode(sc CombinationScore) []float64 {
	encoded := make([]float64, len(bs.sweeps))

	for ii, sweep := range bs.sweeps {
		if sweep.IsPreset() {
			encoded[ii] = bs.encodeCategorical(sc.Preset, sweep.Values())
			continue
		}

		if sweep.Min() != "" && sweep.Max() != "" {
			encoded[ii] = bs.encodeNumeric(sc.Params[sweep.Field()], sweep)
		} else {
			encoded[ii] = bs.encodeCategorical(sc.Params[sweep.Field()], sweep.Values())
		}
	}

	return encoded
}

// encodeNumeric maps a numeric parameter value to [0, 1] using linear scaling.
func (bs *bayesianStrategy) encodeNumeric(val string, sweep ParamSweep) float64 {
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0.5
	}

	minVal, err := strconv.ParseFloat(sweep.Min(), 64)
	if err != nil {
		return 0.5
	}

	maxVal, err := strconv.ParseFloat(sweep.Max(), 64)
	if err != nil {
		return 0.5
	}

	if maxVal == minVal {
		return 0.5
	}

	normalized := (parsed - minVal) / (maxVal - minVal)

	return math.Max(0, math.Min(1, normalized))
}

// encodeCategorical maps a categorical value to [0, 1] based on its index.
func (bs *bayesianStrategy) encodeCategorical(val string, values []string) float64 {
	for ii, vv := range values {
		if vv == val {
			if len(values) == 1 {
				return 0.5
			}

			return float64(ii) / float64(len(values)-1)
		}
	}

	return 0.5
}

// decode maps a normalized [0,1]^D point back to a RunConfig.
func (bs *bayesianStrategy) decode(point []float64) RunConfig {
	cfg := RunConfig{}

	for ii, sweep := range bs.sweeps {
		if sweep.IsPreset() {
			cfg.Preset = bs.decodeCategorical(point[ii], sweep.Values())
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

			decoded := minVal + point[ii]*(maxVal-minVal)

			if cfg.Params == nil {
				cfg.Params = make(map[string]string)
			}

			cfg.Params[sweep.Field()] = fmt.Sprintf("%g", decoded)
		} else {
			if cfg.Params == nil {
				cfg.Params = make(map[string]string)
			}

			cfg.Params[sweep.Field()] = bs.decodeCategorical(point[ii], sweep.Values())
		}
	}

	return cfg
}

// decodeCategorical maps a [0, 1] value to the nearest categorical value.
func (bs *bayesianStrategy) decodeCategorical(normalized float64, values []string) string {
	if len(values) == 0 {
		return ""
	}

	if len(values) == 1 {
		return values[0]
	}

	idx := int(math.Round(normalized * float64(len(values)-1)))
	if idx < 0 {
		idx = 0
	}

	if idx >= len(values) {
		idx = len(values) - 1
	}

	return values[idx]
}

// topKIndices returns the indices of the k largest values in the slice.
func (bs *bayesianStrategy) topKIndices(values []float64, kk int) []int {
	if kk >= len(values) {
		indices := make([]int, len(values))
		for ii := range values {
			indices[ii] = ii
		}

		return indices
	}

	// Simple selection: track the top-k indices and their minimum.
	topIdx := make([]int, kk)
	topVal := make([]float64, kk)

	for ii := range kk {
		topIdx[ii] = ii
		topVal[ii] = values[ii]
	}

	// Find minimum in current top-k.
	minPos := 0

	for ii := 1; ii < kk; ii++ {
		if topVal[ii] < topVal[minPos] {
			minPos = ii
		}
	}

	for ii := kk; ii < len(values); ii++ {
		if values[ii] > topVal[minPos] {
			topIdx[minPos] = ii
			topVal[minPos] = values[ii]

			// Re-find minimum.
			minPos = 0
			for jj := 1; jj < kk; jj++ {
				if topVal[jj] < topVal[minPos] {
					minPos = jj
				}
			}
		}
	}

	return topIdx
}
