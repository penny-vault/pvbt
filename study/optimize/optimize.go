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

package optimize

import (
	"context"
	"fmt"
	"maps"

	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/study/report"
)

// Ensure Optimizer satisfies the study.Study interface at compile time.
var _ study.Study = (*Optimizer)(nil)

// Optimizer is a study.Study that evaluates strategy parameter combinations
// across cross-validation splits and ranks them by out-of-sample performance.
type Optimizer struct {
	splits     []study.Split
	objective  portfolio.Rankable
	topN       int
	baseParams map[string]string
}

// Option configures an Optimizer.
type Option func(*Optimizer)

// WithObjective sets the metric used to rank parameter combinations.
func WithObjective(metric portfolio.Rankable) Option {
	return func(opt *Optimizer) {
		opt.objective = metric
	}
}

// WithTopN sets the number of top combinations to include in the equity
// curve section of the report.
func WithTopN(topN int) Option {
	return func(opt *Optimizer) {
		opt.topN = topN
	}
}

// WithBaseParams sets parameter values applied to every parameter
// combination before the swept value is overlaid. Use this to fix
// non-swept strategy flags (e.g. when sweeping --sector-cap but
// pinning --top-holdings to a specific value).
func WithBaseParams(params map[string]string) Option {
	return func(opt *Optimizer) {
		if len(params) == 0 {
			opt.baseParams = nil
			return
		}

		opt.baseParams = maps.Clone(params)
	}
}

// New creates an Optimizer for the given cross-validation splits.
// Default objective is Sharpe; default topN is 10.
func New(splits []study.Split, opts ...Option) *Optimizer {
	opt := &Optimizer{
		splits:    splits,
		objective: portfolio.Sharpe.(portfolio.Rankable),
		topN:      10,
	}

	for _, fn := range opts {
		fn(opt)
	}

	return opt
}

// Name returns the human-readable study name.
func (opt *Optimizer) Name() string { return "Parameter Optimization" }

// Description returns a short explanation of what the study does.
func (opt *Optimizer) Description() string {
	return "Evaluate parameter combinations across cross-validation splits and rank by out-of-sample performance"
}

// Configurations returns a single RunConfig whose date range spans the
// earliest start and latest end across all configured splits.
func (opt *Optimizer) Configurations(_ context.Context) ([]study.RunConfig, error) {
	if len(opt.splits) == 0 {
		return nil, fmt.Errorf("optimizer: no splits configured")
	}

	earliest := opt.splits[0].FullRange.Start
	latest := opt.splits[0].FullRange.End

	for _, sp := range opt.splits[1:] {
		if sp.FullRange.Start.Before(earliest) {
			earliest = sp.FullRange.Start
		}

		if sp.FullRange.End.After(latest) {
			latest = sp.FullRange.End
		}
	}

	cfg := study.RunConfig{
		Name:  "Full Range",
		Start: earliest,
		End:   latest,
		Metadata: map[string]string{
			"study": "parameter-optimization",
		},
	}

	if len(opt.baseParams) > 0 {
		cfg.Params = maps.Clone(opt.baseParams)
	}

	return []study.RunConfig{cfg}, nil
}

// Analyze processes all RunResults, groups them by combination and split,
// ranks by out-of-sample objective score, and builds the optimization report.
func (opt *Optimizer) Analyze(results []study.RunResult) (report.Report, error) {
	return analyzeResults(opt.splits, opt.objective, opt.topN, results)
}
