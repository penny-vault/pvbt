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
	"sync"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
)

// Runner holds study configuration and executes the study.
type Runner struct {
	Study       Study
	NewStrategy func() engine.Strategy
	Options     []engine.Option
	Workers     int
	Sweeps      []ParamSweep
}

// Run executes the study and returns channels for progress and the final result.
// If Configurations() fails, Run returns nil channels and the error synchronously.
func (runner *Runner) Run(ctx context.Context) (<-chan Progress, <-chan Result, error) {
	configs, err := runner.Study.Configurations(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("generating configurations: %w", err)
	}

	// Cross-product with sweeps.
	configs = CrossProduct(configs, runner.Sweeps)

	workers := runner.Workers
	if workers <= 0 {
		workers = 1
	}

	progressCh := make(chan Progress, len(configs)*2)
	resultCh := make(chan Result, 1)

	go runner.execute(ctx, configs, workers, progressCh, resultCh)

	return progressCh, resultCh, nil
}

func (runner *Runner) execute(ctx context.Context, configs []RunConfig, workers int, progressCh chan<- Progress, resultCh chan<- Result) {
	defer close(progressCh)
	defer close(resultCh)

	results := make([]RunResult, len(configs))

	sem := make(chan struct{}, workers)

	var waitGroup sync.WaitGroup

	for idx, cfg := range configs {
		select {
		case <-ctx.Done():
			results[idx] = RunResult{Config: cfg, Err: ctx.Err()}
			progressCh <- Progress{RunName: cfg.Name, RunIndex: idx, TotalRuns: len(configs), Status: RunFailed, Err: ctx.Err()}

			continue
		case sem <- struct{}{}:
		}

		waitGroup.Add(1)

		go func(runIdx int, runCfg RunConfig) {
			defer waitGroup.Done()
			defer func() { <-sem }()

			progressCh <- Progress{RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs), Status: RunStarted}

			runResult := runner.runSingle(ctx, runCfg)
			results[runIdx] = runResult

			if runResult.Err != nil {
				progressCh <- Progress{RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs), Status: RunFailed, Err: runResult.Err}
			} else {
				progressCh <- Progress{RunName: runCfg.Name, RunIndex: runIdx, TotalRuns: len(configs), Status: RunCompleted}
			}
		}(idx, cfg)
	}

	waitGroup.Wait()

	analysisReport, analyzeErr := runner.Study.Analyze(results)
	resultCh <- Result{Runs: results, Report: analysisReport, Err: analyzeErr}
}

func (runner *Runner) runSingle(ctx context.Context, cfg RunConfig) RunResult {
	strategy := runner.NewStrategy()

	// Build engine options: base options + config-specific overrides.
	opts := make([]engine.Option, len(runner.Options))
	copy(opts, runner.Options)

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
