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

package engine_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// liveStrategy is a minimal strategy that sets a schedule and does nothing in Compute.
type liveStrategy struct{}

func (s *liveStrategy) Name() string { return "liveStrategy" }

func (s *liveStrategy) Setup(e *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic("liveStrategy.Setup: " + err.Error())
	}
	e.Schedule(tc)
}

func (s *liveStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}

// TestRunLiveContextCancel verifies that RunLive returns a channel and no error,
// and that the channel is closed when the context is cancelled.
func TestRunLiveContextCancel(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	assets := []asset.Asset{aapl}

	dataStart := time.Now().AddDate(0, 0, -30)
	metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	df := makeDailyTestData(t, dataStart, 60, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	assetProvider := &mockAssetProvider{assets: assets}
	strategy := &liveStrategy{}

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(assetProvider),
	)

	broker := engine.NewSimulatedBroker()
	acct := portfolio.New(
		portfolio.WithCash(100_000.0),
		portfolio.WithBroker(broker),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := eng.RunLive(ctx, acct)
	if err != nil {
		t.Fatalf("RunLive returned unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("RunLive returned nil channel")
	}

	// Drain the channel until it is closed (context timeout triggers close).
	for range ch {
	}
	// Reaching here means the channel was closed without panicking.
}

// TestRunLiveNoBroker verifies that RunLive returns an error when the account
// has no broker set.
func TestRunLiveNoBroker(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	assets := []asset.Asset{aapl}

	dataStart := time.Now().AddDate(0, 0, -30)
	metrics := []data.Metric{data.MetricClose, data.AdjClose, data.Dividend}
	df := makeDailyTestData(t, dataStart, 60, assets, metrics)
	provider := data.NewTestProvider(metrics, df)

	assetProvider := &mockAssetProvider{assets: assets}
	strategy := &liveStrategy{}

	eng := engine.New(strategy,
		engine.WithDataProvider(provider),
		engine.WithAssetProvider(assetProvider),
	)

	// Account without a broker.
	acct := portfolio.New(portfolio.WithCash(100_000.0))

	_, err := eng.RunLive(context.Background(), acct)
	if err == nil {
		t.Fatal("expected error for account with no broker, got nil")
	}
	if !strings.Contains(err.Error(), "broker") {
		t.Errorf("expected error message to mention broker, got: %v", err)
	}
}

// TestRunLiveNoSchedule verifies that RunLive returns an error when the
// strategy does not set a schedule during Setup.
func TestRunLiveNoSchedule(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	assets := []asset.Asset{aapl}
	assetProvider := &mockAssetProvider{assets: assets}
	strategy := &noScheduleStrategy{}

	eng := engine.New(strategy, engine.WithAssetProvider(assetProvider))

	broker := engine.NewSimulatedBroker()
	acct := portfolio.New(
		portfolio.WithCash(100_000.0),
		portfolio.WithBroker(broker),
	)

	_, err := eng.RunLive(context.Background(), acct)
	if err == nil {
		t.Fatal("expected error for missing schedule, got nil")
	}
	if !strings.Contains(err.Error(), "schedule") {
		t.Errorf("expected error message to mention schedule, got: %v", err)
	}
}
