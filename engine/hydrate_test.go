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

package engine

import (
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/universe"
)

type hydrateTarget struct {
	FloatVal    float64           `default:"3.14"`
	IntVal      int               `default:"42"`
	StringVal   string            `default:"hello"`
	BoolVal     bool              `default:"true"`
	DurationVal time.Duration     `default:"5m"`
	AssetVal    asset.Asset       `default:"AAPL"`
	UniverseVal universe.Universe `default:"AAPL,GOOG"`
	NoDefault   float64
	PreSet      float64 `default:"99.0"`
}

func TestHydrateScalarFields(t *testing.T) {
	target := &hydrateTarget{PreSet: 1.0}

	e := &Engine{
		assets: map[string]asset.Asset{
			"AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
			"GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
		},
	}

	hydrateFields(e, target)

	if target.FloatVal != 3.14 {
		t.Errorf("FloatVal: expected 3.14, got %f", target.FloatVal)
	}
	if target.IntVal != 42 {
		t.Errorf("IntVal: expected 42, got %d", target.IntVal)
	}
	if target.StringVal != "hello" {
		t.Errorf("StringVal: expected 'hello', got %q", target.StringVal)
	}
	if target.BoolVal != true {
		t.Errorf("BoolVal: expected true, got %v", target.BoolVal)
	}
	if target.DurationVal != 5*time.Minute {
		t.Errorf("DurationVal: expected 5m, got %v", target.DurationVal)
	}
	if target.NoDefault != 0 {
		t.Errorf("NoDefault: expected 0, got %f", target.NoDefault)
	}
	// PreSet should NOT be overwritten.
	if target.PreSet != 1.0 {
		t.Errorf("PreSet: expected 1.0 (not overwritten), got %f", target.PreSet)
	}
}

func TestHydrateAssetField(t *testing.T) {
	target := &hydrateTarget{}
	e := &Engine{
		assets: map[string]asset.Asset{
			"AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
			"GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
		},
	}

	hydrateFields(e, target)

	if target.AssetVal.CompositeFigi != "FIGI-AAPL" {
		t.Errorf("AssetVal: expected FIGI-AAPL, got %q", target.AssetVal.CompositeFigi)
	}
}

func TestHydrateUniverseField(t *testing.T) {
	target := &hydrateTarget{}
	e := &Engine{
		assets: map[string]asset.Asset{
			"AAPL": {CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"},
			"GOOG": {CompositeFigi: "FIGI-GOOG", Ticker: "GOOG"},
		},
	}

	hydrateFields(e, target)

	if target.UniverseVal == nil {
		t.Fatal("UniverseVal should not be nil")
	}
	members := target.UniverseVal.Assets(time.Now())
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].CompositeFigi != "FIGI-AAPL" {
		t.Errorf("first member: expected FIGI-AAPL, got %q", members[0].CompositeFigi)
	}
}
