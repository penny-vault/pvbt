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
	"reflect"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/penny-vault/pvbt/universe"
)

type paramTestStrategy struct {
	Lookback float64           `pvbt:"lookback" desc:"Lookback in months" default:"6.0"`
	Ticker   asset.Asset       `pvbt:"ticker" desc:"Primary ticker" default:"SPY"`
	RiskOn   universe.Universe `pvbt:"riskOn" desc:"Risk-on universe" default:"VFINX,PRIDX" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
	Name_    string            // exported, no pvbt tag -- name derived from field name
	hidden   int               // unexported, should be skipped
	Duration time.Duration     `pvbt:"dur" desc:"Interval" default:"5m"`
	Enabled  bool              `pvbt:"enabled" desc:"Enable feature" default:"true"`
	Count    int               `pvbt:"count" desc:"Number of items" default:"10"`
	Label    string            `pvbt:"label" desc:"Display label" default:"hello"`
}

func (s *paramTestStrategy) Name() string                                                      { return "test" }
func (s *paramTestStrategy) Setup(_ *engine.Engine)                                            {}
func (s *paramTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}

func TestStrategyParameters(t *testing.T) {
	s := &paramTestStrategy{}
	params := engine.StrategyParameters(s)

	// Should include exported fields only: Lookback, Ticker, RiskOn, Name_, Duration, Enabled, Count, Label = 8
	if len(params) != 8 {
		t.Fatalf("expected 8 parameters, got %d", len(params))
	}

	// Check first param
	p := findParam(params, "lookback")
	if p == nil {
		t.Fatal("expected parameter 'lookback'")
	}
	if p.Description != "Lookback in months" {
		t.Errorf("expected desc 'Lookback in months', got %q", p.Description)
	}
	if p.Default != "6.0" {
		t.Errorf("expected default '6.0', got %q", p.Default)
	}
	if p.GoType != reflect.TypeOf(float64(0)) {
		t.Errorf("expected float64 type, got %v", p.GoType)
	}

	// Check field with no pvbt tag -- name derived from field name lowercased
	p2 := findParam(params, "name_")
	if p2 == nil {
		t.Fatal("expected parameter 'name_' (lowercased from Name_)")
	}
	if p2.FieldName != "Name_" {
		t.Errorf("expected FieldName 'Name_', got %q", p2.FieldName)
	}
}

func TestStrategyParametersSuggestions(t *testing.T) {
	s := &paramTestStrategy{}
	params := engine.StrategyParameters(s)

	p := findParam(params, "riskOn")
	if p == nil {
		t.Fatal("expected parameter 'riskOn'")
	}
	if p.Suggestions == nil {
		t.Fatal("expected Suggestions map to be populated")
	}
	if len(p.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(p.Suggestions))
	}
	if v, ok := p.Suggestions["Classic"]; !ok || v != "VFINX,PRIDX" {
		t.Errorf("expected Classic=VFINX,PRIDX, got %q", v)
	}
	if v, ok := p.Suggestions["Modern"]; !ok || v != "SPY,QQQ" {
		t.Errorf("expected Modern=SPY,QQQ, got %q", v)
	}

	// Fields without suggest tag should have nil Suggestions.
	p2 := findParam(params, "lookback")
	if p2 == nil {
		t.Fatal("expected parameter 'lookback'")
	}
	if p2.Suggestions != nil {
		t.Errorf("expected nil Suggestions for lookback, got %v", p2.Suggestions)
	}
}

func findParam(params []engine.Parameter, name string) *engine.Parameter {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}
