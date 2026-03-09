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

package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/broker"
)

// mockBroker records submitted orders and returns pre-configured fills.
type mockBroker struct {
	submitted []broker.Order
	fills     []broker.Fill // one Fill per Submit call, consumed in order
	callIdx   int
}

func (m *mockBroker) Connect(context.Context) error              { return nil }
func (m *mockBroker) Close() error                               { return nil }
func (m *mockBroker) Cancel(string) error                        { return nil }
func (m *mockBroker) Replace(string, broker.Order) (broker.Fill, error) {
	return broker.Fill{}, nil
}
func (m *mockBroker) Orders() ([]broker.Order, error)       { return nil, nil }
func (m *mockBroker) Positions() ([]broker.Position, error) { return nil, nil }
func (m *mockBroker) Balance() (broker.Balance, error)      { return broker.Balance{}, nil }

func (m *mockBroker) Submit(order broker.Order) (broker.Fill, error) {
	m.submitted = append(m.submitted, order)
	if m.callIdx < len(m.fills) {
		fill := m.fills[m.callIdx]
		m.callIdx++
		return fill, nil
	}
	return broker.Fill{
		Price:    100.0,
		Qty:      order.Qty,
		FilledAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}, nil
}

var _ broker.Broker = (*mockBroker)(nil)

var testAsset = asset.Asset{CompositeFigi: "TEST001", Ticker: "TEST"}

func TestOrderMarketBuy(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))

	acct.Order(testAsset, Buy, 10)

	// Verify broker received the correct order.
	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	ord := mb.submitted[0]
	if ord.Side != broker.Buy {
		t.Errorf("expected broker.Buy, got %d", ord.Side)
	}
	if ord.Qty != 10 {
		t.Errorf("expected qty 10, got %f", ord.Qty)
	}
	if ord.OrderType != broker.Market {
		t.Errorf("expected Market order type, got %d", ord.OrderType)
	}
	if ord.TimeInForce != broker.Day {
		t.Errorf("expected Day TIF, got %d", ord.TimeInForce)
	}

	// Cash should decrease by price * qty = 100 * 10 = 1000.
	if acct.Cash() != 9000 {
		t.Errorf("expected cash 9000, got %f", acct.Cash())
	}

	// Position should be 10 shares.
	if acct.Position(testAsset) != 10 {
		t.Errorf("expected position 10, got %f", acct.Position(testAsset))
	}
}

func TestOrderLimitBuy(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))

	acct.Order(testAsset, Buy, 5, Limit(50.0))

	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	ord := mb.submitted[0]
	if ord.OrderType != broker.Limit {
		t.Errorf("expected Limit order type, got %d", ord.OrderType)
	}
	if ord.LimitPrice != 50.0 {
		t.Errorf("expected LimitPrice 50.0, got %f", ord.LimitPrice)
	}
}

func TestOrderStopBuy(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))

	acct.Order(testAsset, Buy, 5, Stop(45.0))

	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	ord := mb.submitted[0]
	if ord.OrderType != broker.Stop {
		t.Errorf("expected Stop order type, got %d", ord.OrderType)
	}
	if ord.StopPrice != 45.0 {
		t.Errorf("expected StopPrice 45.0, got %f", ord.StopPrice)
	}
}

func TestOrderStopLimitBuy(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))

	acct.Order(testAsset, Buy, 5, Limit(50.0), Stop(45.0))

	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	ord := mb.submitted[0]
	if ord.OrderType != broker.StopLimit {
		t.Errorf("expected StopLimit order type, got %d", ord.OrderType)
	}
	if ord.LimitPrice != 50.0 {
		t.Errorf("expected LimitPrice 50.0, got %f", ord.LimitPrice)
	}
	if ord.StopPrice != 45.0 {
		t.Errorf("expected StopPrice 45.0, got %f", ord.StopPrice)
	}
}

func TestOrderGoodTilCancel(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))

	acct.Order(testAsset, Buy, 5, GoodTilCancel)

	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	if mb.submitted[0].TimeInForce != broker.GTC {
		t.Errorf("expected GTC, got %d", mb.submitted[0].TimeInForce)
	}
}

func TestOrderSell(t *testing.T) {
	fillTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	mb := &mockBroker{
		fills: []broker.Fill{
			{Price: 100.0, Qty: 10, FilledAt: fillTime},
			{Price: 105.0, Qty: 10, FilledAt: fillTime},
		},
	}
	acct := New(WithCash(10000), WithBroker(mb))

	// Buy first, then sell.
	acct.Order(testAsset, Buy, 10)
	acct.Order(testAsset, Sell, 10)

	if len(mb.submitted) != 2 {
		t.Fatalf("expected 2 submitted orders, got %d", len(mb.submitted))
	}

	sellOrd := mb.submitted[1]
	if sellOrd.Side != broker.Sell {
		t.Errorf("expected broker.Sell, got %d", sellOrd.Side)
	}

	// Cash: 10000 - 1000 (buy) + 1050 (sell) = 10050.
	if acct.Cash() != 10050 {
		t.Errorf("expected cash 10050, got %f", acct.Cash())
	}

	// Position should be 0 after selling.
	if acct.Position(testAsset) != 0 {
		t.Errorf("expected position 0, got %f", acct.Position(testAsset))
	}
}

func TestOrderGoodTilDate(t *testing.T) {
	mb := &mockBroker{}
	acct := New(WithCash(10000), WithBroker(mb))
	gtdDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	acct.Order(testAsset, Buy, 5, GoodTilDate(gtdDate))

	if len(mb.submitted) != 1 {
		t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
	}
	ord := mb.submitted[0]
	if ord.TimeInForce != broker.GTD {
		t.Errorf("expected GTD, got %d", ord.TimeInForce)
	}
	if !ord.GTDDate.Equal(gtdDate) {
		t.Errorf("expected GTDDate %v, got %v", gtdDate, ord.GTDDate)
	}
}

func TestOrderTimeInForceModifiers(t *testing.T) {
	tests := []struct {
		name string
		mod  OrderModifier
		want broker.TimeInForce
	}{
		{"DayOrder", DayOrder, broker.Day},
		{"GoodTilCancel", GoodTilCancel, broker.GTC},
		{"FillOrKill", FillOrKill, broker.FOK},
		{"ImmediateOrCancel", ImmediateOrCancel, broker.IOC},
		{"OnTheOpen", OnTheOpen, broker.OnOpen},
		{"OnTheClose", OnTheClose, broker.OnClose},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mb := &mockBroker{}
			acct := New(WithCash(10000), WithBroker(mb))
			acct.Order(testAsset, Buy, 1, tc.mod)
			if len(mb.submitted) != 1 {
				t.Fatalf("expected 1 submitted order, got %d", len(mb.submitted))
			}
			if mb.submitted[0].TimeInForce != tc.want {
				t.Errorf("expected TIF %d, got %d", tc.want, mb.submitted[0].TimeInForce)
			}
		})
	}
}
