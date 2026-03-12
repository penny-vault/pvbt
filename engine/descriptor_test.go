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
	"encoding/json"
	"testing"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/tradecron"
)

// descriptorStrategy implements both Strategy and Descriptor.
type descriptorStrategy struct {
	Lookback int `pvbt:"lookback" desc:"Lookback period" default:"6"`
	Tickers  string `pvbt:"tickers" desc:"Asset tickers" default:"SPY,QQQ" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
}

func (s *descriptorStrategy) Name() string                                                      { return "DescriptorTest" }
func (s *descriptorStrategy) Setup(e *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(err)
	}
	e.Schedule(tc)
	e.SetBenchmark(asset.Asset{Ticker: "SPY"})
	e.RiskFreeAsset(asset.Asset{Ticker: "SHV"})
}
func (s *descriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}
func (s *descriptorStrategy) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode:   "dt",
		Description: "A test strategy with descriptor",
		Source:      "unit test",
		Version:     "1.0.0",
	}
}

// plainStrategy implements only Strategy (no Descriptor).
type plainStrategy struct {
	Window int `pvbt:"window" desc:"Rolling window" default:"12"`
}

func (s *plainStrategy) Name() string                                                      { return "PlainTest" }
func (s *plainStrategy) Setup(e *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(err)
	}
	e.Schedule(tc)
}
func (s *plainStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}

func TestDescribeStrategyWithDescriptor(t *testing.T) {
	s := &descriptorStrategy{}
	e := engine.New(s)
	s.Setup(e)

	info := engine.DescribeStrategy(e)

	if info.Name != "DescriptorTest" {
		t.Errorf("expected name DescriptorTest, got %q", info.Name)
	}
	if info.ShortCode != "dt" {
		t.Errorf("expected shortcode dt, got %q", info.ShortCode)
	}
	if info.Description != "A test strategy with descriptor" {
		t.Errorf("expected description, got %q", info.Description)
	}
	if info.Source != "unit test" {
		t.Errorf("expected source 'unit test', got %q", info.Source)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %q", info.Version)
	}
	if info.Schedule != "0 16 * * 1-5" {
		t.Errorf("expected schedule '0 16 * * 1-5', got %q", info.Schedule)
	}
	if info.Benchmark != "SPY" {
		t.Errorf("expected benchmark SPY, got %q", info.Benchmark)
	}
	if info.RiskFree != "SHV" {
		t.Errorf("expected riskFree SHV, got %q", info.RiskFree)
	}

	// Check parameters.
	if len(info.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(info.Parameters))
	}

	// Check suggestions are pivoted correctly.
	if info.Suggestions == nil {
		t.Fatal("expected suggestions to be populated")
	}
	if len(info.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestion presets, got %d", len(info.Suggestions))
	}
	classic, ok := info.Suggestions["Classic"]
	if !ok {
		t.Fatal("expected 'Classic' suggestion preset")
	}
	if classic["tickers"] != "VFINX,PRIDX" {
		t.Errorf("expected Classic.tickers=VFINX,PRIDX, got %q", classic["tickers"])
	}

	// Verify JSON round-trip.
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded engine.StrategyInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.Name != info.Name {
		t.Errorf("round-trip name mismatch: %q vs %q", decoded.Name, info.Name)
	}
	if decoded.ShortCode != info.ShortCode {
		t.Errorf("round-trip shortcode mismatch: %q vs %q", decoded.ShortCode, info.ShortCode)
	}
}

func TestDescribeStrategyWithoutDescriptor(t *testing.T) {
	s := &plainStrategy{}
	e := engine.New(s)
	s.Setup(e)

	info := engine.DescribeStrategy(e)

	if info.Name != "PlainTest" {
		t.Errorf("expected name PlainTest, got %q", info.Name)
	}
	if info.ShortCode != "" {
		t.Errorf("expected empty shortcode, got %q", info.ShortCode)
	}
	if info.Description != "" {
		t.Errorf("expected empty description, got %q", info.Description)
	}
	if info.Benchmark != "" {
		t.Errorf("expected empty benchmark, got %q", info.Benchmark)
	}
	if info.RiskFree != "" {
		t.Errorf("expected empty riskFree, got %q", info.RiskFree)
	}
	if info.Suggestions != nil {
		t.Errorf("expected nil suggestions, got %v", info.Suggestions)
	}

	// Check parameter.
	if len(info.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(info.Parameters))
	}
	if info.Parameters[0].Name != "window" {
		t.Errorf("expected parameter name 'window', got %q", info.Parameters[0].Name)
	}
	if info.Parameters[0].Type != "int" {
		t.Errorf("expected parameter type 'int', got %q", info.Parameters[0].Type)
	}

	// Verify JSON omits empty fields.
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(data)

	// shortcode, description, source, version should not appear.
	for _, field := range []string{`"shortcode"`, `"source"`, `"version"`} {
		if contains(jsonStr, field) {
			t.Errorf("expected %s to be omitted from JSON, got: %s", field, jsonStr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
