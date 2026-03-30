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

package ibkr

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bytedance/sonic"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	maxReconnectAttempts     = 3
)

// ibWSMessage represents an incoming WebSocket message from the IB streamer.
type ibWSMessage struct {
	Topic string          `json:"topic"`
	Args  json.RawMessage `json:"args"`
}

// ibWSFillArgs holds the fill-relevant fields from a streamer "sor" message.
type ibWSFillArgs struct {
	OrderID        string  `json:"orderId"`
	Status         string  `json:"status"`
	FilledQuantity float64 `json:"filledQuantity"`
	AvgPrice       float64 `json:"avgPrice"`
}

// orderStreamer manages a WebSocket connection to the IB order streamer,
// delivering fill updates on the fills channel.
type orderStreamer struct {
	wsURL             string
	conn              *websocket.Conn
	fills             chan<- broker.Fill
	seenFills         map[string]time.Time
	heartbeatInterval time.Duration
	tradesFn          func(ctx context.Context) ([]ibTradeEntry, error)
	cancel            context.CancelFunc
	mu                sync.Mutex
}

// newOrderStreamer creates an orderStreamer that sends fills on the given
// channel and connects to the given WebSocket URL.
func newOrderStreamer(fills chan broker.Fill, wsURL string, tradesFn func(ctx context.Context) ([]ibTradeEntry, error)) *orderStreamer {
	return &orderStreamer{
		wsURL:             wsURL,
		fills:             fills,
		seenFills:         make(map[string]time.Time),
		heartbeatInterval: defaultHeartbeatInterval,
		tradesFn:          tradesFn,
	}
}

// connect dials the WebSocket, sends the subscription message, and starts
// the read and heartbeat loops.
func (os *orderStreamer) connect(ctx context.Context) error {
	conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, os.wsURL, nil)
	if dialErr != nil {
		return fmt.Errorf("ibkr streamer connect: %w", dialErr)
	}

	os.mu.Lock()
	os.conn = conn
	os.mu.Unlock()

	if writeErr := conn.WriteMessage(websocket.TextMessage, []byte("sor+{}")); writeErr != nil {
		conn.Close()
		return fmt.Errorf("ibkr streamer subscribe: %w", writeErr)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	os.cancel = cancel

	go os.readLoop(streamCtx)
	go os.heartbeatLoop(streamCtx)

	return nil
}

// readLoop reads JSON messages from the WebSocket and dispatches fills.
func (os *orderStreamer) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		os.mu.Lock()
		conn := os.conn
		os.mu.Unlock()

		if conn == nil {
			return
		}

		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if reconnErr := os.reconnect(ctx); reconnErr != nil {
				log.Error().Err(reconnErr).Msg("ibkr: streamer reconnect failed")
				return
			}

			continue
		}

		var msg ibWSMessage
		if unmarshalErr := sonic.Unmarshal(data, &msg); unmarshalErr != nil {
			continue
		}

		if msg.Topic != "sor" {
			continue
		}

		var args ibWSFillArgs
		if unmarshalErr := sonic.Unmarshal(msg.Args, &args); unmarshalErr != nil {
			continue
		}

		if args.Status != "Filled" && args.Status != "PartiallyFilled" {
			continue
		}

		os.deliverFill(ctx, args.OrderID, args.FilledQuantity, args.AvgPrice)
	}
}

// heartbeatLoop sends a "tic" text message at the configured interval.
func (os *orderStreamer) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(os.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			os.mu.Lock()
			conn := os.conn
			os.mu.Unlock()

			if conn == nil {
				return
			}

			if writeErr := conn.WriteMessage(websocket.TextMessage, []byte("tic")); writeErr != nil {
				log.Warn().Err(writeErr).Msg("ibkr: heartbeat write failed")
				return
			}
		}
	}
}

// reconnect attempts to re-establish the WebSocket connection with
// exponential backoff. On success it re-subscribes and polls for missed fills.
func (os *orderStreamer) reconnect(ctx context.Context) error {
	os.mu.Lock()
	if os.conn != nil {
		os.conn.Close()
		os.conn = nil
	}
	os.mu.Unlock()

	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}

			backoff *= 2
		}

		conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, os.wsURL, nil)
		if dialErr != nil {
			continue
		}

		if writeErr := conn.WriteMessage(websocket.TextMessage, []byte("sor+{}")); writeErr != nil {
			conn.Close()
			continue
		}

		os.mu.Lock()
		os.conn = conn
		os.mu.Unlock()

		os.pollMissedFills(ctx)

		return nil
	}

	return broker.ErrStreamDisconnected
}

// pollMissedFills calls tradesFn (if set) and delivers any unseen fills.
func (os *orderStreamer) pollMissedFills(ctx context.Context) {
	if os.tradesFn == nil {
		return
	}

	trades, fetchErr := os.tradesFn(ctx)
	if fetchErr != nil {
		log.Warn().Err(fetchErr).Msg("ibkr: poll missed fills failed")
		return
	}

	for _, trade := range trades {
		os.deliverFill(ctx, trade.OrderID, trade.Quantity, trade.Price)
	}
}

// deliverFill deduplicates and sends a fill on the fills channel.
func (os *orderStreamer) deliverFill(ctx context.Context, orderID string, qty float64, price float64) {
	fillKey := fmt.Sprintf("%s-%.4f-%.4f", orderID, qty, price)

	os.mu.Lock()

	_, alreadySeen := os.seenFills[fillKey]
	if !alreadySeen {
		os.seenFills[fillKey] = time.Now()
	}
	os.mu.Unlock()

	if alreadySeen {
		return
	}

	fill := broker.Fill{
		OrderID:  orderID,
		Price:    price,
		Qty:      qty,
		FilledAt: time.Now(),
	}

	select {
	case os.fills <- fill:
	case <-ctx.Done():
	}
}

// close cancels the streamer context and closes the WebSocket connection.
func (os *orderStreamer) close() error {
	if os.cancel != nil {
		os.cancel()
	}

	os.mu.Lock()
	conn := os.conn
	os.conn = nil
	os.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}

	return nil
}
