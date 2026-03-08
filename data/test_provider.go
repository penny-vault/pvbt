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

package data

import (
	"context"

	"github.com/rs/zerolog"
)

// Compile-time interface check.
var _ BatchProvider = (*TestProvider)(nil)

// TestProvider is a BatchProvider backed by a predetermined DataFrame.
// It is intended for use in tests and simulations where data is known
// ahead of time.
type TestProvider struct {
	frame   *DataFrame
	metrics []Metric
}

// NewTestProvider returns a TestProvider that serves the given metrics
// from the given DataFrame.
func NewTestProvider(metrics []Metric, frame *DataFrame) *TestProvider {
	return &TestProvider{
		frame:   frame,
		metrics: metrics,
	}
}

// Provides returns the set of metrics this provider can supply.
func (p *TestProvider) Provides() []Metric {
	return p.metrics
}

// Fetch narrows the stored DataFrame to the request's assets, metrics,
// and time range.
func (p *TestProvider) Fetch(ctx context.Context, req DataRequest) (*DataFrame, error) {
	log := zerolog.Ctx(ctx)
	log.Debug().
		Int("assets", len(req.Assets)).
		Int("metrics", len(req.Metrics)).
		Time("start", req.Start).
		Time("end", req.End).
		Msg("TestProvider.Fetch")

	result := p.frame.Assets(req.Assets...).Metrics(req.Metrics...).Between(req.Start, req.End)

	return result, nil
}

// Close is a no-op for TestProvider.
func (p *TestProvider) Close() error {
	return nil
}
