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
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/study/report"
)

// Runner holds study configuration and executes the study.
type Runner struct {
	Study          Study
	NewStrategy    func() engine.Strategy
	Options        []engine.Option
	Workers        int
	Sweeps         []ParamSweep
	SearchStrategy SearchStrategy
	Splits         []Split
	Objective      Metric
}

// Run executes the study and returns channels for progress and the final result.
// If Configurations() fails, Run returns nil channels and the error synchronously.
func (runner *Runner) Run(ctx context.Context) (<-chan Progress, <-chan Result, error) {
	if runner.SearchStrategy != nil && len(runner.Sweeps) > 0 {
		return nil, nil, fmt.Errorf("runner: SearchStrategy and Sweeps are mutually exclusive")
	}

	configs, err := runner.Study.Configurations(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("generating configurations: %w", err)
	}

	workers := runner.Workers
	if workers <= 0 {
		workers = 1
	}

	if runner.SearchStrategy != nil {
		return runner.runSearch(ctx, configs, workers)
	}

	// Existing sweep path.
	configs = CrossProduct(configs, runner.Sweeps)

	progressCh := make(chan Progress, len(configs)*2)
	resultCh := make(chan Result, 1)

	go runner.execute(ctx, configs, workers, progressCh, resultCh)

	return progressCh, resultCh, nil
}

func (runner *Runner) execute(ctx context.Context, configs []RunConfig, workers int, progressCh chan<- Progress, resultCh chan<- Result) {
	defer close(progressCh)
	defer close(resultCh)

	results := runner.runBatch(ctx, configs, workers, 0, 0, progressCh)

	analysisReport, analyzeErr := runner.Study.Analyze(results)
	resultCh <- Result{Runs: results, Report: analysisReport, Err: analyzeErr}
}

// runBatch executes a slice of RunConfigs concurrently and returns the results.
// batchIdx and batchTotal are forwarded into Progress messages.
func (runner *Runner) runBatch(ctx context.Context, configs []RunConfig, workers, batchIdx, batchTotal int, progressCh chan<- Progress) []RunResult {
	results := make([]RunResult, len(configs))

	sem := make(chan struct{}, workers)

	var waitGroup sync.WaitGroup

	for idx, cfg := range configs {
		select {
		case <-ctx.Done():
			results[idx] = RunResult{Config: cfg, Err: ctx.Err()}
			progressCh <- Progress{
				RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs),
				BatchIndex: batchIdx, BatchSize: batchTotal,
				Status: RunFailed, Err: ctx.Err(),
			}

			continue
		case sem <- struct{}{}:
		}

		waitGroup.Add(1)

		go func(runIdx int, runCfg RunConfig) {
			defer waitGroup.Done()
			defer func() { <-sem }()

			progressCh <- Progress{
				RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs),
				BatchIndex: batchIdx, BatchSize: batchTotal,
				Status: RunStarted,
			}

			runResult := runner.runSingle(ctx, runCfg)
			results[runIdx] = runResult

			if runResult.Err != nil {
				progressCh <- Progress{
					RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs),
					BatchIndex: batchIdx, BatchSize: batchTotal,
					Status: RunFailed, Err: runResult.Err,
				}
			} else {
				progressCh <- Progress{
					RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs),
					BatchIndex: batchIdx, BatchSize: batchTotal,
					Status: RunCompleted,
				}
			}
		}(idx, cfg)
	}

	waitGroup.Wait()

	return results
}

// configOrigin tracks the provenance of each config in a batch.
type configOrigin struct {
	combinationID int
	splitIndex    int
	combo         RunConfig
}

// runSearch implements the iterative search path: call SearchStrategy.Next()
// in a loop, expanding each batch with Splits, executing, scoring, and
// feeding scores back until the strategy signals done.
func (runner *Runner) runSearch(ctx context.Context, baseConfigs []RunConfig, workers int) (<-chan Progress, <-chan Result, error) {
	progressCh := make(chan Progress, 64)
	resultCh := make(chan Result, 1)

	go func() {
		defer close(progressCh)
		defer close(resultCh)

		var allResults []RunResult

		var scores []CombinationScore

		combinationID := 0
		batchIdx := 0

		for {
			combos, done := runner.SearchStrategy.Next(scores)

			if len(combos) == 0 && done {
				break
			}

			// Expand combos with base configs and splits.
			var batchConfigs []RunConfig

			// Track which combination ID each config belongs to, and which split.
			var origins []configOrigin

			for _, combo := range combos {
				comboID := combinationID
				combinationID++

				if len(runner.Splits) == 0 {
					// No splits: cross combo with each base config.
					for _, base := range baseConfigs {
						cfg := mergeComboWithBase(combo, base)
						if cfg.Metadata == nil {
							cfg.Metadata = make(map[string]string)
						}

						cfg.Metadata["_combination_id"] = strconv.Itoa(comboID)

						batchConfigs = append(batchConfigs, cfg)
						origins = append(origins, configOrigin{
							combinationID: comboID,
							splitIndex:    -1,
							combo:         combo,
						})
					}
				} else {
					// With splits: for each split, create a config with the split's FullRange.
					for splitIdx, sp := range runner.Splits {
						for _, base := range baseConfigs {
							cfg := mergeComboWithBase(combo, base)
							cfg.Start = sp.FullRange.Start
							cfg.End = sp.FullRange.End

							if cfg.Metadata == nil {
								cfg.Metadata = make(map[string]string)
							}

							cfg.Metadata["_combination_id"] = strconv.Itoa(comboID)
							cfg.Metadata["_split_index"] = strconv.Itoa(splitIdx)
							cfg.Metadata["_split_name"] = sp.Name

							batchConfigs = append(batchConfigs, cfg)
							origins = append(origins, configOrigin{
								combinationID: comboID,
								splitIndex:    splitIdx,
								combo:         combo,
							})
						}
					}
				}
			}

			batchResults := runner.runBatch(ctx, batchConfigs, workers, batchIdx, len(combos), progressCh)
			allResults = append(allResults, batchResults...)

			// Score each combination by grouping results by _combination_id.
			scores = runner.scoreBatch(combos, batchResults, origins, combinationID-len(combos))

			batchIdx++

			if done {
				break
			}
		}

		analysisReport, analyzeErr := runner.Study.Analyze(allResults)
		resultCh <- Result{Runs: allResults, Report: analysisReport, Err: analyzeErr}
	}()

	return progressCh, resultCh, nil
}

// scoreBatch groups batch results by combination ID and computes the mean
// score across splits for each combination.
func (runner *Runner) scoreBatch(combos []RunConfig, batchResults []RunResult, origins []configOrigin, firstComboID int) []CombinationScore {
	// Group results by combination ID.
	type comboGroup struct {
		combo   RunConfig
		results []RunResult
		origins []configOrigin
	}

	groups := make(map[int]*comboGroup)

	for idx, origin := range origins {
		grp, exists := groups[origin.combinationID]
		if !exists {
			grp = &comboGroup{combo: origin.combo}
			groups[origin.combinationID] = grp
		}

		grp.results = append(grp.results, batchResults[idx])
		grp.origins = append(grp.origins, origin)
	}

	scores := make([]CombinationScore, 0, len(combos))

	for comboIdx := range combos {
		comboID := firstComboID + comboIdx
		grp, exists := groups[comboID]

		if !exists {
			scores = append(scores, CombinationScore{
				Params: combos[comboIdx].Params,
				Preset: combos[comboIdx].Preset,
				Score:  math.NaN(),
				Runs:   nil,
			})

			continue
		}

		var totalScore float64

		var validCount int

		for resultIdx, runResult := range grp.results {
			if runResult.Err != nil || runResult.Portfolio == nil {
				continue
			}

			origin := grp.origins[resultIdx]

			var score float64

			if origin.splitIndex >= 0 && origin.splitIndex < len(runner.Splits) {
				sp := runner.Splits[origin.splitIndex]
				score = WindowedScore(runResult.Portfolio, sp.Test, runner.Objective)
			} else {
				// No splits; score the entire run.
				score = WindowedScore(runResult.Portfolio, DateRange{
					Start: runResult.Config.Start,
					End:   runResult.Config.End,
				}, runner.Objective)
			}

			if !math.IsNaN(score) {
				totalScore += score
				validCount++
			}
		}

		meanScore := math.NaN()
		if validCount > 0 {
			meanScore = totalScore / float64(validCount)
		}

		scores = append(scores, CombinationScore{
			Params: grp.combo.Params,
			Preset: grp.combo.Preset,
			Score:  meanScore,
			Runs:   grp.results,
		})
	}

	return scores
}

// mergeComboWithBase merges a combo RunConfig (params/preset from SearchStrategy)
// onto a base RunConfig (dates, deposit, metadata from Configurations).
func mergeComboWithBase(combo, base RunConfig) RunConfig {
	cfg := cloneRunConfig(base)

	if combo.Preset != "" {
		cfg.Preset = combo.Preset
	}

	if len(combo.Params) > 0 {
		if cfg.Params == nil {
			cfg.Params = make(map[string]string, len(combo.Params))
		}

		for key, val := range combo.Params {
			cfg.Params[key] = val
		}
	}

	if combo.Name != "" {
		cfg.Name = appendName(cfg.Name, combo.Name)
	}

	return cfg
}

func (runner *Runner) runSingle(ctx context.Context, cfg RunConfig) RunResult {
	strategy := runner.NewStrategy()

	// Build engine options: base options + config-specific overrides.
	opts := make([]engine.Option, len(runner.Options))
	copy(opts, runner.Options)

	// If the study implements EngineCustomizer, append per-run options.
	if customizer, ok := runner.Study.(EngineCustomizer); ok {
		opts = append(opts, customizer.EngineOptions(cfg)...)
	}

	if cfg.Deposit > 0 {
		opts = append(opts, engine.WithInitialDeposit(cfg.Deposit))
	}

	eng := engine.New(strategy, opts...)

	// Apply preset and parameter overrides after engine construction
	// so that asset and universe fields can be resolved.
	if cfg.Preset != "" || len(cfg.Params) > 0 {
		if err := engine.ApplyParams(eng, cfg.Preset, cfg.Params); err != nil {
			return RunResult{Config: cfg, Err: fmt.Errorf("applying params: %w", err)}
		}
	}

	portfolioResult, err := eng.Backtest(ctx, cfg.Start, cfg.End)
	if err != nil {
		return RunResult{Config: cfg, Err: err}
	}

	// Store metadata on portfolio.
	for key, val := range cfg.Metadata {
		portfolioResult.SetMetadata(key, val)
	}

	// Type-assert to ReportablePortfolio.
	reportable, ok := portfolioResult.(report.ReportablePortfolio)
	if !ok {
		return RunResult{Config: cfg, Err: fmt.Errorf("portfolio does not implement ReportablePortfolio")}
	}

	return RunResult{Config: cfg, Portfolio: reportable}
}
