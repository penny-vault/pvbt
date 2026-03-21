package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/penny-vault/pvbt/broker"
)

const (
	maxReconnectAttempts = 3
	pruneThreshold       = 24 * time.Hour
)

// wsMessage holds the result of a single WebSocket read.
type wsMessage struct {
	data []byte
	err  error
}

type fillStreamer struct {
	client       *apiClient
	fills        chan broker.Fill
	wsURL        string
	wsConn       *websocket.Conn
	seenFills    map[string]time.Time // fill ID -> fill time for deduplication + pruning
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context
	lastPruneDay time.Time // tracks last day pruneSeenFills ran
}

func newFillStreamer(client *apiClient, fills chan broker.Fill, wsURL string) *fillStreamer {
	return &fillStreamer{
		client:    client,
		fills:     fills,
		wsURL:     wsURL,
		seenFills: make(map[string]time.Time),
		done:      make(chan struct{}),
	}
}

// sendConnect sends the required connect action to the WebSocket server.
func (streamer *fillStreamer) sendConnect() error {
	msg := map[string]any{
		"action":     "connect",
		"value":      []string{streamer.client.account()},
		"auth-token": streamer.client.sessionToken(),
	}

	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	if conn == nil {
		return ErrStreamDisconnected
	}

	return conn.WriteJSON(msg)
}

// connect dials the WebSocket and starts the background read loop.
func (streamer *fillStreamer) connect(ctx context.Context) error {
	streamer.ctx = ctx

	conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
	if dialErr != nil {
		return fmt.Errorf("fill streamer connect: %w", dialErr)
	}

	streamer.mu.Lock()
	streamer.wsConn = conn
	streamer.mu.Unlock()

	if connectErr := streamer.sendConnect(); connectErr != nil {
		return fmt.Errorf("fill streamer send connect: %w", connectErr)
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
// sending the error as the final message. Each connection gets its own
// readPump goroutine; when the connection is replaced, the old readPump
// naturally exits because the old conn is closed.
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

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

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
		case <-heartbeat.C:
			streamer.mu.Lock()
			currentConn := streamer.wsConn
			streamer.mu.Unlock()

			if currentConn != nil {
				heartbeatMsg := map[string]string{
					"action":     "heartbeat",
					"auth-token": streamer.client.sessionToken(),
				}

				if writeErr := currentConn.WriteJSON(heartbeatMsg); writeErr != nil {
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

// handleMessage parses a WebSocket message as a streamer envelope and delivers fills.
func (streamer *fillStreamer) handleMessage(data []byte) {
	var msg streamerMessage
	if unmarshalErr := json.Unmarshal(data, &msg); unmarshalErr != nil {
		return
	}

	if msg.Type != "Order" {
		return
	}

	var order orderResponse
	if unmarshalErr := json.Unmarshal(msg.Data, &order); unmarshalErr != nil {
		return
	}

	for _, leg := range order.Legs {
		for _, legFill := range leg.Fills {
			streamer.mu.Lock()

			_, alreadySeen := streamer.seenFills[legFill.FillID]
			if !alreadySeen {
				streamer.seenFills[legFill.FillID] = time.Now()
			}
			streamer.mu.Unlock()

			if alreadySeen {
				continue
			}

			filledAt, parseErr := time.Parse(time.RFC3339, legFill.FilledAt)
			if parseErr != nil {
				continue
			}

			fill := broker.Fill{
				OrderID:  order.ID,
				Price:    legFill.Price,
				Qty:      parseLegFillQuantity(legFill.Quantity),
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
			return ErrStreamDisconnected
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
				return ErrStreamDisconnected
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

		streamer.mu.Lock()
		streamer.wsConn = conn
		streamer.mu.Unlock()

		if connectErr := streamer.sendConnect(); connectErr != nil {
			continue
		}

		streamer.pollMissedFills(ctx)

		return nil
	}

	return ErrStreamDisconnected
}

// pollMissedFills queries orders via REST and sends any fills not yet seen.
func (streamer *fillStreamer) pollMissedFills(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		return
	}

	for _, order := range orders {
		if order.Status != "Filled" {
			continue
		}

		for _, leg := range order.Legs {
			for _, legFill := range leg.Fills {
				streamer.mu.Lock()

				_, alreadySeen := streamer.seenFills[legFill.FillID]
				if !alreadySeen {
					streamer.seenFills[legFill.FillID] = time.Now()
				}
				streamer.mu.Unlock()

				if alreadySeen {
					continue
				}

				filledAt, parseErr := time.Parse(time.RFC3339, legFill.FilledAt)
				if parseErr != nil {
					continue
				}

				fill := broker.Fill{
					OrderID:  order.ID,
					Price:    legFill.Price,
					Qty:      parseLegFillQuantity(legFill.Quantity),
					FilledAt: filledAt,
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
