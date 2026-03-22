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

package montecarlo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
)

// Compile-time interface checks.
var (
	_ study.Study            = (*MonteCarloStudy)(nil)
	_ study.EngineCustomizer = (*MonteCarloStudy)(nil)
)

// MonteCarloStudy implements study.Study and study.EngineCustomizer to run
// Monte Carlo simulations over resampled historical data.
type MonteCarloStudy struct {
	// Configuration
	Simulations   int
	Resampler     data.Resampler
	Seed          uint64
	RuinThreshold float64 // negative, e.g. -0.30

	// Data
	historicalData *data.DataFrame
	metrics        []data.Metric

	// Optional historical result for percentile ranking.
	HistoricalResult report.ReportablePortfolio

	// RunConfig template
	StartDate      time.Time
	EndDate        time.Time
	InitialDeposit float64
}

// New returns a MonteCarloStudy backed by the given historical DataFrame,
// with sensible defaults: 1000 simulations, block bootstrap with block size
// 20, seed 42, and a ruin threshold of -30%.
func New(historicalData *data.DataFrame, metrics []data.Metric) *MonteCarloStudy {
	return &MonteCarloStudy{
		Simulations:    1000,
		Resampler:      &data.BlockBootstrap{BlockSize: 20},
		Seed:           42,
		RuinThreshold:  -0.30,
		historicalData: historicalData,
		metrics:        metrics,
	}
}

// Name returns the human-readable study name.
func (mcs *MonteCarloStudy) Name() string { return "Monte Carlo Simulation" }

// Description returns a short explanation of what the study does.
func (mcs *MonteCarloStudy) Description() string {
	return "Assess strategy robustness via resampled historical data"
}

// Configurations returns one RunConfig per simulation path. Each config
// carries a unique seed in its Metadata so that EngineOptions can construct
// a deterministic ResamplingProvider for that path.
func (mcs *MonteCarloStudy) Configurations(_ context.Context) ([]study.RunConfig, error) {
	configs := make([]study.RunConfig, mcs.Simulations)

	for pathIdx := range mcs.Simulations {
		seed := mcs.Seed + uint64(pathIdx)

		configs[pathIdx] = study.RunConfig{
			Name:    fmt.Sprintf("Path %d", pathIdx+1),
			Start:   mcs.StartDate,
			End:     mcs.EndDate,
			Deposit: mcs.InitialDeposit,
			Metadata: map[string]string{
				"study":           "monte-carlo",
				"simulation_seed": strconv.FormatUint(seed, 10),
			},
		}
	}

	return configs, nil
}

// EngineOptions returns engine options that inject a ResamplingProvider for
// the given RunConfig. The seed is read from cfg.Metadata["simulation_seed"].
// If the metadata key is absent or unparseable, nil is returned.
func (mcs *MonteCarloStudy) EngineOptions(cfg study.RunConfig) []engine.Option {
	seedStr, hasSeed := cfg.Metadata["simulation_seed"]
	if !hasSeed {
		return nil
	}

	seed, parseErr := strconv.ParseUint(seedStr, 10, 64)
	if parseErr != nil {
		return nil
	}

	provider := data.NewResamplingProvider(mcs.historicalData, mcs.Resampler, seed, mcs.metrics)

	return []engine.Option{engine.WithDataProvider(provider)}
}

// Analyze delegates to analyzeResults to compute percentile distributions
// and build the Monte Carlo report.
func (mcs *MonteCarloStudy) Analyze(results []study.RunResult) (report.Report, error) {
	return analyzeResults(results, mcs.HistoricalResult, mcs.RuinThreshold)
}
