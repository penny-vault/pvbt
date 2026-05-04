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

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/optimize"
	"github.com/penny-vault/pvbt/study/report"
	"github.com/penny-vault/pvbt/study/stress"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newStudyCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "study",
		Short: "Run analysis studies on the strategy",
	}

	cmd.AddCommand(newStressTestCmd(strategy))
	cmd.AddCommand(newOptimizeCmd(strategy))

	return cmd
}

func newStressTestCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stress-test [scenario-names...|all]",
		Short: "Run strategy against historical market stress scenarios",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStressTest(cmd, strategy, args)
		},
	}

	cmd.Flags().Int("workers", runtime.GOMAXPROCS(0), "Number of concurrent workers")
	cmd.Flags().String("format", "html", "Output format (text, json, html)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance per scenario run")

	registerStrategyFlags(cmd, strategy)

	return cmd
}

func runStressTest(cmd *cobra.Command, strategy engine.Strategy, args []string) error {
	ctx := log.Logger.WithContext(context.Background())

	scenarios, err := resolveScenarios(args)
	if err != nil {
		return err
	}

	stressStudy := stress.New(scenarios)

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	cash, err := cmd.Flags().GetFloat64("cash")
	if err != nil {
		return fmt.Errorf("get --cash: %w", err)
	}

	opts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithInitialDeposit(cash),
		// Mark flag-set fields as user-applied so an explicit zero
		// (e.g. --sector-cap 0) survives engine hydration. Names are
		// determined from the registered strategy flags, so per-scenario
		// strategy instances built by the factory share the same set.
		engine.WithUserParams(strategyFlagFieldNames(cmd, strategy)...),
	}

	workers, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return err
	}

	runner := &study.Runner{
		Study:       stressStudy,
		NewStrategy: strategyFactoryWithFlags(strategy, cmd),
		Options:     opts,
		Workers:     workers,
	}

	progressCh, resultCh, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	// Drain progress channel with simple logging.
	for prog := range progressCh {
		switch prog.Status {
		case study.RunStarted:
			log.Info().Str("run", prog.RunName).Int("index", prog.RunIndex).Int("total", prog.TotalRuns).Msg("started")
		case study.RunCompleted:
			log.Info().Str("run", prog.RunName).Msg("completed")
		case study.RunFailed:
			log.Warn().Str("run", prog.RunName).Err(prog.Err).Msg("failed")
		}
	}

	result := <-resultCh
	if result.Err != nil {
		return fmt.Errorf("study analysis failed: %w", result.Err)
	}

	formatStr, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(formatStr)) {
	case "json":
		return result.Report.Data(os.Stdout)
	case "text":
		textReport, ok := result.Report.(interface{ Text(io.Writer) error })
		if !ok {
			return fmt.Errorf("study stress-test: report does not support text rendering")
		}

		return textReport.Text(os.Stdout)
	case "html", "":
		outputFile := "stress-test-report.html"

		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("create report file: %w", err)
		}
		defer file.Close()

		if err := report.Render(result.Report, file); err != nil {
			return fmt.Errorf("render report: %w", err)
		}

		log.Info().Str("path", outputFile).Msg("report written")

		return nil
	default:
		return fmt.Errorf("study stress-test: unknown --format %q (supported: text, json, html)", formatStr)
	}
}

// strategyFactory returns a function that creates fresh copies of the strategy
// by reflecting over the original and creating a new zero-value instance of the
// same concrete type.
func strategyFactory(original engine.Strategy) func() engine.Strategy {
	originalType := reflect.TypeOf(original)
	if originalType.Kind() == reflect.Pointer {
		originalType = originalType.Elem()
	}

	return func() engine.Strategy {
		return reflect.New(originalType).Interface().(engine.Strategy)
	}
}

// strategyFactoryWithFlags returns a factory that creates a fresh
// strategy instance and immediately applies the user's CLI flag values
// to it. Unlike strategyFactory, this propagates --flag values
// (including asset.Asset and universe.Universe fields handled by
// applyStrategyFlags) so each per-run strategy instance starts with
// the user's chosen configuration rather than struct-zero defaults.
func strategyFactoryWithFlags(original engine.Strategy, cmd *cobra.Command) func() engine.Strategy {
	originalType := reflect.TypeOf(original)
	if originalType.Kind() == reflect.Pointer {
		originalType = originalType.Elem()
	}

	return func() engine.Strategy {
		instance := reflect.New(originalType).Interface().(engine.Strategy)
		applyStrategyFlags(cmd, instance)

		return instance
	}
}

func resolveScenarios(args []string) ([]study.Scenario, error) {
	if len(args) == 0 || (len(args) == 1 && args[0] == "all") {
		return nil, nil // nil triggers default scenarios in stress.New
	}

	scenarios, err := study.ScenariosByName(args)
	if err != nil {
		return nil, fmt.Errorf("resolve scenarios: %w", err)
	}

	return scenarios, nil
}

// parseSimpleDuration parses a human-readable duration string such as
// "5y", "6m", or "30d". It supports y (years as 365 days), m (months as
// 30 days), d (days), h (hours), and falls back to time.ParseDuration for
// formats like "72h".
func parseSimpleDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Try standard Go duration parsing first (handles "72h", "30m", etc.).
	if dur, err := time.ParseDuration(input); err == nil {
		return dur, nil
	}

	unit := input[len(input)-1:]
	numStr := input[:len(input)-1]

	var multiplier time.Duration

	switch unit {
	case "y":
		multiplier = 365 * 24 * time.Hour
	case "m":
		multiplier = 30 * 24 * time.Hour
	case "d":
		multiplier = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unrecognised duration %q: use a number followed by y, m, or d (e.g. 5y, 6m, 30d)", input)
	}

	var count int

	if _, err := fmt.Sscanf(numStr, "%d", &count); err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", input, err)
	}

	return time.Duration(count) * multiplier, nil
}

// parseMetric maps a flag string to the corresponding portfolio.Rankable.
func parseMetric(metricStr string) (portfolio.Rankable, error) {
	switch strings.ToLower(strings.TrimSpace(metricStr)) {
	case "sharpe":
		return portfolio.Sharpe.(portfolio.Rankable), nil
	case "cagr":
		return portfolio.CAGR.(portfolio.Rankable), nil
	case "max-drawdown", "maxdrawdown":
		return portfolio.MaxDrawdown.(portfolio.Rankable), nil
	case "sortino":
		return portfolio.Sortino.(portfolio.Rankable), nil
	case "calmar":
		return portfolio.Calmar.(portfolio.Rankable), nil
	default:
		return nil, fmt.Errorf("unknown metric %q: choose from sharpe, cagr, max-drawdown, sortino, calmar", metricStr)
	}
}

// newOptimizeCmd builds the "study optimize" subcommand.
func newOptimizeCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Search for the best strategy parameters using cross-validation",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOptimize(cmd, strategy)
		},
	}

	cmd.Flags().String("search", "grid", "Search strategy (grid, random)")
	cmd.Flags().String("metric", "sharpe", "Objective metric (sharpe, cagr, max-drawdown, sortino, calmar)")
	cmd.Flags().String("validation", "train-test", "Validation scheme (train-test, kfold, walk-forward, scenario)")
	cmd.Flags().Int("folds", 5, "Number of folds for kfold validation")
	cmd.Flags().String("train-end", "", "Cutoff date for train/test split (YYYY-MM-DD)")
	cmd.Flags().String("min-train", "5y", "Minimum training window for walk-forward (e.g. 5y, 18m)")
	cmd.Flags().String("test-len", "2y", "Test window length for walk-forward (e.g. 2y, 6m)")
	cmd.Flags().String("step", "1y", "Step size for walk-forward (e.g. 1y, 6m)")
	cmd.Flags().Int("samples", 100, "Number of random samples (random search only)")
	cmd.Flags().Int("workers", runtime.GOMAXPROCS(0), "Number of concurrent workers")
	cmd.Flags().Int("top", 10, "Number of top parameter combinations to include in the report")
	cmd.Flags().String("format", "html", "Output format (text, json, html)")
	cmd.Flags().String("scenarios", "", "Comma-separated scenario names for scenario validation")
	cmd.Flags().Int("holdout", 1, "Number of scenarios to hold out per split (scenario validation)")
	cmd.Flags().Float64("cash", 100000, "Initial cash balance per parameter combination run")

	registerStrategyFlagsForSweep(cmd, strategy)

	return cmd
}

func runOptimize(cmd *cobra.Command, strategy engine.Strategy) error {
	ctx := log.Logger.WithContext(context.Background())

	// --- metric ---
	metricStr, err := cmd.Flags().GetString("metric")
	if err != nil {
		return fmt.Errorf("get --metric: %w", err)
	}

	metric, err := parseMetric(metricStr)
	if err != nil {
		return err
	}

	// --- validation splits ---
	validationStr, err := cmd.Flags().GetString("validation")
	if err != nil {
		return fmt.Errorf("get --validation: %w", err)
	}

	splits, err := buildSplits(cmd, validationStr)
	if err != nil {
		return err
	}

	// --- parameter sweeps from strategy flags ---
	sweeps := collectParamSweeps(cmd, strategy)
	if len(sweeps) == 0 {
		return fmt.Errorf("study optimize: no parameter ranges configured; pass at least one strategy flag using min:max:step syntax (e.g. --lookback 5:30:5)")
	}

	// Non-swept strategy flags become fixed values applied to every combo.
	fixedParams := collectFixedParams(cmd, strategy, sweeps)

	// --- search strategy ---
	searchStr, err := cmd.Flags().GetString("search")
	if err != nil {
		return fmt.Errorf("get --search: %w", err)
	}

	samples, err := cmd.Flags().GetInt("samples")
	if err != nil {
		return fmt.Errorf("get --samples: %w", err)
	}

	var searchStrategy study.SearchStrategy

	switch strings.ToLower(strings.TrimSpace(searchStr)) {
	case "grid":
		searchStrategy = study.NewGrid(sweeps...)
	case "random":
		searchStrategy = study.NewRandom(sweeps, samples, time.Now().UnixNano())
	default:
		return fmt.Errorf("unknown search strategy %q: choose from grid, random", searchStr)
	}

	// --- optimizer ---
	topN, err := cmd.Flags().GetInt("top")
	if err != nil {
		return fmt.Errorf("get --top: %w", err)
	}

	optimizer := optimize.New(splits,
		optimize.WithObjective(metric),
		optimize.WithTopN(topN),
		optimize.WithBaseParams(fixedParams),
	)

	// --- data provider ---
	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	cash, err := cmd.Flags().GetFloat64("cash")
	if err != nil {
		return fmt.Errorf("get --cash: %w", err)
	}

	opts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
		engine.WithInitialDeposit(cash),
	}

	workers, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return fmt.Errorf("get --workers: %w", err)
	}

	runner := &study.Runner{
		Study:          optimizer,
		NewStrategy:    strategyFactory(strategy),
		Options:        opts,
		Workers:        workers,
		SearchStrategy: searchStrategy,
		Splits:         splits,
		Objective:      metric,
	}

	progressCh, resultCh, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	for prog := range progressCh {
		switch prog.Status {
		case study.RunStarted:
			log.Info().Str("run", prog.RunName).Int("index", prog.RunIndex).Int("total", prog.TotalRuns).Msg("started")
		case study.RunCompleted:
			log.Info().Str("run", prog.RunName).Msg("completed")
		case study.RunFailed:
			log.Warn().Str("run", prog.RunName).Err(prog.Err).Msg("failed")
		}
	}

	result := <-resultCh
	if result.Err != nil {
		return fmt.Errorf("optimize study failed: %w", result.Err)
	}

	formatStr, err := cmd.Flags().GetString("format")
	if err != nil {
		return fmt.Errorf("get --format: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(formatStr)) {
	case "json":
		return result.Report.Data(os.Stdout)
	case "text":
		textReport, ok := result.Report.(interface{ Text(io.Writer) error })
		if !ok {
			return fmt.Errorf("study optimize: report does not support text rendering")
		}

		return textReport.Text(os.Stdout)
	case "html", "":
		outputFile := "optimize-report.html"

		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("create report file: %w", err)
		}
		defer file.Close()

		if err := report.Render(result.Report, file); err != nil {
			return fmt.Errorf("render report: %w", err)
		}

		log.Info().Str("path", outputFile).Msg("report written")

		return nil
	default:
		return fmt.Errorf("study optimize: unknown --format %q (supported: text, json, html)", formatStr)
	}
}

// buildSplits constructs the cross-validation splits based on the --validation flag.
func buildSplits(cmd *cobra.Command, validationStr string) ([]study.Split, error) {
	switch strings.ToLower(strings.TrimSpace(validationStr)) {
	case "train-test":
		return buildTrainTestSplits(cmd)
	case "kfold":
		return buildKFoldSplits(cmd)
	case "walk-forward":
		return buildWalkForwardSplits(cmd)
	case "scenario":
		return buildScenarioSplits(cmd)
	default:
		return nil, fmt.Errorf("unknown validation scheme %q: choose from train-test, kfold, walk-forward, scenario", validationStr)
	}
}

// buildTrainTestSplits builds a single train/test split.
// It requires --train-end and infers the outer range from the splits full range.
// Because the optimizer's Configurations method determines the date range from
// splits, we need sensible defaults. If --train-end is not supplied we fall back
// to a default split spanning 10y of history ending today.
func buildTrainTestSplits(cmd *cobra.Command) ([]study.Split, error) {
	trainEndStr, err := cmd.Flags().GetString("train-end")
	if err != nil {
		return nil, fmt.Errorf("get --train-end: %w", err)
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	end := now

	var cutoff time.Time
	if trainEndStr != "" {
		cutoff, err = time.Parse("2006-01-02", trainEndStr)
		if err != nil {
			return nil, fmt.Errorf("parse --train-end %q: %w", trainEndStr, err)
		}
	} else {
		// Default: 80% train, 20% test over 10 years.
		start := now.AddDate(-10, 0, 0)
		cutoff = start.Add(time.Duration(float64(end.Sub(start)) * 0.8))
	}

	start := cutoff.AddDate(-8, 0, 0) // provide some training history

	splits, err := study.TrainTest(start, cutoff, end)
	if err != nil {
		return nil, fmt.Errorf("build train/test split: %w", err)
	}

	return splits, nil
}

// buildKFoldSplits builds k-fold splits over a default 10-year window.
func buildKFoldSplits(cmd *cobra.Command) ([]study.Split, error) {
	folds, err := cmd.Flags().GetInt("folds")
	if err != nil {
		return nil, fmt.Errorf("get --folds: %w", err)
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	start := now.AddDate(-10, 0, 0)

	splits, err := study.KFold(start, now, folds)
	if err != nil {
		return nil, fmt.Errorf("build k-fold splits: %w", err)
	}

	return splits, nil
}

// buildWalkForwardSplits builds walk-forward splits over a default 15-year window.
func buildWalkForwardSplits(cmd *cobra.Command) ([]study.Split, error) {
	minTrainStr, err := cmd.Flags().GetString("min-train")
	if err != nil {
		return nil, fmt.Errorf("get --min-train: %w", err)
	}

	testLenStr, err := cmd.Flags().GetString("test-len")
	if err != nil {
		return nil, fmt.Errorf("get --test-len: %w", err)
	}

	stepStr, err := cmd.Flags().GetString("step")
	if err != nil {
		return nil, fmt.Errorf("get --step: %w", err)
	}

	minTrain, err := parseSimpleDuration(minTrainStr)
	if err != nil {
		return nil, fmt.Errorf("parse --min-train: %w", err)
	}

	testLen, err := parseSimpleDuration(testLenStr)
	if err != nil {
		return nil, fmt.Errorf("parse --test-len: %w", err)
	}

	step, err := parseSimpleDuration(stepStr)
	if err != nil {
		return nil, fmt.Errorf("parse --step: %w", err)
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	start := now.AddDate(-15, 0, 0)

	splits, err := study.WalkForward(start, now, minTrain, testLen, step)
	if err != nil {
		return nil, fmt.Errorf("build walk-forward splits: %w", err)
	}

	return splits, nil
}

// buildScenarioSplits builds scenario leave-n-out splits.
func buildScenarioSplits(cmd *cobra.Command) ([]study.Split, error) {
	scenariosStr, err := cmd.Flags().GetString("scenarios")
	if err != nil {
		return nil, fmt.Errorf("get --scenarios: %w", err)
	}

	holdout, err := cmd.Flags().GetInt("holdout")
	if err != nil {
		return nil, fmt.Errorf("get --holdout: %w", err)
	}

	var scenarios []study.Scenario

	if scenariosStr == "" {
		scenarios = study.AllScenarios()
	} else {
		names := strings.Split(scenariosStr, ",")
		for idx := range names {
			names[idx] = strings.TrimSpace(names[idx])
		}

		scenarios, err = study.ScenariosByName(names)
		if err != nil {
			return nil, fmt.Errorf("resolve --scenarios: %w", err)
		}
	}

	splits, err := study.ScenarioLeaveNOut(scenarios, holdout)
	if err != nil {
		return nil, fmt.Errorf("build scenario splits: %w", err)
	}

	return splits, nil
}
