package tradier

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/penny-vault/pvbt/broker"
	"github.com/rs/zerolog/log"
)

const (
	maxReconnectAttempts = 3
	pruneThreshold       = 24 * time.Hour
	sandboxPollInterval  = 2 * time.Second
	productionWSURL      = "wss://ws.tradier.com/v1/accounts/events"
	sandboxWSURL         = "wss://sandbox-ws.tradier.com/v1/accounts/events"
)

type accountStreamer struct {
	client       *apiClient
	fills        chan broker.Fill
	wsURL        string
	wsConn       *websocket.Conn
	sessionID    string
	seenFills    map[string]time.Time
	mu           sync.Mutex
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context
	lastPruneDay time.Time
	sandbox      bool
	lastOrders   map[int64]string // orderID -> last known status
}

type wsSubscription struct {
	Events          []string `json:"events"`
	SessionID       string   `json:"sessionid"`
	ExcludeAccounts []string `json:"excludeAccounts"`
}

func newAccountStreamer(client *apiClient, fills chan broker.Fill, wsURL string, sessionID string, sandbox bool) *accountStreamer {
	return &accountStreamer{
		client:     client,
		fills:      fills,
		wsURL:      wsURL,
		sessionID:  sessionID,
		seenFills:  make(map[string]time.Time),
		done:       make(chan struct{}),
		sandbox:    sandbox,
		lastOrders: make(map[int64]string),
	}
}

func (streamer *accountStreamer) connect(ctx context.Context) error {
	streamer.ctx = ctx

	conn, _, dialErr := websocket.DefaultDialer.DialContext(ctx, streamer.wsURL, nil)
	if dialErr != nil {
		return fmt.Errorf("tradier: account streamer connect: %w", dialErr)
	}

	streamer.mu.Lock()
	streamer.wsConn = conn
	streamer.mu.Unlock()

	sub := wsSubscription{
		Events:          []string{"order"},
		SessionID:       streamer.sessionID,
		ExcludeAccounts: []string{},
	}

	if writeErr := conn.WriteJSON(sub); writeErr != nil {
		conn.Close()
		return fmt.Errorf("tradier: send subscription: %w", writeErr)
	}

	streamer.wg.Add(1)

	go streamer.readLoop()

	return nil
}

func (streamer *accountStreamer) close() error {
	close(streamer.done)

	streamer.mu.Lock()
	conn := streamer.wsConn
	streamer.mu.Unlock()

	if conn != nil {
		conn.Close()
	}

	streamer.wg.Wait()

	return nil
}

func (streamer *accountStreamer) readLoop() {
	defer streamer.wg.Done()

	for {
		streamer.mu.Lock()
		conn := streamer.wsConn
		streamer.mu.Unlock()

		_, data, readErr := conn.ReadMessage()
		if readErr != nil {
			select {
			case <-streamer.done:
				return
			case <-streamer.ctx.Done():
				return
			default:
			}

			if reconnectErr := streamer.reconnect(); reconnectErr != nil {
				log.Error().Err(reconnectErr).Msg("tradier: streamer disconnected after reconnect attempts")
				return
			}

			continue
		}

		var ev tradierAccountEvent
		if unmarshalErr := json.Unmarshal(data, &ev); unmarshalErr != nil {
			log.Warn().Err(unmarshalErr).Msg("tradier: failed to unmarshal account event")
			continue
		}

		streamer.pruneSeen()
		streamer.processEvent(ev)
	}
}

func (streamer *accountStreamer) reconnect() error {
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
		case <-streamer.ctx.Done():
			return streamer.ctx.Err()
		default:
		}

		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-streamer.done:
				timer.Stop()
				return ErrStreamDisconnected
			case <-streamer.ctx.Done():
				timer.Stop()
				return streamer.ctx.Err()
			}

			backoff *= 2
		}

		newSessionID, sessionErr := streamer.client.createStreamSession(streamer.ctx)
		if sessionErr != nil {
			log.Warn().Err(sessionErr).Int("attempt", attempt+1).Msg("tradier: create stream session during reconnect")
			continue
		}

		conn, _, dialErr := websocket.DefaultDialer.DialContext(streamer.ctx, streamer.wsURL, nil)
		if dialErr != nil {
			log.Warn().Err(dialErr).Int("attempt", attempt+1).Msg("tradier: dial WebSocket during reconnect")
			continue
		}

		sub := wsSubscription{
			Events:          []string{"order"},
			SessionID:       newSessionID,
			ExcludeAccounts: []string{},
		}

		if writeErr := conn.WriteJSON(sub); writeErr != nil {
			conn.Close()
			log.Warn().Err(writeErr).Int("attempt", attempt+1).Msg("tradier: send subscription during reconnect")

			continue
		}

		streamer.mu.Lock()
		streamer.sessionID = newSessionID
		streamer.wsConn = conn
		streamer.mu.Unlock()

		streamer.pollForMissedFills()

		return nil
	}

	return ErrStreamDisconnected
}

func (streamer *accountStreamer) pollForMissedFills() {
	orders, fetchErr := streamer.client.getOrders(streamer.ctx)
	if fetchErr != nil {
		log.Warn().Err(fetchErr).Msg("tradier: poll missed fills: get orders failed")
		return
	}

	for _, order := range orders {
		if order.Status != "filled" && order.Status != "partially_filled" {
			continue
		}

		orderID := fmt.Sprintf("%d", order.ID)
		fillTime := parseFillTime(order.TransactionDate)
		fillKey := fmt.Sprintf("%s-%.2f-%s", orderID, order.LastFillQuantity, order.TransactionDate)

		streamer.mu.Lock()

		_, alreadySeen := streamer.seenFills[fillKey]
		if !alreadySeen {
			streamer.seenFills[fillKey] = time.Now()
		}
		streamer.mu.Unlock()

		if alreadySeen {
			continue
		}

		fill := broker.Fill{
			OrderID:  orderID,
			Price:    order.AvgFillPrice,
			Qty:      order.ExecQuantity,
			FilledAt: fillTime,
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

func (streamer *accountStreamer) processEvent(ev tradierAccountEvent) {
	if ev.Status != "filled" && ev.Status != "partially_filled" {
		return
	}

	orderID := fmt.Sprintf("%d", ev.ID)
	fillKey := fmt.Sprintf("%s-%.2f-%s", orderID, ev.LastFillQuantity, ev.TransactionDate)

	streamer.mu.Lock()

	_, alreadySeen := streamer.seenFills[fillKey]
	if !alreadySeen {
		streamer.seenFills[fillKey] = time.Now()
	}
	streamer.mu.Unlock()

	if alreadySeen {
		return
	}

	fillTime := parseFillTime(ev.TransactionDate)

	fill := broker.Fill{
		OrderID:  orderID,
		Price:    ev.AvgFillPrice,
		Qty:      ev.LastFillQuantity,
		FilledAt: fillTime,
	}

	select {
	case streamer.fills <- fill:
	case <-streamer.done:
	case <-streamer.ctx.Done():
	}
}

func (streamer *accountStreamer) pruneSeen() {
	today := time.Now().Truncate(pruneThreshold)

	streamer.mu.Lock()
	defer streamer.mu.Unlock()

	if today.Equal(streamer.lastPruneDay) {
		return
	}

	streamer.lastPruneDay = today
	cutoff := time.Now().Add(-pruneThreshold)

	for fillKey, seenAt := range streamer.seenFills {
		if seenAt.Before(cutoff) {
			delete(streamer.seenFills, fillKey)
		}
	}
}

func (streamer *accountStreamer) startPolling(ctx context.Context) {
	streamer.ctx = ctx

	streamer.wg.Add(1)

	go func() {
		defer streamer.wg.Done()

		ticker := time.NewTicker(sandboxPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-streamer.done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				streamer.pollOrders(ctx)
			}
		}
	}()
}

func (streamer *accountStreamer) pollOrders(ctx context.Context) {
	orders, fetchErr := streamer.client.getOrders(ctx)
	if fetchErr != nil {
		log.Warn().Err(fetchErr).Msg("tradier: sandbox poll: get orders failed")
		return
	}

	for _, order := range orders {
		prev, known := streamer.lastOrders[order.ID]
		currentStatus := order.Status

		streamer.mu.Lock()
		streamer.lastOrders[order.ID] = currentStatus
		streamer.mu.Unlock()

		if known && prev == currentStatus {
			continue
		}

		if currentStatus != "filled" && currentStatus != "partially_filled" {
			continue
		}

		// Only emit if this is a new fill transition.
		if known && (prev == "filled" || prev == "partially_filled") {
			continue
		}

		orderID := fmt.Sprintf("%d", order.ID)
		fillTime := parseFillTime(order.TransactionDate)
		fillKey := fmt.Sprintf("%s-%.2f-%s", orderID, order.ExecQuantity, order.TransactionDate)

		streamer.mu.Lock()

		_, alreadySeen := streamer.seenFills[fillKey]
		if !alreadySeen {
			streamer.seenFills[fillKey] = time.Now()
		}
		streamer.mu.Unlock()

		if alreadySeen {
			continue
		}

		fill := broker.Fill{
			OrderID:  orderID,
			Price:    order.AvgFillPrice,
			Qty:      order.ExecQuantity,
			FilledAt: fillTime,
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

// parseFillTime parses a Tradier transaction date string. If parsing fails the
// current time is returned, which is safe but imprecise.
func parseFillTime(raw string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	}

	for _, format := range formats {
		if ts, parseErr := time.Parse(format, raw); parseErr == nil {
			return ts
		}
	}

	log.Warn().Str("timestamp", raw).Msg("tradier: could not parse fill timestamp, using current time")

	return time.Now()
}
