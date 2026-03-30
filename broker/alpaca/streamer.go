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

package alpaca

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/penny-vault/pvbt/broker"
)

const (
	maxReconnectAttempts = 3
	pruneThreshold       = 24 * time.Hour
	authReadDeadline     = 10 * time.Second
	pingInterval         = 30 * time.Second
)

// wsMessage holds the result of a single WebSocket read.
type wsMessage struct {
	data []byte
	err  error
}

type fillStreamer struct {
	apiKey       string
	apiSecret    string
	fills        chan broker.Fill
	wsURL        string
	wsConn       *websocket.Conn
	seenFills    map[string]time.Time // execution_id -> seen time
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context
	client       *apiClient
	lastPruneDay time.Time
}

// connect dials the WebSocket, authenticates, subscribes to trade_updates,
// and starts the background read loop.
func (streamer *fillStreamer) connect(ctx context.Context) error {
	streamer.ctx = ctx

	conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
	if dialErr != nil {
		return fmt.Errorf("fill streamer connect: %w", dialErr)
	}

	streamer.mu.Lock()
	streamer.wsConn = conn
	streamer.mu.Unlock()

	// Send auth message.
	authMsg := wsAuthMessage{
		Action: "auth",
		Key:    streamer.apiKey,
		Secret: streamer.apiSecret,
	}

	if writeErr := conn.WriteJSON(authMsg); writeErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer send auth: %w", writeErr)
	}

	// Read auth response with deadline.
	if deadlineErr := conn.SetReadDeadline(time.Now().Add(authReadDeadline)); deadlineErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer set auth deadline: %w", deadlineErr)
	}

	_, authData, readErr := conn.ReadMessage()
	if readErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer read auth response: %w", readErr)
	}

	var authResp wsAuthResponse
	if unmarshalErr := sonic.Unmarshal(authData, &authResp); unmarshalErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer parse auth response: %w", unmarshalErr)
	}

	if authResp.Data.Status != "authorized" {
		conn.Close()
		return broker.ErrNotAuthenticated
	}

	// Send listen message.
	listenMsg := wsListenMessage{
		Action: "listen",
	}
	listenMsg.Data.Streams = []string{"trade_updates"}

	if writeErr := conn.WriteJSON(listenMsg); writeErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer send listen: %w", writeErr)
	}

	// Read listen ack with deadline.
	if deadlineErr := conn.SetReadDeadline(time.Now().Add(authReadDeadline)); deadlineErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer set listen deadline: %w", deadlineErr)
	}

	if _, _, ackErr := conn.ReadMessage(); ackErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer read listen ack: %w", ackErr)
	}

	// Clear the read deadline for normal operation.
	if deadlineErr := conn.SetReadDeadline(time.Time{}); deadlineErr != nil {
		conn.Close()
		return fmt.Errorf("fill streamer clear deadline: %w", deadlineErr)
	}

	streamer.wg.Add(1)

	go streamer.run()

	return nil
}

// close signals the background goroutine to exit and waits for it.
// The fills channel is NOT closed here; the broker is responsible for that.
func (streamer *fillStreamer) close() error {
	close(streamer.done)

	// Close the WebSocket to unblock any in-progress ReadMessage call.
	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	if conn != nil {
		conn.Close()
	}

	streamer.wg.Wait()

	return nil
}

// readPump reads messages from the WebSocket connection and sends them to
// the messages channel. It exits when the connection returns an error,
// sending the error as the final message.
func (streamer *fillStreamer) readPump(conn *websocket.Conn, messages chan<- wsMessage) {
	for {
		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			messages <- wsMessage{err: readErr}
			return
		}

		messages <- wsMessage{data: data}
	}
}

// run is the background goroutine that reads fill events from the WebSocket.
func (streamer *fillStreamer) run() {
	defer streamer.wg.Done()

	messages := make(chan wsMessage, 16)

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	go streamer.readPump(conn, messages)

	for {
		select {
		case <-streamer.done:
			return
		case <-streamer.ctx.Done():
			return
		case <-pingTicker.C:
			streamer.mu.Lock()
			currentConn := streamer.wsConn
			streamer.mu.Unlock()

			if currentConn != nil {
				deadline := time.Now().Add(authReadDeadline)
				if writeErr := currentConn.WriteControl(websocket.PingMessage, nil, deadline); writeErr != nil {
					continue
				}
			}
		case msg := <-messages:
			if msg.err != nil {
				select {
				case <-streamer.done:
					return
				case <-streamer.ctx.Done():
					return
				default:
				}

				if reconnectErr := streamer.reconnect(streamer.ctx); reconnectErr != nil {
					return
				}

				streamer.mu.Lock()
				conn = streamer.wsConn
				streamer.mu.Unlock()

				go streamer.readPump(conn, messages)

				continue
			}

			streamer.pruneSeenFills()
			streamer.handleMessage(msg.data)
		}
	}
}

// handleMessage parses a WebSocket message and delivers fills.
func (streamer *fillStreamer) handleMessage(data []byte) {
	var msg wsStreamMessage
	if unmarshalErr := sonic.Unmarshal(data, &msg); unmarshalErr != nil {
		return
	}

	if msg.Stream != "trade_updates" {
		return
	}

	var tradeUpdate wsTradeUpdate
	if unmarshalErr := sonic.Unmarshal(msg.Data, &tradeUpdate); unmarshalErr != nil {
		return
	}

	if tradeUpdate.Event != "fill" && tradeUpdate.Event != "partial_fill" {
		return
	}

	executionID := tradeUpdate.ExecutionID

	streamer.mu.Lock()

	_, alreadySeen := streamer.seenFills[executionID]
	if !alreadySeen {
		streamer.seenFills[executionID] = time.Now()
	}

	streamer.mu.Unlock()

	if alreadySeen {
		return
	}

	filledAt, parseErr := time.Parse(time.RFC3339, tradeUpdate.Timestamp)
	if parseErr != nil {
		return
	}

	fill := broker.Fill{
		OrderID:  tradeUpdate.Order.ID,
		Price:    parseFloat(tradeUpdate.Price),
		Qty:      parseFloat(tradeUpdate.Qty),
		FilledAt: filledAt,
	}

	select {
	case streamer.fills <- fill:
	case <-streamer.done:
		return
	case <-streamer.ctx.Done():
		return
	}
}

// reconnect attempts to re-establish the WebSocket connection with exponential backoff.
func (streamer *fillStreamer) reconnect(ctx context.Context) error {
	streamer.mu.Lock()
	if streamer.wsConn != nil {
		streamer.wsConn.Close()
		streamer.wsConn = nil
	}
	streamer.mu.Unlock()

	backoff := 1 * time.Second

	for attempt := range maxReconnectAttempts {
		select {
		case <-streamer.done:
			return broker.ErrStreamDisconnected
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Wait with backoff (skip on first attempt).
		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return broker.ErrStreamDisconnected
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}

			backoff *= 2
		}

		conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
		if dialErr != nil {
			continue
		}

		// Authenticate the new connection.
		authMsg := wsAuthMessage{
			Action: "auth",
			Key:    streamer.apiKey,
			Secret: streamer.apiSecret,
		}

		if writeErr := conn.WriteJSON(authMsg); writeErr != nil {
			conn.Close()
			continue
		}

		if deadlineErr := conn.SetReadDeadline(time.Now().Add(authReadDeadline)); deadlineErr != nil {
			conn.Close()
			continue
		}

		_, authData, readErr := conn.ReadMessage()
		if readErr != nil {
			conn.Close()
			continue
		}

		var authResp wsAuthResponse
		if unmarshalErr := sonic.Unmarshal(authData, &authResp); unmarshalErr != nil {
			conn.Close()
			continue
		}

		if authResp.Data.Status != "authorized" {
			conn.Close()
			continue
		}

		// Subscribe to trade updates.
		listenMsg := wsListenMessage{
			Action: "listen",
		}
		listenMsg.Data.Streams = []string{"trade_updates"}

		if writeErr := conn.WriteJSON(listenMsg); writeErr != nil {
			conn.Close()
			continue
		}

		// Read listen ack.
		if deadlineErr := conn.SetReadDeadline(time.Now().Add(authReadDeadline)); deadlineErr != nil {
			conn.Close()
			continue
		}

		if _, _, ackErr := conn.ReadMessage(); ackErr != nil {
			conn.Close()
			continue
		}

		// Clear the read deadline for normal operation.
		if deadlineErr := conn.SetReadDeadline(time.Time{}); deadlineErr != nil {
			conn.Close()
			continue
		}

		streamer.mu.Lock()
		streamer.wsConn = conn
		streamer.mu.Unlock()

		streamer.pollMissedFills(ctx)

		return nil
	}

	return broker.ErrStreamDisconnected
}

// pollMissedFills queries orders via REST and sends any fills not yet seen.
func (streamer *fillStreamer) pollMissedFills(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		return
	}

	for _, order := range orders {
		if order.Status != "filled" && order.Status != "partially_filled" {
			continue
		}

		dedupKey := "poll-" + order.ID

		streamer.mu.Lock()

		_, alreadySeen := streamer.seenFills[dedupKey]
		if !alreadySeen {
			streamer.seenFills[dedupKey] = time.Now()
		}

		streamer.mu.Unlock()

		if alreadySeen {
			continue
		}

		fill := broker.Fill{
			OrderID:  order.ID,
			Price:    parseFloat(order.FilledAvgPrice),
			Qty:      parseFloat(order.FilledQty),
			FilledAt: time.Now(),
		}

		select {
		case streamer.fills <- fill:
		case <-streamer.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

// pruneSeenFills removes entries from seenFills that are older than 24 hours.
// It runs at most once per calendar day.
func (streamer *fillStreamer) pruneSeenFills() {
	today := time.Now().Truncate(pruneThreshold)

	streamer.mu.Lock()
	defer streamer.mu.Unlock()

	if today.Equal(streamer.lastPruneDay) {
		return
	}

	streamer.lastPruneDay = today
	cutoff := time.Now().Add(-pruneThreshold)

	for fillID, seenAt := range streamer.seenFills {
		if seenAt.Before(cutoff) {
			delete(streamer.seenFills, fillID)
		}
	}
}
