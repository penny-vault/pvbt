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
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
	"github.com/penny-vault/pvbt/engine"
)

func TestSimulatedBrokerSubmitMarketOrder(t *testing.T) {
	aapl := asset.Asset{CompositeFigi: "FIGI-AAPL", Ticker: "AAPL"}
	date := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)

	sb := engine.NewSimulatedBroker()
	sb.SetPrices(func(a asset.Asset) (float64, bool) {
		if a.CompositeFigi == "FIGI-AAPL" {
			return 150.0, true
		}
		return 0, false
	}, date)

	fills, err := sb.Submit(broker.Order{
		Asset:     aapl,
		Side:      broker.Buy,
		Qty:       100,
		OrderType: broker.Market,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}
	if fills[0].Price != 150.0 {
		t.Errorf("expected price 150.0, got %f", fills[0].Price)
	}
	if fills[0].Qty != 100 {
		t.Errorf("expected qty 100, got %f", fills[0].Qty)
	}
	if !fills[0].FilledAt.Equal(date) {
		t.Errorf("expected fill at %v, got %v", date, fills[0].FilledAt)
	}
}

func TestSimulatedBrokerSubmitUnknownAsset(t *testing.T) {
	unknown := asset.Asset{CompositeFigi: "FIGI-UNKNOWN", Ticker: "UNKNOWN"}
	date := time.Date(2025, 6, 15, 16, 0, 0, 0, time.UTC)

	sb := engine.NewSimulatedBroker()
	sb.SetPrices(func(_ asset.Asset) (float64, bool) {
		return 0, false
	}, date)

	_, err := sb.Submit(broker.Order{
		Asset:     unknown,
		Side:      broker.Buy,
		Qty:       100,
		OrderType: broker.Market,
	})

	if err == nil {
		t.Fatal("expected error for unknown asset")
	}
}

func TestSimulatedBrokerConnectClose(t *testing.T) {
	sb := engine.NewSimulatedBroker()
	if err := sb.Connect(context.Background()); err != nil {
		t.Fatalf("Connect should succeed: %v", err)
	}
	if err := sb.Close(); err != nil {
		t.Fatalf("Close should succeed: %v", err)
	}
}

// Compile-time interface check.
var _ broker.Broker = (*engine.SimulatedBroker)(nil)
