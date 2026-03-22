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
	"os"
	"reflect"
	"runtime"

	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/report"
	"github.com/penny-vault/pvbt/study"
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
	cmd.Flags().String("format", "text", "Output format (text, json)")

	return cmd
}

func runStressTest(cmd *cobra.Command, strategy engine.Strategy, args []string) error {
	ctx := log.Logger.WithContext(context.Background())

	scenarios := resolveScenarios(args)
	stressStudy := stress.New(scenarios)

	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}

	opts := []engine.Option{
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(provider),
	}

	workers, err := cmd.Flags().GetInt("workers")
	if err != nil {
		return err
	}

	runner := &study.Runner{
		Study:       stressStudy,
		NewStrategy: strategyFactory(strategy),
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

	return result.Report.Render(report.Format(formatStr), os.Stdout)
}

// strategyFactory returns a function that creates fresh copies of the strategy
// by reflecting over the original and creating a new zero-value instance of the
// same concrete type.
func strategyFactory(original engine.Strategy) func() engine.Strategy {
	originalType := reflect.TypeOf(original)
	if originalType.Kind() == reflect.Ptr {
		originalType = originalType.Elem()
	}

	return func() engine.Strategy {
		return reflect.New(originalType).Interface().(engine.Strategy)
	}
}

func resolveScenarios(args []string) []stress.Scenario {
	if len(args) == 0 || (len(args) == 1 && args[0] == "all") {
		return nil // nil triggers default scenarios
	}

	defaults := stress.DefaultScenarios()

	byName := make(map[string]stress.Scenario)
	for _, scenario := range defaults {
		byName[scenario.Name] = scenario
	}

	var selected []stress.Scenario

	for _, name := range args {
		if scenario, ok := byName[name]; ok {
			selected = append(selected, scenario)
		}
	}

	return selected
}
